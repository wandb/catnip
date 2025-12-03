import { DurableObject } from "cloudflare:workers";
import { getContainer } from "@cloudflare/containers";
import type { KeepAliveContainer, CodespaceStore } from "./index";

/**
 * KeepAliveCoordinator
 *
 * Tracks active codespaces and coordinates keep-alive pings.
 *
 * Responsibilities:
 * - Track last activity time per codespace
 * - Use Durable Object alarms to check every 60 seconds
 * - Invoke KeepAliveContainer to ping codespaces every 5 minutes when active
 * - Stop pinging after 5 minutes of inactivity (app pings /v1/info every minute)
 */

// Configuration constants
const INACTIVITY_THRESHOLD_MS = 5 * 60 * 1000; // 5 minutes
const PING_INTERVAL_MS = 5 * 60 * 1000; // 5 minutes
const ALARM_INTERVAL_MS = 60 * 1000; // 60 seconds

interface CodespaceActivity {
  codespaceName: string;
  githubUser: string;
  lastActivityTime: number;
  lastPingTime: number;
}

export class KeepAliveCoordinator extends DurableObject<{
  KEEPALIVE_CONTAINER: DurableObjectNamespace<KeepAliveContainer>;
  CODESPACE_STORE: DurableObjectNamespace<CodespaceStore>;
}> {
  private sql: SqlStorage;

  constructor(
    ctx: DurableObjectState,
    env: {
      KEEPALIVE_CONTAINER: DurableObjectNamespace<KeepAliveContainer>;
      CODESPACE_STORE: DurableObjectNamespace<CodespaceStore>;
    },
  ) {
    super(ctx, env);
    this.sql = ctx.storage.sql;

    // Initialize database schema
    // NOTE: If the table already exists with a github_token column from the old schema,
    // you'll need to drop it manually. The column is no longer used - tokens are now
    // fetched from the encrypted CodespaceStore instead.
    //
    // To manually drop the column, you can add a one-time endpoint or run:
    // CREATE TABLE codespace_activity_new (...); INSERT INTO ...; DROP TABLE ...; RENAME ...
    this.sql.exec(`
      CREATE TABLE IF NOT EXISTS codespace_activity (
        codespace_name TEXT PRIMARY KEY,
        github_user TEXT NOT NULL,
        last_activity_time INTEGER NOT NULL,
        last_ping_time INTEGER NOT NULL
      );
      CREATE INDEX IF NOT EXISTS idx_last_activity ON codespace_activity(last_activity_time);
    `);
  }

  /**
   * Set up the alarm to fire every 60 seconds
   */
  async alarm(): Promise<void> {
    const now = Date.now();
    const inactivityCutoff = now - INACTIVITY_THRESHOLD_MS;

    console.log("🫀 KeepAlive alarm checking active codespaces");

    // Get all active codespaces (activity within inactivity threshold)
    const rows = this.sql
      .exec(
        "SELECT * FROM codespace_activity WHERE last_activity_time > ?",
        inactivityCutoff,
      )
      .toArray();

    console.log(`🫀 Found ${rows.length} active codespace(s)`);

    // Check each active codespace
    for (const row of rows) {
      const activity: CodespaceActivity = {
        codespaceName: row.codespace_name as string,
        githubUser: row.github_user as string,
        lastActivityTime: row.last_activity_time as number,
        lastPingTime: row.last_ping_time as number,
      };

      // Only ping if:
      // 1. Last activity is within inactivity threshold (already filtered by query)
      // 2. Last ping was more than ping interval ago
      if (activity.lastPingTime < now - PING_INTERVAL_MS) {
        console.log(`🫀 Sending keep-alive ping for ${activity.codespaceName}`);
        await this.pingCodespace(activity.codespaceName, activity.githubUser);
      } else {
        const nextPingIn = Math.ceil(
          (activity.lastPingTime + PING_INTERVAL_MS - now) / 1000,
        );
        console.log(
          `🫀 Skipping ${activity.codespaceName}, last ping was ${Math.floor((now - activity.lastPingTime) / 1000)}s ago (next ping in ${nextPingIn}s)`,
        );
      }
    }

    // Clean up inactive codespaces (no activity within inactivity threshold)
    const result = this.sql.exec(
      "DELETE FROM codespace_activity WHERE last_activity_time <= ?",
      inactivityCutoff,
    );

    if (result.rowsWritten > 0) {
      console.log(`🫀 Cleaned up ${result.rowsWritten} inactive codespace(s)`);
    }

    // Only schedule next alarm if there are still active codespaces
    const remainingRows = this.sql
      .exec(
        "SELECT COUNT(*) as count FROM codespace_activity WHERE last_activity_time > ?",
        inactivityCutoff,
      )
      .toArray();

    const remainingCount = (remainingRows[0]?.count as number) || 0;

    if (remainingCount > 0) {
      await this.ctx.storage.setAlarm(Date.now() + ALARM_INTERVAL_MS);
      console.log(
        `🫀 Next alarm scheduled in ${ALARM_INTERVAL_MS / 1000}s (${remainingCount} active codespace(s))`,
      );
    } else {
      console.log(
        "🫀 No active codespaces, alarm will restart on next activity",
      );
    }
  }

  /**
   * Ping a codespace using the KeepAliveContainer
   * Fetches encrypted credentials from CodespaceStore
   */
  private async pingCodespace(
    codespaceName: string,
    githubUser: string,
  ): Promise<void> {
    try {
      // Fetch encrypted credentials from CodespaceStore
      const codespaceStore = this.env.CODESPACE_STORE.get(
        this.env.CODESPACE_STORE.idFromName("global"),
      );

      const credentialsResponse = await codespaceStore.fetch(
        `http://do/internal/codespace/${githubUser}/${codespaceName}`,
      );

      if (!credentialsResponse.ok) {
        console.error(
          `❌ Failed to fetch credentials for ${codespaceName}: ${credentialsResponse.status}`,
        );
        return;
      }

      const credentials = await credentialsResponse.json<{
        githubToken: string;
        codespaceName: string;
        githubUser: string;
      }>();

      // Check if we got a valid token
      if (!credentials.githubToken) {
        console.error(
          `❌ No valid token available for ${codespaceName} (may be expired)`,
        );
        return;
      }

      // Get a container instance to handle this ping
      const container = getContainer(this.env.KEEPALIVE_CONTAINER, "keepalive");

      // Call the container's /ping endpoint with the fetched token
      const response = await container.fetch(
        `http://container/ping/${codespaceName}`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            githubToken: credentials.githubToken,
          }),
        },
      );

      const result = await response.json();

      if (response.ok) {
        console.log(`✅ Keep-alive successful for ${codespaceName}`);
        console.log(`   Container output:`, result);

        // Update last ping time
        this.sql.exec(
          "UPDATE codespace_activity SET last_ping_time = ? WHERE codespace_name = ?",
          Date.now(),
          codespaceName,
        );
      } else {
        console.error(`❌ Keep-alive failed for ${codespaceName}:`, result);
        // Don't update last_ping_time on failure - will retry on next alarm
      }
    } catch (error) {
      console.error(`❌ Error pinging ${codespaceName}:`, error);
      // Don't update last_ping_time on error - will retry on next alarm
    }
  }

  /**
   * Handle requests from the worker
   */
  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);

    // POST /activity - Update activity for a codespace
    // Note: This endpoint does NOT accept or store tokens. Tokens are stored encrypted in CodespaceStore.
    if (url.pathname === "/activity" && request.method === "POST") {
      const body = await request.json<{
        codespaceName: string;
        githubUser: string;
      }>();

      const { codespaceName, githubUser } = body;

      if (!codespaceName || !githubUser) {
        return Response.json(
          {
            error: "Missing required fields: codespaceName, githubUser",
          },
          { status: 400 },
        );
      }

      const now = Date.now();

      // Check if codespace already exists
      const existing = this.sql
        .exec(
          "SELECT last_ping_time FROM codespace_activity WHERE codespace_name = ?",
          codespaceName,
        )
        .toArray();

      if (existing.length > 0) {
        // Update existing entry - preserve last_ping_time
        this.sql.exec(
          "UPDATE codespace_activity SET github_user = ?, last_activity_time = ? WHERE codespace_name = ?",
          githubUser,
          now,
          codespaceName,
        );
      } else {
        // Insert new entry - initialize last_ping_time to 0 so it pings soon
        this.sql.exec(
          "INSERT INTO codespace_activity (codespace_name, github_user, last_activity_time, last_ping_time) VALUES (?, ?, ?, ?)",
          codespaceName,
          githubUser,
          now,
          0, // Initialize to 0 so first ping happens quickly
        );

        console.log(`🫀 New codespace tracked: ${codespaceName}`);
      }

      // Ensure alarm is set
      // NOTE: There's a potential race condition where multiple concurrent /activity
      // requests could both see no alarm and attempt to schedule one. This is safe because:
      // 1. setAlarm() is idempotent - setting the same alarm multiple times is harmless
      // 2. Only one alarm fires at the scheduled time regardless of how many times it's set
      // 3. The impact is minimal - worst case is redundant setAlarm() calls, not multiple executions
      // 4. The check-then-set pattern here is sufficient for this use case
      const currentAlarm = await this.ctx.storage.getAlarm();
      if (!currentAlarm) {
        await this.ctx.storage.setAlarm(Date.now() + ALARM_INTERVAL_MS);
        console.log(
          `🫀 Alarm scheduled for ${ALARM_INTERVAL_MS / 1000} seconds`,
        );
      }

      return Response.json({ success: true });
    }

    // GET /status - Get current state (for debugging)
    // NOTE: This endpoint is only accessible from within the Worker (Durable Objects
    // cannot be called externally), so no additional auth is needed.
    if (url.pathname === "/status" && request.method === "GET") {
      const rows = this.sql
        .exec(
          "SELECT * FROM codespace_activity ORDER BY last_activity_time DESC",
        )
        .toArray();

      const codespaces = rows.map((row) => ({
        codespaceName: row.codespace_name,
        githubUser: row.github_user,
        lastActivityTime: row.last_activity_time,
        lastPingTime: row.last_ping_time,
        lastActivityAgo: Math.floor(
          (Date.now() - (row.last_activity_time as number)) / 1000,
        ),
        lastPingAgo: Math.floor(
          (Date.now() - (row.last_ping_time as number)) / 1000,
        ),
      }));

      const alarm = await this.ctx.storage.getAlarm();

      return Response.json({
        codespaces,
        alarmScheduled: alarm ? new Date(alarm).toISOString() : null,
        now: new Date().toISOString(),
      });
    }

    return Response.json({ error: "Not found" }, { status: 404 });
  }
}
