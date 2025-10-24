import { DurableObject } from "cloudflare:workers";

interface CodespaceCredentials {
  githubToken: string;
  githubUser: string;
  codespaceName: string;
  githubRepository?: string;
  createdAt: number;
  updatedAt: number;
}

interface StoredCodespaceCredentials {
  keyId: number | null;
  salt: string | null;
  iv: string | null;
  encryptedData: string | null;
  createdAt: number;
  updatedAt: number;
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
        codespace_name TEXT PRIMARY KEY,
        github_user TEXT NOT NULL,
        key_id INTEGER,
        salt TEXT,
        iv TEXT,
        encrypted_data TEXT,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
      );
      CREATE INDEX IF NOT EXISTS idx_github_user ON codespace_credentials(github_user);
      CREATE INDEX IF NOT EXISTS idx_created_at ON codespace_credentials(created_at);
      CREATE INDEX IF NOT EXISTS idx_updated_at ON codespace_credentials(updated_at);
    `);

    // Migration: Add github_repository column if it doesn't exist
    try {
      // Try to add the column - will fail silently if it already exists
      this.sql.exec(`
        ALTER TABLE codespace_credentials ADD COLUMN github_repository TEXT;
      `);
    } catch (_error) {
      // Column already exists or other error - safe to ignore
      // SQLite will throw if column already exists
    }
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
    if (
      stored.keyId === null ||
      stored.salt === null ||
      stored.iv === null ||
      stored.encryptedData === null
    ) {
      throw new Error("Invalid stored credentials: missing encryption data");
    }

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
    const pathParts = url.pathname.split("/");

    // Handle specific codespace lookup: /internal/codespace/{username}/{codespaceName}
    if (pathParts.length >= 4 && request.method === "GET") {
      const codespaceName = pathParts.pop();
      const githubUser = pathParts.pop();

      if (githubUser && codespaceName) {
        const rows = this.sql
          .exec(
            "SELECT * FROM codespace_credentials WHERE github_user = ? AND codespace_name = ? ORDER BY updated_at DESC LIMIT 1",
            githubUser,
            codespaceName,
          )
          .toArray();

        if (rows.length === 0) {
          return new Response("Codespace not found", { status: 404 });
        }

        const row = rows[0];
        const result = {
          keyId: row.key_id as number | null,
          salt: row.salt as string | null,
          iv: row.iv as string | null,
          encryptedData: row.encrypted_data as string | null,
          createdAt: row.created_at as number,
          updatedAt: row.updated_at as number,
        } as StoredCodespaceCredentials;

        // Check if credentials are already nullified (expired)
        if (
          !result.encryptedData ||
          !result.salt ||
          !result.iv ||
          !result.keyId
        ) {
          // Return basic codespace info without credentials
          const basicCodespace: CodespaceCredentials = {
            githubToken: "", // Empty token - will need to be refreshed
            githubUser: githubUser,
            codespaceName: row.codespace_name as string,
            githubRepository: row.github_repository as string | undefined,
            createdAt: result.createdAt,
            updatedAt: result.updatedAt,
          };
          return Response.json(basicCodespace);
        }

        try {
          const credentials = await this.decrypt(result);

          // Check if credentials are expired (24 hours)
          const twentyFourHoursAgo = Date.now() - 24 * 60 * 60 * 1000;
          if (credentials.updatedAt < twentyFourHoursAgo) {
            // Null out expired credentials but keep codespace record
            this.sql.exec(
              "UPDATE codespace_credentials SET key_id = NULL, salt = NULL, iv = NULL, encrypted_data = NULL WHERE codespace_name = ?",
              credentials.codespaceName,
            );

            // Return basic codespace info without credentials
            const basicCodespace: CodespaceCredentials = {
              githubToken: "", // Empty token - will need to be refreshed
              githubUser: githubUser,
              codespaceName: credentials.codespaceName,
              githubRepository: credentials.githubRepository,
              createdAt: credentials.createdAt,
              updatedAt: credentials.updatedAt,
            };
            return Response.json(basicCodespace);
          }

          return Response.json(credentials);
        } catch (error) {
          console.error("Decryption error:", error);
          // Return basic codespace info without credentials on decryption error
          const basicCodespace: CodespaceCredentials = {
            githubToken: "", // Empty token - will need to be refreshed
            githubUser: githubUser,
            codespaceName: row.codespace_name as string,
            githubRepository: row.github_repository as string | undefined,
            createdAt: result.createdAt,
            updatedAt: result.updatedAt,
          };
          return Response.json(basicCodespace);
        }
      }
    }

    // Handle specific codespace deletion: /internal/codespace/{username}/{codespaceName}
    if (pathParts.length >= 4 && request.method === "DELETE") {
      const codespaceName = pathParts.pop();
      const githubUser = pathParts.pop();

      if (githubUser && codespaceName) {
        const result = this.sql.exec(
          "DELETE FROM codespace_credentials WHERE codespace_name = ? AND github_user = ?",
          codespaceName,
          githubUser,
        );

        if (result.rowsWritten !== 1) {
          return new Response("Codespace not found in store", { status: 404 });
        }

        return new Response("OK");
      }
    }

    const githubUser = pathParts.pop();

    if (request.method === "GET" && githubUser) {
      // Check if requesting all codespaces
      const getAllParam = url.searchParams.get("all");

      if (getAllParam === "true") {
        // Get all codespaces by GitHub user (including those without credentials)
        const rows = this.sql
          .exec(
            "SELECT * FROM codespace_credentials WHERE github_user = ? ORDER BY updated_at DESC",
            githubUser,
          )
          .toArray();

        if (rows.length === 0) {
          return new Response("Not found", { status: 404 });
        }

        const availableCodespaces: CodespaceCredentials[] = [];
        const twentyFourHoursAgo = Date.now() - 24 * 60 * 60 * 1000;

        for (const row of rows) {
          const result = {
            keyId: row.key_id as number | null,
            salt: row.salt as string | null,
            iv: row.iv as string | null,
            encryptedData: row.encrypted_data as string | null,
            createdAt: row.created_at as number,
            updatedAt: row.updated_at as number,
          } as StoredCodespaceCredentials;

          // If credentials are nullified (expired), create a basic codespace entry
          if (
            !result.encryptedData ||
            !result.salt ||
            !result.iv ||
            !result.keyId
          ) {
            const basicCodespace: CodespaceCredentials = {
              githubToken: "", // Empty token - will need to be refreshed
              githubUser: githubUser,
              codespaceName: row.codespace_name as string,
              createdAt: result.createdAt,
              updatedAt: result.updatedAt,
            };
            availableCodespaces.push(basicCodespace);
            continue;
          }

          try {
            const credentials = await this.decrypt(result);

            // Check if credentials are expired (24 hours)
            if (credentials.updatedAt < twentyFourHoursAgo) {
              // Null out expired credentials but keep codespace record
              this.sql.exec(
                "UPDATE codespace_credentials SET key_id = NULL, salt = NULL, iv = NULL, encrypted_data = NULL WHERE codespace_name = ?",
                credentials.codespaceName,
              );

              // Still add as available codespace without credentials
              const basicCodespace: CodespaceCredentials = {
                githubToken: "", // Empty token - will need to be refreshed
                githubUser: githubUser,
                codespaceName: credentials.codespaceName,
                createdAt: credentials.createdAt,
                updatedAt: credentials.updatedAt,
              };
              availableCodespaces.push(basicCodespace);
              continue;
            }

            availableCodespaces.push(credentials);
          } catch (error) {
            console.error("Decryption error for codespace:", error);
            // Still add as available codespace without credentials
            const basicCodespace: CodespaceCredentials = {
              githubToken: "", // Empty token - will need to be refreshed
              githubUser: githubUser,
              codespaceName: row.codespace_name as string,
              createdAt: result.createdAt,
              updatedAt: result.updatedAt,
            };
            availableCodespaces.push(basicCodespace);
            continue;
          }
        }

        if (availableCodespaces.length === 0) {
          return new Response("No codespaces found", { status: 404 });
        }

        return Response.json(availableCodespaces);
      } else {
        // Get most recent codespace by GitHub user (including those without credentials)
        const rows = this.sql
          .exec(
            "SELECT * FROM codespace_credentials WHERE github_user = ? ORDER BY updated_at DESC LIMIT 1",
            githubUser,
          )
          .toArray();

        if (rows.length === 0) {
          return new Response("Not found", { status: 404 });
        }

        const row = rows[0];
        const result = {
          keyId: row.key_id as number | null,
          salt: row.salt as string | null,
          iv: row.iv as string | null,
          encryptedData: row.encrypted_data as string | null,
          createdAt: row.created_at as number,
          updatedAt: row.updated_at as number,
        } as StoredCodespaceCredentials;

        // Check if credentials are already nullified (expired)
        if (
          !result.encryptedData ||
          !result.salt ||
          !result.iv ||
          !result.keyId
        ) {
          // Return basic codespace info without credentials
          const basicCodespace: CodespaceCredentials = {
            githubToken: "", // Empty token - will need to be refreshed
            githubUser: githubUser,
            codespaceName: row.codespace_name as string,
            githubRepository: row.github_repository as string | undefined,
            createdAt: result.createdAt,
            updatedAt: result.updatedAt,
          };
          return Response.json(basicCodespace);
        }

        try {
          const credentials = await this.decrypt(result);

          // Check if credentials are expired (24 hours)
          const twentyFourHoursAgo = Date.now() - 24 * 60 * 60 * 1000;
          if (credentials.updatedAt < twentyFourHoursAgo) {
            // Null out expired credentials but keep codespace record
            this.sql.exec(
              "UPDATE codespace_credentials SET key_id = NULL, salt = NULL, iv = NULL, encrypted_data = NULL WHERE codespace_name = ?",
              credentials.codespaceName,
            );

            // Return basic codespace info without credentials
            const basicCodespace: CodespaceCredentials = {
              githubToken: "", // Empty token - will need to be refreshed
              githubUser: githubUser,
              codespaceName: credentials.codespaceName,
              githubRepository: credentials.githubRepository,
              createdAt: credentials.createdAt,
              updatedAt: credentials.updatedAt,
            };
            return Response.json(basicCodespace);
          }

          return Response.json(credentials);
        } catch (error) {
          console.error("Decryption error:", error);
          // Return basic codespace info without credentials on decryption error
          const basicCodespace: CodespaceCredentials = {
            githubToken: "", // Empty token - will need to be refreshed
            githubUser: githubUser,
            codespaceName: row.codespace_name as string,
            githubRepository: row.github_repository as string | undefined,
            createdAt: result.createdAt,
            updatedAt: result.updatedAt,
          };
          return Response.json(basicCodespace);
        }
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

      // Check if credentials already exist for this codespace
      const existingRows = this.sql
        .exec(
          "SELECT created_at FROM codespace_credentials WHERE codespace_name = ? LIMIT 1",
          credentials.codespaceName,
        )
        .toArray();

      const createdAt =
        existingRows.length > 0 ? (existingRows[0].created_at as number) : now;

      // Insert or replace credentials for this specific codespace
      this.sql.exec(
        `INSERT OR REPLACE INTO codespace_credentials
          (codespace_name, github_user, github_repository, key_id, salt, iv, encrypted_data, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        credentials.codespaceName,
        credentials.githubUser,
        credentials.githubRepository || null,
        this.currentKeyId,
        salt,
        iv,
        encrypted,
        createdAt,
        now,
      );

      return new Response("OK");
    }

    if (request.method === "DELETE" && githubUser) {
      // Delete all credentials for this user
      this.sql.exec(
        "DELETE FROM codespace_credentials WHERE github_user = ?",
        githubUser,
      );
      return new Response("OK");
    }

    // Cleanup old credentials (older than 24 hours) - null out encrypted data but keep records
    if (request.method === "POST" && url.pathname.endsWith("/cleanup")) {
      const twentyFourHoursAgo = Date.now() - 24 * 60 * 60 * 1000;
      this.sql.exec(
        "UPDATE codespace_credentials SET key_id = NULL, salt = NULL, iv = NULL, encrypted_data = NULL WHERE updated_at < ? AND encrypted_data IS NOT NULL",
        twentyFourHoursAgo,
      );
      return new Response("OK");
    }

    return new Response("Method not allowed", { status: 405 });
  }
}
