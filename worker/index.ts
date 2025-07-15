import { Hono, Context } from "hono";
import { cors } from "hono/cors";
import { getSignedCookie, setSignedCookie, setCookie } from "hono/cookie";
import { HTTPException } from "hono/http-exception";
import { githubAuth } from "@hono/oauth-providers/github";
import { Container, getContainer } from "@cloudflare/containers";
import { Webhooks } from "@octokit/webhooks";
import { App } from "@octokit/app";

// Durable Object for container management
export class CatnipContainer extends Container {
  defaultPort = 8080;
  sleepAfter = "10m";
  environmentVariables = {
    CATNIP_PROXY: "http://localhost:8787",
  };
}

export interface Env {
  CATNIP_CONTAINER: DurableObjectNamespace<CatnipContainer>;
  ASSETS: Fetcher;
  SESSIONS: DurableObjectNamespace;
  GITHUB_CLIENT_ID: string;
  GITHUB_CLIENT_SECRET: string;
  GITHUB_APP_ID?: string;
  GITHUB_APP_PRIVATE_KEY?: string;
  GITHUB_WEBHOOK_SECRET: string;
  CATNIP_ENCRYPTION_KEY: string;
  ENVIRONMENT?: string;
}

interface SessionData {
  userId: string;
  username: string;
  accessToken: string;
  refreshToken?: string;
  expiresAt: number;
  refreshTokenExpiresAt?: number;
}

type Variables = {
  userId: string;
  username: string;
  accessToken: string;
  sessionId: string;
  session: SessionData;
};

type HonoEnv = {
  Bindings: Env;
  Variables: Variables;
};

// Define container route patterns
const CONTAINER_ROUTES = [
  /^\/v1\//, // API routes
  /^\/.*\.git/, // Git repositories
  /^\/\d+\//, // Port forwarding (e.g., /3000/)
];

function shouldRouteToContainer(pathname: string): boolean {
  return CONTAINER_ROUTES.some((pattern) => pattern.test(pathname));
}

// Helper function to get GitHub App installation token with caching
async function getGitHubAppToken(env: Env, ctx: ExecutionContext): Promise<string | null> {
  if (!env.GITHUB_APP_ID || !env.GITHUB_APP_PRIVATE_KEY) {
    return null;
  }

  const cacheKey = `github-app-token-${env.GITHUB_APP_ID}-wandb`;
  const now = Date.now();

  // Check Cloudflare cache first
  const cache = caches.default;
  const cacheRequest = new Request(`https://cache.internal/${cacheKey}`);
  const cachedResponse = await cache.match(cacheRequest);
  
  if (cachedResponse) {
    const cachedData = await cachedResponse.json() as { token: string; expiresAt: number };
    if (now < cachedData.expiresAt) {
      return cachedData.token;
    }
  }

  try {
    const app = new App({
      appId: env.GITHUB_APP_ID,
      privateKey: env.GITHUB_APP_PRIVATE_KEY,
    });

    // Get the installation for the wandb organization
    const installations = await app.octokit.request('GET /app/installations');
    const installation = installations.data.find(
      (inst: any) => inst.account?.login === "wandb"
    );

    if (!installation) {
      console.error("No GitHub App installation found for wandb organization");
      return null;
    }

    // Get installation token
    const installationToken = await app.octokit.request('POST /app/installations/{installation_id}/access_tokens', {
      installation_id: installation.id,
    });

    // Cache the token for 50 minutes (tokens expire after 1 hour)
    const expiresAt = now + (50 * 60 * 1000);
    const tokenData = {
      token: installationToken.data.token,
      expiresAt,
    };

    // Store in Cloudflare cache
    const cacheResponse = new Response(JSON.stringify(tokenData), {
      headers: {
        'Content-Type': 'application/json',
        'Cache-Control': 'max-age=3000', // 50 minutes
      },
    });
    
    ctx.waitUntil(cache.put(cacheRequest, cacheResponse));

    return installationToken.data.token;
  } catch (error) {
    console.error("Failed to get GitHub App installation token:", error);
    return null;
  }
}

// Factory function to create app with environment bindings
export function createApp(env: Env) {
  const app = new Hono<HonoEnv>();

  // CORS for API routes
  app.use("/v1/*", cors());

  // Session middleware - load session from signed cookie
  app.use("*", async (c, next) => {
    // Skip session loading if no encryption key
    if (!c.env.CATNIP_ENCRYPTION_KEY) {
      console.warn(
        "CATNIP_ENCRYPTION_KEY not set, skipping session middleware"
      );
      return next();
    }

    try {
      const sessionId = await getSignedCookie(
        c,
        c.env.CATNIP_ENCRYPTION_KEY,
        "catnip-session"
      );
      if (sessionId) {
        try {
          // Get session from Durable Object
          const sessionDO = c.env.SESSIONS.get(
            c.env.SESSIONS.idFromName("global")
          );
          const response = await sessionDO.fetch(
            `https://internal/session/${sessionId}`
          );

          if (response.ok) {
            const session = await response.json<SessionData>();
            c.set("session", session);
            c.set("sessionId", sessionId);
            c.set("userId", session.userId);
            c.set("username", session.username);
            c.set("accessToken", session.accessToken);
          }
        } catch (error) {
          console.error("Failed to load session from DO:", error);
        }
      }
    } catch (error) {
      console.error("Failed to get signed cookie:", error);
    }

    await next();
  });

  // GitHub OAuth - handles both login initiation and callback
  app.use(
    "/v1/auth/github",
    githubAuth({
      client_id: env.GITHUB_CLIENT_ID,
      client_secret: env.GITHUB_CLIENT_SECRET,
      scope: ["read:user", "user:email", "repo"],
      oauthApp: !env.GITHUB_APP_ID, // Use OAuth App mode if no GitHub App ID is set
    })
  );

  // After OAuth completes
  app.get("/v1/auth/github", async (c) => {
    // Check for OAuth errors first
    const error = c.req.query("error");
    const errorDescription = c.req.query("error_description");

    if (error) {
      console.error("OAuth error:", { error, errorDescription });
      return c.text(`Authentication failed: ${errorDescription || error}`, 400);
    }

    const tokenData = c.get("token") as
      | { token: string; expires_in?: number }
      | undefined;
    const refreshTokenData = c.get("refresh-token") as
      | { token: string; expires_in: number }
      | undefined;
    const user = c.get("user-github");
    const grantedScopes = c.get("granted-scopes");

    if (!tokenData || !user) {
      console.error("Missing token or user data after OAuth", {
        hasToken: !!tokenData,
        hasUser: !!user,
        userKeys: user ? Object.keys(user) : [],
      });
      return c.text("Authentication failed: Missing required data", 400);
    }

    // Log token information for debugging
    console.log("OAuth completed:", {
      userId: user.id,
      username: user.login,
      hasRefreshToken: !!refreshTokenData,
      tokenExpiresIn: tokenData.expires_in,
      refreshTokenExpiresIn: refreshTokenData?.expires_in,
      grantedScopes,
      isGitHubApp: !!c.env.GITHUB_APP_ID,
      oauthAppMode: !c.env.GITHUB_APP_ID,
      userEmail: user.email,
    });

    // Create session in Durable Object
    const sessionId = crypto.randomUUID();
    const sessionDO = c.env.SESSIONS.get(c.env.SESSIONS.idFromName("global"));

    const now = Date.now();
    const isGitHubApp = !!c.env.GITHUB_APP_ID;

    // Calculate token expiration using actual values from OAuth response
    let expiresAt: number;
    if (tokenData.expires_in) {
      // expires_in is in seconds, convert to milliseconds
      expiresAt = now + tokenData.expires_in * 1000;
    } else if (isGitHubApp) {
      // Default to 8 hours for GitHub App if no expires_in provided
      expiresAt = now + 8 * 60 * 60 * 1000;
    } else {
      // Default to 90 days for OAuth App without expiry
      expiresAt = now + 90 * 24 * 60 * 60 * 1000;
    }

    // Calculate refresh token expiration if available
    let refreshTokenExpiresAt: number | undefined;
    if (refreshTokenData?.expires_in) {
      // expires_in is in seconds, convert to milliseconds
      refreshTokenExpiresAt = now + refreshTokenData.expires_in * 1000;
    }

    const sessionData: SessionData = {
      userId: user.id?.toString() || "unknown",
      username: user.login || "unknown",
      accessToken: tokenData.token,
      refreshToken: refreshTokenData?.token,
      expiresAt,
      refreshTokenExpiresAt,
    };

    await sessionDO.fetch(`https://internal/session/${sessionId}`, {
      method: "PUT",
      body: JSON.stringify(sessionData),
    });

    // Set signed cookie with just session ID
    if (!c.env.CATNIP_ENCRYPTION_KEY) {
      console.error(
        "Cannot set signed cookie: CATNIP_ENCRYPTION_KEY not configured"
      );
      return c.text("Server configuration error", 500);
    }

    await setSignedCookie(
      c,
      "catnip-session",
      sessionId,
      c.env.CATNIP_ENCRYPTION_KEY,
      {
        httpOnly: true,
        secure: true,
        sameSite: "Lax",
        maxAge: 30 * 24 * 60 * 60, // 30 days - longer than token expiry to support refresh
        path: "/",
      }
    );

    // Check for return URL
    const returnTo = c.req.query("return_to");
    if (returnTo && returnTo.startsWith("/")) {
      return c.redirect(returnTo);
    }

    return c.redirect("/");
  });

  // Logout endpoint
  app.get("/v1/auth/logout", async (c) => {
    const sessionId = c.get("sessionId");

    if (sessionId) {
      // Delete session from DO
      const sessionDO = c.env.SESSIONS.get(c.env.SESSIONS.idFromName("global"));
      await sessionDO.fetch(`https://internal/session/${sessionId}`, {
        method: "DELETE",
      });
    }

    // Clear cookie
    setCookie(c, "catnip-session", "", {
      httpOnly: true,
      secure: true,
      sameSite: "Lax",
      maxAge: 0,
      path: "/",
    });

    return c.redirect("/");
  });

  // Auth status endpoint
  app.get("/v1/auth/status", async (c) => {
    const session = c.get("session");
    const sessionId = c.get("sessionId");

    if (!session || !sessionId) {
      return c.json({ authenticated: false });
    }

    // Session already loaded by middleware, just check expiry
    if (Date.now() > session.expiresAt) {
      return c.json({ authenticated: false });
    }

    return c.json({
      authenticated: true,
      userId: session.userId,
      username: session.username,
    });
  });

  // Settings endpoint - bypasses auth to expose configuration
  app.get("/v1/settings", (c) => {
    return c.json({
      catnipProxy: "http://localhost:8787",
      authRequired: true,
    });
  });

  // Debug endpoint to check environment variables
  app.get("/v1/debug/env", (c) => {
    return c.json({
      hasGithubAppId: !!c.env.GITHUB_APP_ID,
      hasGithubAppPrivateKey: !!c.env.GITHUB_APP_PRIVATE_KEY,
      githubAppId: c.env.GITHUB_APP_ID,
      environment: c.env.ENVIRONMENT || "unknown",
    });
  });

  // GitHub App webhook endpoint
  app.post("/v1/github/webhooks", async (c) => {
    const signature = c.req.header("x-hub-signature-256");
    const body = await c.req.text();

    if (!signature) {
      return c.text("Missing signature", 401);
    }

    try {
      const webhooks = new Webhooks({
        secret: c.env.GITHUB_WEBHOOK_SECRET,
      });

      // Verify webhook signature
      const verified = await webhooks.verify(body, signature);
      if (!verified) {
        return c.text("Invalid signature", 401);
      }

      // Parse the event
      const event = JSON.parse(body);
      const eventName = c.req.header("x-github-event") || "unknown";

      // Log webhook event
      console.log(`Received GitHub webhook: ${eventName}`, event.action);

      // TODO: Handle specific webhook events
      return c.json({ received: true });
    } catch (error) {
      console.error("Webhook error:", error);
      return c.text("Webhook processing failed", 500);
    }
  });

  // GitHub App manifest endpoint
  app.get("/v1/github/app-manifest", (c) => {
    const baseUrl = new URL(c.req.url).origin;

    return c.json({
      name: "Catnip",
      url: baseUrl,
      hook_attributes: {
        url: `${baseUrl}/v1/github/webhooks`,
      },
      redirect_url: `${baseUrl}/v1/auth/github`,
      callback_urls: [`${baseUrl}/v1/auth/github`],
      setup_url: `${baseUrl}/v1/github/setup`,
      description: "Agentic coding made fun and productive",
      public: false,
      default_permissions: {
        contents: "write",
        issues: "write",
        pull_requests: "write",
        actions: "write",
        administration: "read",
      },
      default_events: [
        "push",
        "pull_request",
        "issues",
        "issue_comment",
        "pull_request_review",
        "pull_request_review_comment",
      ],
    });
  });

  // GitHub release proxy endpoints for install.sh
  app.get("/v1/github/releases/latest", async (c) => {
    const token = await getGitHubAppToken(c.env, c.executionCtx);
    if (!token) {
      return c.text("GitHub App authentication not configured", 500);
    }

    try {
      const response = await fetch("https://api.github.com/repos/wandb/catnip/releases/latest", {
        headers: {
          "Authorization": `token ${token}`,
          "Accept": "application/vnd.github.v3+json",
          "User-Agent": "catnip-proxy/1.0",
        },
      });

      if (!response.ok) {
        return c.text("Failed to fetch latest release", 500);
      }

      const data = await response.json();
      return c.json(data as any);
    } catch (error) {
      console.error("Error fetching latest release:", error);
      return c.text("Internal server error", 500);
    }
  });

  app.get("/v1/github/releases/download/:version/:filename", async (c) => {
    const token = await getGitHubAppToken(c.env, c.executionCtx);
    if (!token) {
      return c.text("GitHub App authentication not configured", 500);
    }

    const version = c.req.param("version");
    const filename = c.req.param("filename");

    if (!version || !filename) {
      return c.text("Version and filename are required", 400);
    }

    try {
      // Get the release info first
      const releaseResponse = await fetch(`https://api.github.com/repos/wandb/catnip/releases/tags/${version}`, {
        headers: {
          "Authorization": `token ${token}`,
          "Accept": "application/vnd.github.v3+json",
          "User-Agent": "catnip-proxy/1.0",
        },
      });

      if (!releaseResponse.ok) {
        return c.text("Failed to get release info", 500);
      }

      const releaseData = await releaseResponse.json() as any;
      
      // Find the asset with the matching filename
      const asset = releaseData.assets.find((asset: any) => asset.name === filename);
      if (!asset) {
        return c.text("Asset not found", 404);
      }

      // Determine content type based on file extension
      const isTextFile = filename.endsWith('.txt') || filename.endsWith('.md') || filename.endsWith('.json');
      const contentType = isTextFile ? 'text/plain' : 'application/octet-stream';

      // Download the asset using the GitHub API with the same token
      // GitHub API always requires application/octet-stream for asset downloads
      const assetResponse = await fetch(`https://api.github.com/repos/wandb/catnip/releases/assets/${asset.id}`, {
        headers: {
          "Authorization": `Bearer ${token}`,
          "Accept": "application/octet-stream",
          "User-Agent": "catnip-proxy/1.0",
          "X-GitHub-Api-Version": "2022-11-28",
        },
      });

      if (!assetResponse.ok) {
        return c.text("Failed to download release asset", 500);
      }

      // Return the asset data as a stream
      return new Response(assetResponse.body, {
        status: 200,
        headers: {
          "Content-Type": contentType,
          "Content-Length": assetResponse.headers.get("Content-Length") || "",
          "Cache-Control": "public, max-age=3600", // Cache for 1 hour
        },
      });
    } catch (error) {
      console.error("Error downloading release asset:", error);
      return c.text("Internal server error", 500);
    }
  });

  // Authentication middleware for protected routes
  async function requireAuth(c: Context<HonoEnv>, next: () => Promise<void>) {
    const session = c.get("session");

    if (!session) {
      // Check Authorization header for API access
      const authHeader = c.req.header("Authorization");
      if (authHeader?.startsWith("Bearer ")) {
        const token = authHeader.substring(7);
        // TODO: Validate GitHub token
        c.set("accessToken", token);
        return next();
      }

      throw new HTTPException(401, { message: "Authentication required" });
    }

    // Check if expired
    if (Date.now() > session.expiresAt) {
      throw new HTTPException(401, { message: "Session expired" });
    }

    // Access token already loaded by middleware
    return next();
  }

  // Protected container routes
  app.use("*", async (c, next) => {
    const pathname = new URL(c.req.url).pathname;

    // Skip auth for non-container routes
    if (!shouldRouteToContainer(pathname)) {
      return next();
    }

    // Skip auth for auth endpoints and settings
    if (pathname.startsWith("/v1/auth/") || pathname === "/v1/settings") {
      return next();
    }

    // Require authentication
    return requireAuth(c, next);
  });

  // Handle container routes
  app.all("*", async (c) => {
    const url = new URL(c.req.url);
    const userAgent = c.req.header("User-Agent") || "";
    // Check if this is curl or wget requesting the root path
    if (
      url.pathname === "/" &&
      (userAgent.toLowerCase().includes("curl") ||
        userAgent.toLowerCase().includes("wget"))
    ) {
      // Serve the install script
      try {
        const installScriptUrl = new URL("/install.sh", c.req.url);
        const response = await c.env.ASSETS.fetch(
          new Request(installScriptUrl, {
            method: "GET",
            headers: c.req.raw.headers,
          })
        );

        if (response.ok) {
          // Return the install script with proper content type
          return new Response(response.body, {
            status: response.status,
            headers: {
              "Content-Type": "text/plain; charset=utf-8",
              "Cache-Control": "public, max-age=300", // Cache for 5 minutes
            },
          });
        }
      } catch (e) {
        console.error("Failed to serve install script:", e);
      }
    }

    // Check if this should route to container
    if (shouldRouteToContainer(url.pathname)) {
      const userId = c.get("userId") || "default";
      const container = await getContainer(c.env.CATNIP_CONTAINER, userId);
      const containerUrl = new URL(c.req.url);
      containerUrl.host = `container:8080`;
      return container.fetch(
        new Request(containerUrl, {
          method: c.req.method,
          headers: c.req.raw.headers,
          body: c.req.raw.body,
        })
      );
    }

    // All other requests go to static assets
    try {
      return await c.env.ASSETS.fetch(c.req.raw);
    } catch (e) {
      // If ASSETS binding is not available in development
      return c.text("Static asset serving not configured", 503);
    }
  });

  return app;
}

// Default export that creates app on demand
export default {
  fetch(request: Request, env: Env, ctx: ExecutionContext) {
    return createApp(env).fetch(request, env, ctx);
  },
} satisfies ExportedHandler<Env>;

// Export Durable Objects
export { SessionStore } from "./sessions";
