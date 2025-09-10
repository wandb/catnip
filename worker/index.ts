import { Hono, Context } from "hono";
import { cors } from "hono/cors";
import { getSignedCookie, setSignedCookie, setCookie } from "hono/cookie";
import { HTTPException } from "hono/http-exception";
import { githubAuth } from "@hono/oauth-providers/github";
import { Container, getContainer } from "@cloudflare/containers";
import { Webhooks } from "@octokit/webhooks";

// Durable Object for container management
export class CatnipContainer extends Container {
  defaultPort = 6369;
  sleepAfter = "10m";
  environmentVariables = {
    CATNIP_PROXY: "https://catnip.run",
  };
}

export interface Env {
  CATNIP_CONTAINER: DurableObjectNamespace<CatnipContainer>;
  ASSETS: Fetcher;
  SESSIONS: DurableObjectNamespace;
  CODESPACE_STORE: DurableObjectNamespace;
  CATNIP_ASSETS: R2Bucket;
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

interface CodespaceCredentials {
  githubToken: string;
  githubUser: string;
  codespaceName: string;
  createdAt: number;
  updatedAt: number;
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
        "CATNIP_ENCRYPTION_KEY not set, skipping session middleware",
      );
      return next();
    }

    try {
      const sessionId = await getSignedCookie(
        c,
        c.env.CATNIP_ENCRYPTION_KEY,
        "catnip-session",
      );
      if (sessionId) {
        try {
          // Get session from Durable Object
          const sessionDO = c.env.SESSIONS.get(
            c.env.SESSIONS.idFromName("global"),
          );
          const response = await sessionDO.fetch(
            `https://internal/session/${sessionId}`,
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
      scope: ["read:user", "user:email", "repo", "codespace"],
      oauthApp: !env.GITHUB_APP_ID, // Use OAuth App mode if no GitHub App ID is set
    }),
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
        "Cannot set signed cookie: CATNIP_ENCRYPTION_KEY not configured",
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
      },
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

      // Release events are now handled by GitHub Actions uploading to R2

      return c.json({ received: true });
    } catch (error) {
      console.error("Webhook error:", error);
      return c.text("Webhook processing failed", 500);
    }
  });

  // GitHub Codespace credentials endpoint
  app.post("/v1/auth/github/codespace", async (c) => {
    try {
      const body = await c.req.json();
      const { GITHUB_TOKEN, GITHUB_USER, CODESPACE_NAME } = body;

      if (!GITHUB_TOKEN || !GITHUB_USER || !CODESPACE_NAME) {
        return c.json(
          {
            error:
              "Missing required fields: GITHUB_TOKEN, GITHUB_USER, CODESPACE_NAME",
          },
          400,
        );
      }

      // Validate token belongs to user by checking GitHub API
      try {
        const validateResponse = await fetch("https://api.github.com/user", {
          headers: {
            Authorization: `Bearer ${GITHUB_TOKEN}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        });

        if (!validateResponse.ok) {
          console.error(
            "GitHub token validation failed:",
            validateResponse.status,
            await validateResponse.text(),
          );
          return c.json({ error: "Invalid GitHub token" }, 401);
        }

        const userData = (await validateResponse.json()) as { login: string };
        if (userData.login !== GITHUB_USER) {
          console.error("Token user mismatch:", {
            expected: GITHUB_USER,
            actual: userData.login,
          });
          return c.json(
            { error: "GitHub token does not belong to specified user" },
            403,
          );
        }
      } catch (error) {
        console.error("Error validating GitHub token:", error);
        return c.json({ error: "Failed to validate GitHub token" }, 500);
      }

      // Store credentials in Durable Object
      const codespaceStore = c.env.CODESPACE_STORE.get(
        c.env.CODESPACE_STORE.idFromName("global"),
      );

      const credentials: CodespaceCredentials = {
        githubToken: GITHUB_TOKEN,
        githubUser: GITHUB_USER,
        codespaceName: CODESPACE_NAME,
        createdAt: Date.now(),
        updatedAt: Date.now(),
      };

      const storeResponse = await codespaceStore.fetch(
        `https://internal/codespace/${GITHUB_USER}`,
        {
          method: "PUT",
          body: JSON.stringify(credentials),
        },
      );

      if (!storeResponse.ok) {
        console.error("Failed to store codespace credentials");
        return c.json({ error: "Failed to store credentials" }, 500);
      }

      return c.json({
        success: true,
        message: "Codespace credentials stored successfully",
      });
    } catch (error) {
      console.error("Codespace endpoint error:", error);
      return c.json({ error: "Internal server error" }, 500);
    }
  });

  // Get codespace credentials endpoint
  app.get("/v1/auth/github/codespace/:user", async (c) => {
    const user = c.req.param("user");

    if (!user) {
      return c.json({ error: "User parameter required" }, 400);
    }

    try {
      const codespaceStore = c.env.CODESPACE_STORE.get(
        c.env.CODESPACE_STORE.idFromName("global"),
      );

      const response = await codespaceStore.fetch(
        `https://internal/codespace/${user}`,
      );

      if (!response.ok) {
        if (response.status === 404) {
          return c.json(
            { error: "No codespace credentials found for user" },
            404,
          );
        }
        return c.json({ error: "Failed to retrieve credentials" }, 500);
      }

      const credentials = await response.json();
      return c.json(credentials as Record<string, unknown>);
    } catch (error) {
      console.error("Error retrieving codespace credentials:", error);
      return c.json({ error: "Internal server error" }, 500);
    }
  });

  // Codespace access endpoint - requires authentication and redirects to codespace
  app.get("/v1/codespace", requireAuth, async (c) => {
    try {
      const username = c.get("username");
      const accessToken = c.get("accessToken");

      if (!username || !accessToken) {
        return c.json({ error: "No authenticated user or access token" }, 401);
      }

      // Get codespace credentials for the authenticated user (to get the codespace name)
      const codespaceStore = c.env.CODESPACE_STORE.get(
        c.env.CODESPACE_STORE.idFromName("global"),
      );

      const response = await codespaceStore.fetch(
        `https://internal/codespace/${username}`,
      );

      if (!response.ok) {
        if (response.status === 404) {
          return c.json(
            {
              error:
                "No codespace found. Please set up a codespace first by running catnip in a GitHub Codespace.",
            },
            404,
          );
        }
        return c.json(
          { error: "Failed to retrieve codespace information" },
          500,
        );
      }

      const credentials = (await response.json()) as CodespaceCredentials;

      // Use authenticated user's OAuth token for GitHub API calls instead of stored token
      const githubResponse = await fetch(
        `https://api.github.com/user/codespaces`,
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        },
      );

      if (!githubResponse.ok) {
        console.error(
          "GitHub API error:",
          githubResponse.status,
          await githubResponse.text(),
        );
        return c.json(
          {
            error:
              "Failed to access GitHub API. Please ensure you have codespace permissions.",
          },
          500,
        );
      }

      const codespaces = (await githubResponse.json()) as Array<{
        name: string;
        state: string;
        web_url: string;
      }>;

      // Find the matching codespace
      const targetCodespace = codespaces.find(
        (cs) => cs.name === credentials.codespaceName,
      );

      if (!targetCodespace) {
        return c.json(
          {
            error: `Codespace ${credentials.codespaceName} not found in your GitHub account`,
          },
          404,
        );
      }

      // If codespace is not running, start it using the user's OAuth token
      if (targetCodespace.state !== "Available") {
        console.log(
          `Starting codespace ${credentials.codespaceName} (current state: ${targetCodespace.state})`,
        );

        const startResponse = await fetch(
          `https://api.github.com/user/codespaces/${credentials.codespaceName}/start`,
          {
            method: "POST",
            headers: {
              Authorization: `Bearer ${accessToken}`,
              Accept: "application/vnd.github.v3+json",
              "User-Agent": "Catnip-Worker/1.0",
            },
          },
        );

        if (!startResponse.ok) {
          console.error(
            "Failed to start codespace:",
            startResponse.status,
            await startResponse.text(),
          );
          return c.json(
            {
              error:
                "Failed to start codespace. Please ensure you have codespace permissions.",
            },
            500,
          );
        }
      }

      // Redirect to the codespace URL with port 6369
      const codespaceUrl = `https://${credentials.codespaceName}-6369.app.github.dev`;
      return c.redirect(codespaceUrl);
    } catch (error) {
      console.error("Codespace access error:", error);
      return c.json({ error: "Internal server error" }, 500);
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
        codespaces: "write",
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

  // Release metadata endpoint - serves release info from R2
  app.get("/v1/github/releases/latest", async (c) => {
    try {
      const releaseObject = await c.env.CATNIP_ASSETS.get(
        "releases/latest.json",
      );

      if (!releaseObject) {
        return c.text("Latest release not found", 404);
      }

      const releaseText = await releaseObject.text();
      const releaseData = JSON.parse(releaseText);
      return c.json(releaseData);
    } catch (error) {
      console.error("Error fetching latest release from R2:", error);
      return c.text("Internal server error", 500);
    }
  });

  app.get("/v1/github/releases/download/:version/:filename", async (c) => {
    const version = c.req.param("version");
    const filename = c.req.param("filename");

    console.log(`Download request: version=${version}, filename=${filename}`);

    if (!version || !filename) {
      console.error("Missing version or filename", { version, filename });
      return c.text("Version and filename are required", 400);
    }

    try {
      // Get the asset from R2
      const assetKey = `releases/${version}/${filename}`;
      const assetObject = await c.env.CATNIP_ASSETS.get(assetKey);

      if (!assetObject) {
        console.error(`Asset not found in R2: ${assetKey}`);
        return c.text("Asset not found", 404);
      }

      // Determine content type based on file extension
      const isTextFile =
        filename.endsWith(".txt") ||
        filename.endsWith(".md") ||
        filename.endsWith(".json");
      const contentType = isTextFile
        ? "text/plain"
        : "application/octet-stream";

      // Return the asset data with proper caching headers
      return new Response(assetObject.body, {
        status: 200,
        headers: {
          "Content-Type": contentType,
          "Cache-Control": "public, max-age=3600, s-maxage=3600", // Cache for 1 hour in CDN too
          "Content-Disposition": `attachment; filename="${filename}"`,
          ETag: assetObject.etag,
        },
      });
    } catch (error) {
      console.error("Error downloading release asset from R2:", error);
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
          }),
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
      } catch (e: any) {
        console.error("Failed to serve install script:", e);
      }
    }

    // Check if this should route to container
    if (shouldRouteToContainer(url.pathname)) {
      const userId = c.get("userId") || "default";
      const container = await getContainer(c.env.CATNIP_CONTAINER, userId);
      return container.fetch(c.req.raw);
    }

    // All other requests go to static assets
    try {
      return await c.env.ASSETS.fetch(c.req.raw);
    } catch (e) {
      // If ASSETS binding is not available in development
      void e; // Acknowledge the error variable
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
export { CodespaceStore } from "./codespace-store";
