import { DurableObject } from "cloudflare:workers";
import { getContainer } from "@cloudflare/containers";
import type { KeepAliveContainer } from "./index";

/**
 * KeepAliveCoordinator
 *
 * Tracks active codespaces and coordinates keep-alive pings.
 *
 * Responsibilities:
 * - Track last activity time per codespace
 * - Use Durable Object alarms to check every 60 seconds
 * - Invoke KeepAliveContainer to ping codespaces every 5 minutes when active
 * - Stop pinging after 10 minutes of inactivity
 */

interface CodespaceActivity {
  codespaceName: string;
  githubUser: string;
  githubToken: string;
  lastActivityTime: number;
  lastPingTime: number;
}

export class KeepAliveCoordinator extends DurableObject<{
  KEEPALIVE_CONTAINER: DurableObjectNamespace<KeepAliveContainer>;
}> {
  private sql: SqlStorage;

  constructor(
    ctx: DurableObjectState,
    env: { KEEPALIVE_CONTAINER: DurableObjectNamespace<KeepAliveContainer> },
  ) {
    super(ctx, env);
    this.sql = ctx.storage.sql;

    // Initialize database schema
    this.sql.exec(`
      CREATE TABLE IF NOT EXISTS codespace_activity (
        codespace_name TEXT PRIMARY KEY,
        github_user TEXT NOT NULL,
        github_token TEXT NOT NULL,
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
    const tenMinutesAgo = now - 10 * 60 * 1000;
    const fiveMinutesAgo = now - 5 * 60 * 1000;

    console.log("🫀 KeepAlive alarm checking active codespaces");

    // Get all active codespaces (activity within last 10 minutes)
    const rows = this.sql
      .exec(
        "SELECT * FROM codespace_activity WHERE last_activity_time > ?",
        tenMinutesAgo,
      )
      .toArray();

    console.log(`🫀 Found ${rows.length} active codespace(s)`);

    // Check each active codespace
    for (const row of rows) {
      const activity: CodespaceActivity = {
        codespaceName: row.codespace_name as string,
        githubUser: row.github_user as string,
        githubToken: row.github_token as string,
        lastActivityTime: row.last_activity_time as number,
        lastPingTime: row.last_ping_time as number,
      };

      // Only ping if:
      // 1. Last activity is within 10 minutes (already filtered by query)
      // 2. Last ping was more than 5 minutes ago
      if (activity.lastPingTime < fiveMinutesAgo) {
        console.log(`🫀 Sending keep-alive ping for ${activity.codespaceName}`);
        await this.pingCodespace(activity);
      } else {
        const nextPingIn = Math.ceil(
          (activity.lastPingTime + 5 * 60 * 1000 - now) / 1000,
        );
        console.log(
          `🫀 Skipping ${activity.codespaceName}, last ping was ${Math.floor((now - activity.lastPingTime) / 1000)}s ago (next ping in ${nextPingIn}s)`,
        );
      }
    }

    // Clean up inactive codespaces (no activity in last 10 minutes)
    const result = this.sql.exec(
      "DELETE FROM codespace_activity WHERE last_activity_time <= ?",
      tenMinutesAgo,
    );

    if (result.rowsWritten > 0) {
      console.log(`🫀 Cleaned up ${result.rowsWritten} inactive codespace(s)`);
    }

    // Schedule next alarm in 60 seconds
    await this.ctx.storage.setAlarm(Date.now() + 60 * 1000);
  }

  /**
   * Ping a codespace using the KeepAliveContainer
   */
  private async pingCodespace(activity: CodespaceActivity): Promise<void> {
    try {
      // Get a container instance to handle this ping
      const container = getContainer(this.env.KEEPALIVE_CONTAINER, "keepalive");

      // Call the container's /ping endpoint
      const response = await container.fetch(
        `http://container/ping/${activity.codespaceName}`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            githubToken: activity.githubToken,
          }),
        },
      );

      const result = await response.json();

      if (response.ok) {
        console.log(`✅ Keep-alive successful for ${activity.codespaceName}`);
        console.log(`   Container output:`, result);

        // Update last ping time
        this.sql.exec(
          "UPDATE codespace_activity SET last_ping_time = ? WHERE codespace_name = ?",
          Date.now(),
          activity.codespaceName,
        );
      } else {
        console.error(
          `❌ Keep-alive failed for ${activity.codespaceName}:`,
          result,
        );
        // Don't update last_ping_time on failure - will retry on next alarm
      }
    } catch (error) {
      console.error(`❌ Error pinging ${activity.codespaceName}:`, error);
      // Don't update last_ping_time on error - will retry on next alarm
    }
  }

  /**
   * Handle requests from the worker
   */
  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);

    // POST /activity - Update activity for a codespace
    if (url.pathname === "/activity" && request.method === "POST") {
      const body = await request.json<{
        codespaceName: string;
        githubUser: string;
        githubToken: string;
      }>();

      const { codespaceName, githubUser, githubToken } = body;

      if (!codespaceName || !githubUser || !githubToken) {
        return Response.json(
          {
            error:
              "Missing required fields: codespaceName, githubUser, githubToken",
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
          "UPDATE codespace_activity SET github_user = ?, github_token = ?, last_activity_time = ? WHERE codespace_name = ?",
          githubUser,
          githubToken,
          now,
          codespaceName,
        );
      } else {
        // Insert new entry - initialize last_ping_time to 0 so it pings soon
        this.sql.exec(
          "INSERT INTO codespace_activity (codespace_name, github_user, github_token, last_activity_time, last_ping_time) VALUES (?, ?, ?, ?, ?)",
          codespaceName,
          githubUser,
          githubToken,
          now,
          0, // Initialize to 0 so first ping happens quickly
        );

        console.log(`🫀 New codespace tracked: ${codespaceName}`);
      }

      // Ensure alarm is set
      const currentAlarm = await this.ctx.storage.getAlarm();
      if (!currentAlarm) {
        await this.ctx.storage.setAlarm(Date.now() + 60 * 1000);
        console.log("🫀 Alarm scheduled for 60 seconds");
      }

      return Response.json({ success: true });
    }

    // GET /status - Get current state (for debugging)
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
