import { DurableObject } from "cloudflare:workers";

interface CodespaceCredentials {
  githubToken: string;
  githubUser: string;
  codespaceName: string;
  createdAt: number;
}

interface StoredCodespaceCredentials {
  keyId: number;
  salt: string;
  iv: string;
  encryptedData: string;
  createdAt: number;
}

export class CodespaceStore extends DurableObject<Record<string, any>> {
  private sql: SqlStorage;
  private keys: Map<number, CryptoKey> = new Map();
  private currentKeyId: number = 0;
  private initialized = false;

  constructor(ctx: DurableObjectState, env: Record<string, any>) {
    super(ctx, env);
    this.sql = ctx.storage.sql;

    // Initialize database schema
    this.sql.exec(`
      CREATE TABLE IF NOT EXISTS codespace_credentials (
        github_user TEXT PRIMARY KEY,
        key_id INTEGER NOT NULL,
        salt TEXT NOT NULL,
        iv TEXT NOT NULL,
        encrypted_data TEXT NOT NULL,
        created_at INTEGER NOT NULL
      );
      CREATE INDEX IF NOT EXISTS idx_created_at ON codespace_credentials(created_at);
    `);
  }

  private async initKeys() {
    if (this.initialized) return;

    // Import encryption keys from environment - reuse the same key system as sessions
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
    data: CodespaceCredentials,
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

  private async decrypt(
    stored: StoredCodespaceCredentials,
  ): Promise<CodespaceCredentials> {
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
    const githubUser = url.pathname.split("/").pop();

    if (request.method === "GET" && githubUser) {
      // Get credentials by GitHub user
      const rows = this.sql
        .exec(
          "SELECT * FROM codespace_credentials WHERE github_user = ? LIMIT 1",
          githubUser,
        )
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
      } as StoredCodespaceCredentials;

      try {
        const credentials = await this.decrypt(result);

        // Check if credentials are expired (24 hours)
        const twentyFourHoursAgo = Date.now() - 24 * 60 * 60 * 1000;
        if (credentials.createdAt < twentyFourHoursAgo) {
          // Clean up expired credentials
          this.sql.exec(
            "DELETE FROM codespace_credentials WHERE github_user = ?",
            githubUser,
          );
          return new Response("Credentials expired", { status: 404 });
        }

        return Response.json(credentials);
      } catch (error) {
        console.error("Decryption error:", error);
        return new Response("Invalid credentials", { status: 500 });
      }
    }

    if (request.method === "PUT" && githubUser) {
      // Store new credentials
      const credentials: CodespaceCredentials = await request.json();
      const { salt, iv, encrypted } = await this.encrypt(
        credentials,
        this.currentKeyId,
      );
      const now = Date.now();

      // Replace existing credentials for this user
      this.sql.exec(
        `INSERT OR REPLACE INTO codespace_credentials 
          (github_user, key_id, salt, iv, encrypted_data, created_at)
        VALUES (?, ?, ?, ?, ?, ?)`,
        githubUser,
        this.currentKeyId,
        salt,
        iv,
        encrypted,
        now,
      );

      return new Response("OK");
    }

    if (request.method === "DELETE" && githubUser) {
      // Delete credentials
      this.sql.exec(
        "DELETE FROM codespace_credentials WHERE github_user = ?",
        githubUser,
      );
      return new Response("OK");
    }

    // Cleanup old credentials (older than 24 hours)
    if (request.method === "POST" && url.pathname.endsWith("/cleanup")) {
      const twentyFourHoursAgo = Date.now() - 24 * 60 * 60 * 1000;
      this.sql.exec(
        "DELETE FROM codespace_credentials WHERE created_at < ?",
        twentyFourHoursAgo,
      );
      return new Response("OK");
    }

    return new Response("Method not allowed", { status: 405 });
  }
}
