import { DurableObject } from "cloudflare:workers";

interface SessionData {
  userId: string;
  username: string;
  accessToken: string;
  refreshToken?: string;
  expiresAt: number;
  refreshTokenExpiresAt?: number;
}

interface StoredSession {
  keyId: number;
  salt: string;
  iv: string;
  encryptedData: string;
  createdAt: number;
  refreshedAt: number;
}

interface MobileSessionRequest {
  sessionId: string;
  userId: string;
  username: string;
  expiresAt: number;
}

export class SessionStore extends DurableObject<Record<string, any>> {
  private sql: SqlStorage;
  private keys: Map<number, CryptoKey> = new Map();
  private currentKeyId: number = 0;
  private initialized = false;

  constructor(ctx: DurableObjectState, env: Record<string, any>) {
    super(ctx, env);
    this.sql = ctx.storage.sql;

    // Initialize database schema
    this.sql.exec(`
      CREATE TABLE IF NOT EXISTS sessions (
        session_id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        key_id INTEGER NOT NULL,
        salt TEXT NOT NULL,
        iv TEXT NOT NULL,
        encrypted_data TEXT NOT NULL,
        created_at INTEGER NOT NULL,
        refreshed_at INTEGER NOT NULL,
        expires_at INTEGER NOT NULL,
        refresh_token_expires_at INTEGER
      );
      CREATE INDEX IF NOT EXISTS idx_user_id ON sessions(user_id);
      CREATE INDEX IF NOT EXISTS idx_refreshed_at ON sessions(refreshed_at);
      CREATE INDEX IF NOT EXISTS idx_expires_at ON sessions(expires_at);
      CREATE INDEX IF NOT EXISTS idx_refresh_token_expires_at ON sessions(refresh_token_expires_at);

      -- Mobile session tokens table
      CREATE TABLE IF NOT EXISTS mobile_sessions (
        mobile_token TEXT PRIMARY KEY,
        session_id TEXT NOT NULL,
        user_id TEXT NOT NULL,
        username TEXT NOT NULL,
        created_at INTEGER NOT NULL,
        expires_at INTEGER NOT NULL,
        FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE CASCADE
      );
      CREATE INDEX IF NOT EXISTS idx_mobile_session_id ON mobile_sessions(session_id);
      CREATE INDEX IF NOT EXISTS idx_mobile_expires ON mobile_sessions(expires_at);
    `);

    // Migrate existing sessions table if needed
    try {
      this.sql.exec(
        `ALTER TABLE sessions ADD COLUMN expires_at INTEGER NOT NULL DEFAULT 0`,
      );
      this.sql.exec(
        `ALTER TABLE sessions ADD COLUMN refresh_token_expires_at INTEGER`,
      );
    } catch {
      // Columns already exist, ignore error
    }
  }

  private async initKeys() {
    if (this.initialized) return;

    // Import encryption keys from environment
    // Support multiple key versions for rotation
    const keyConfigs = [
      { id: 2, env: "CATNIP_ENCRYPTION_KEY_V2" },
      { id: 1, env: "CATNIP_ENCRYPTION_KEY_V1" },
    ];

    for (const config of keyConfigs) {
      const keyString = this.env[config.env] || this.env.CATNIP_ENCRYPTION_KEY;
      if (keyString) {
        const key = await this.importKey(keyString);
        this.keys.set(config.id, key);
        this.currentKeyId = Math.max(this.currentKeyId, config.id);
      }
    }

    // Fallback to single key if no versioned keys
    if (this.keys.size === 0 && this.env.CATNIP_ENCRYPTION_KEY) {
      const key = await this.importKey(this.env.CATNIP_ENCRYPTION_KEY);
      this.keys.set(1, key);
      this.currentKeyId = 1;
    }

    this.initialized = true;
  }

  private async importKey(keyString: string): Promise<CryptoKey> {
    // Handle base64url encoded keys (convert to standard base64)
    const base64 = keyString
      .replace(/-/g, "+")
      .replace(/_/g, "/")
      .padEnd(keyString.length + ((4 - (keyString.length % 4)) % 4), "=");

    const keyData = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
    return await crypto.subtle.importKey(
      "raw",
      keyData,
      { name: "AES-GCM" },
      false,
      ["encrypt", "decrypt"],
    );
  }

  private async encrypt(
    data: SessionData,
    keyId: number,
  ): Promise<{ salt: string; iv: string; encrypted: string }> {
    const key = this.keys.get(keyId);
    if (!key) throw new Error(`Key ${keyId} not found`);

    const salt = crypto.getRandomValues(new Uint8Array(16));
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const encoder = new TextEncoder();

    const encrypted = await crypto.subtle.encrypt(
      {
        name: "AES-GCM",
        iv: iv,
        additionalData: salt,
      },
      key,
      encoder.encode(JSON.stringify(data)),
    );

    return {
      salt: btoa(String.fromCharCode(...salt)),
      iv: btoa(String.fromCharCode(...iv)),
      encrypted: btoa(String.fromCharCode(...new Uint8Array(encrypted))),
    };
  }

  private async decrypt(stored: StoredSession): Promise<SessionData> {
    const key = this.keys.get(stored.keyId);
    if (!key) throw new Error(`Key ${stored.keyId} not found`);

    const salt = Uint8Array.from(atob(stored.salt), (c) => c.charCodeAt(0));
    const iv = Uint8Array.from(atob(stored.iv), (c) => c.charCodeAt(0));
    const encrypted = Uint8Array.from(atob(stored.encryptedData), (c) =>
      c.charCodeAt(0),
    );

    const decrypted = await crypto.subtle.decrypt(
      {
        name: "AES-GCM",
        iv: iv,
        additionalData: salt,
      },
      key,
      encrypted,
    );

    const decoder = new TextDecoder();
    return JSON.parse(decoder.decode(decrypted));
  }

  async fetch(request: Request): Promise<Response> {
    await this.initKeys();
    const url = new URL(request.url);
    const sessionId = url.pathname.split("/").pop();

    if (request.method === "GET" && sessionId) {
      // Get session by ID
      const rows = this.sql
        .exec("SELECT * FROM sessions WHERE session_id = ? LIMIT 1", sessionId)
        .toArray();

      if (rows.length === 0) {
        return new Response("Not found", { status: 404 });
      }

      const row = rows[0];

      const result = {
        keyId: row.key_id as number,
        salt: row.salt as string,
        iv: row.iv as string,
        encryptedData: row.encrypted_data as string,
        createdAt: row.created_at as number,
        refreshedAt: row.refreshed_at as number,
      } as StoredSession;

      try {
        const sessionData = await this.decrypt(result);

        // Check if expired
        if (Date.now() > sessionData.expiresAt) {
          // Clean up expired session
          this.sql.exec("DELETE FROM sessions WHERE session_id = ?", sessionId);
          return new Response("Session expired", { status: 404 });
        }

        // Update refreshed_at on every read
        const now = Date.now();

        // Re-encrypt with current key if needed
        if (result.keyId !== this.currentKeyId) {
          const { salt, iv, encrypted } = await this.encrypt(
            sessionData,
            this.currentKeyId,
          );
          this.sql.exec(
            `UPDATE sessions SET 
              key_id = ?, salt = ?, iv = ?, encrypted_data = ?, refreshed_at = ?
            WHERE session_id = ?`,
            this.currentKeyId,
            salt,
            iv,
            encrypted,
            now,
            sessionId,
          );
        } else {
          // Just update refreshed_at
          this.sql.exec(
            "UPDATE sessions SET refreshed_at = ? WHERE session_id = ?",
            now,
            sessionId,
          );
        }

        return Response.json(sessionData);
      } catch (error) {
        console.error("Decryption error:", error);
        return new Response("Invalid session", { status: 500 });
      }
    }

    if (request.method === "PUT" && sessionId) {
      // Store new session
      const sessionData: SessionData = await request.json();
      const { salt, iv, encrypted } = await this.encrypt(
        sessionData,
        this.currentKeyId,
      );
      const now = Date.now();

      // Check if this is an update or new session
      const existingResult = this.sql.exec(
        "SELECT created_at FROM sessions WHERE session_id = ? LIMIT 1",
        sessionId,
      );
      const existing = existingResult.toArray()[0];

      if (existing) {
        // Update existing session, preserve created_at
        this.sql.exec(
          `UPDATE sessions SET 
            user_id = ?, key_id = ?, salt = ?, iv = ?, encrypted_data = ?, refreshed_at = ?, expires_at = ?, refresh_token_expires_at = ?
          WHERE session_id = ?`,
          sessionData.userId,
          this.currentKeyId,
          salt,
          iv,
          encrypted,
          now,
          sessionData.expiresAt,
          sessionData.refreshTokenExpiresAt || null,
          sessionId,
        );
      } else {
        // Insert new session
        this.sql.exec(
          `INSERT INTO sessions 
            (session_id, user_id, key_id, salt, iv, encrypted_data, created_at, refreshed_at, expires_at, refresh_token_expires_at)
          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
          sessionId,
          sessionData.userId,
          this.currentKeyId,
          salt,
          iv,
          encrypted,
          now,
          now,
          sessionData.expiresAt,
          sessionData.refreshTokenExpiresAt || null,
        );
      }

      return new Response("OK");
    }

    if (request.method === "DELETE" && sessionId) {
      // Delete session
      this.sql.exec("DELETE FROM sessions WHERE session_id = ?", sessionId);
      return new Response("OK");
    }

    // Handle mobile session endpoints
    if (url.pathname.startsWith("/mobile-session/")) {
      const mobileToken = url.pathname.split("/").pop();

      if (request.method === "GET" && mobileToken) {
        // Get mobile session
        const rows = this.sql
          .exec(
            "SELECT * FROM mobile_sessions WHERE mobile_token = ? AND expires_at > ? LIMIT 1",
            mobileToken,
            Date.now(),
          )
          .toArray();

        if (!rows[0]) {
          return new Response("Not found", { status: 404 });
        }

        return Response.json({
          sessionId: rows[0].session_id,
          userId: rows[0].user_id,
          username: rows[0].username,
          createdAt: rows[0].created_at,
          expiresAt: rows[0].expires_at,
        });
      }

      if (request.method === "PUT" && mobileToken) {
        // Store mobile session
        const data = (await request.json()) as MobileSessionRequest;
        const now = Date.now();

        // Delete any existing mobile session for this token
        this.sql.exec(
          "DELETE FROM mobile_sessions WHERE mobile_token = ?",
          mobileToken,
        );

        // Insert new mobile session
        this.sql.exec(
          `INSERT INTO mobile_sessions
            (mobile_token, session_id, user_id, username, created_at, expires_at)
          VALUES (?, ?, ?, ?, ?, ?)`,
          mobileToken,
          data.sessionId,
          data.userId,
          data.username,
          now,
          data.expiresAt,
        );

        return new Response("OK");
      }

      if (request.method === "DELETE" && mobileToken) {
        // Delete mobile session
        this.sql.exec(
          "DELETE FROM mobile_sessions WHERE mobile_token = ?",
          mobileToken,
        );
        return new Response("OK");
      }
    }

    if (request.method === "POST" && url.pathname.endsWith("/refresh")) {
      // Refresh token endpoint
      const { sessionId: sid, refreshToken: _refreshToken } =
        (await request.json()) as { sessionId: string; refreshToken: string };

      const rows = this.sql
        .exec("SELECT * FROM sessions WHERE session_id = ? LIMIT 1", sid)
        .toArray();

      if (rows.length === 0) {
        return new Response("Not found", { status: 404 });
      }

      const row = rows[0];

      const result = {
        keyId: row.key_id as number,
        salt: row.salt as string,
        iv: row.iv as string,
        encryptedData: row.encrypted_data as string,
        createdAt: row.created_at as number,
        refreshedAt: row.refreshed_at as number,
      } as StoredSession;

      try {
        const _sessionData = await this.decrypt(result);

        // TODO: Use refreshToken to get new accessToken from GitHub
        // For now, just update the refreshedAt timestamp

        this.sql.exec(
          "UPDATE sessions SET refreshed_at = ? WHERE session_id = ?",
          Date.now(),
          sid,
        );

        return Response.json({ success: true });
      } catch (_error) {
        return new Response("Refresh failed", { status: 500 });
      }
    }

    // Cleanup old sessions (older than 30 days)
    if (request.method === "POST" && url.pathname.endsWith("/cleanup")) {
      const thirtyDaysAgo = Date.now() - 30 * 24 * 60 * 60 * 1000;
      this.sql.exec(
        "DELETE FROM sessions WHERE refreshed_at < ?",
        thirtyDaysAgo,
      );
      return new Response("OK");
    }

    return new Response("Method not allowed", { status: 405 });
  }
}
