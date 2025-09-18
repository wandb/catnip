import { Hono, Context } from "hono";
import { cors } from "hono/cors";
import {
  getSignedCookie,
  setSignedCookie,
  setCookie,
  getCookie,
} from "hono/cookie";
import { HTTPException } from "hono/http-exception";
import { githubAuth } from "@hono/oauth-providers/github";
import { Container, getContainer } from "@cloudflare/containers";
import { Webhooks } from "@octokit/webhooks";
import { generateMobileToken } from "./mobile-auth";

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
  githubRepository?: string;
  githubOrg?: string;
  githubRepo?: string;
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

// Check if codespace health endpoint is responding
async function checkCodespaceHealth(
  codespaceUrl: string,
  githubToken: string,
  options: { hasFreshCredentials?: boolean } = {},
): Promise<{ healthy: boolean; lastStatus?: number; lastError?: string }> {
  const maxAttempts = 8; // Check for up to 40 seconds (8 * 5s)
  const delayMs = 5000; // 5 second intervals
  let lastStatus: number | undefined;
  let lastError: string | undefined;

  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      console.log(
        `Health check attempt ${attempt}/${maxAttempts} for ${codespaceUrl}`,
      );

      const response = await fetch(`${codespaceUrl}/health`, {
        method: "GET",
        headers: {
          "X-Github-Token": githubToken,
          "User-Agent": "Catnip-Worker/1.0",
        },
        signal: AbortSignal.timeout(3000), // 3 second timeout per request
      });

      lastStatus = response.status;

      if (response.ok) {
        console.log(`Codespace health check passed on attempt ${attempt}`);
        return { healthy: true };
      }

      console.log(`Health check attempt ${attempt} failed: ${response.status}`);

      // If we get a 401, check if we should be patient with fresh credentials
      if (response.status === 401) {
        if (options.hasFreshCredentials && attempt <= 6) {
          console.log(
            `Got 401 on attempt ${attempt}, but continuing due to fresh credentials`,
          );
        } else {
          console.log(`Stopping health check due to 401 authentication error`);
          return {
            healthy: false,
            lastStatus: 401,
            lastError: "Authentication failed",
          };
        }
      }
    } catch (error) {
      console.log(`Health check attempt ${attempt} error:`, error);
      lastError = error instanceof Error ? error.message : String(error);
    }

    // Wait before next attempt (except on last attempt)
    if (attempt < maxAttempts) {
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
  }

  console.log(
    `Codespace health check failed after ${maxAttempts} attempts (40 seconds)`,
  );
  return { healthy: false, lastStatus, lastError };
}

// Factory function to create app with environment bindings
export function createApp(env: Env) {
  const app = new Hono<HonoEnv>();

  // CORS for API routes
  app.use("/v1/*", cors());

  // Session middleware - load session from signed cookie or mobile token
  app.use("*", async (c, next) => {
    // Skip session loading if no encryption key
    if (!c.env.CATNIP_ENCRYPTION_KEY) {
      console.warn(
        "CATNIP_ENCRYPTION_KEY not set, skipping session middleware",
      );
      return next();
    }

    // First check for mobile token in Authorization header
    const authHeader = c.req.header("Authorization");
    if (authHeader?.startsWith("Bearer ")) {
      const mobileToken = authHeader.substring(7);

      try {
        // Get mobile session from Durable Object
        const sessionDO = c.env.SESSIONS.get(
          c.env.SESSIONS.idFromName("global"),
        );
        const mobileResponse = await sessionDO.fetch(
          `https://internal/mobile-session/${mobileToken}`,
        );

        if (mobileResponse.ok) {
          const mobileSession = await mobileResponse.json();

          // Get the actual session data
          const sessionResponse = await sessionDO.fetch(
            `https://internal/session/${mobileSession.sessionId}`,
          );

          if (sessionResponse.ok) {
            const session = await sessionResponse.json<SessionData>();
            c.set("session", session);
            c.set("sessionId", mobileSession.sessionId);
            c.set("userId", session.userId);
            c.set("username", session.username);
            c.set("accessToken", session.accessToken);
            c.set("mobileToken", mobileToken);
          }
        }
      } catch (error) {
        console.error("Failed to load mobile session:", error);
      }
    }

    // Fall back to cookie-based session if no mobile token
    if (!c.get("session")) {
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
    }

    await next();
  });

  // Org subdomain middleware - handles OAuth redirect for subdomains
  app.use("*", async (c, next) => {
    const url = new URL(c.req.url);
    const hostname = url.hostname;

    // Check if this is an org subdomain (e.g., wandb.catnip.run)
    const isOrgSubdomain =
      hostname.includes(".") &&
      !hostname.startsWith("catnip.run") &&
      hostname.endsWith(".catnip.run");

    if (isOrgSubdomain) {
      const org = hostname.split(".")[0];
      const session = c.get("session");

      // If not authenticated and accessing an auth-required endpoint, store org and redirect to main domain
      if (
        !session &&
        (url.pathname.startsWith("/v1/codespace") ||
          url.pathname.startsWith("/v1/auth"))
      ) {
        console.log(`Org subdomain auth redirect: ${org} -> catnip.run`);

        // Store the org in a cookie that works across domains
        const currentUrl = new URL(c.req.url);
        setCookie(c, "catnip-org-preference", org, {
          domain:
            currentUrl.hostname === "localhost" ? undefined : ".catnip.run",
          httpOnly: false, // Allow JS access for redirect logic
          secure: currentUrl.hostname !== "localhost",
          sameSite: "Lax",
          maxAge: 60 * 60, // 1 hour
          path: "/",
        });

        // Redirect to appropriate domain for OAuth (org stored in cookie)
        const authDomain =
          currentUrl.hostname === "localhost"
            ? `${currentUrl.protocol}//${currentUrl.host}`
            : "https://catnip.run";
        return c.redirect(`${authDomain}/v1/auth/github`);
      }
    }

    await next();
  });

  // GitHub OAuth - handles both login initiation and callback
  // Temporarily force OAuth App mode by not passing GitHub App credentials
  app.use("/v1/auth/github", async (c, next) => {
    const currentUrl = new URL(c.req.url);
    const redirectUri = `${currentUrl.protocol}//${currentUrl.host}/v1/auth/github`;

    return githubAuth({
      client_id: env.GITHUB_CLIENT_ID,
      client_secret: env.GITHUB_CLIENT_SECRET,
      scope: ["read:user", "user:email", "repo", "codespace", "read:org"],
      redirect_uri: redirectUri,
      // Use OAuth App mode for broader scope access
      oauthApp: true,
    })(c, next);
  });

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

    console.log(
      `OAuth completed for user: ${user.login}, scopes: ${grantedScopes?.join(", ")}`,
    );

    // Create session in Durable Object
    const sessionId = crypto.randomUUID();
    const sessionDO = c.env.SESSIONS.get(c.env.SESSIONS.idFromName("global"));

    const now = Date.now();

    // Calculate token expiration using actual values from OAuth response
    let expiresAt: number;
    if (tokenData.expires_in) {
      // expires_in is in seconds, convert to milliseconds
      expiresAt = now + tokenData.expires_in * 1000;
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

    // Check if this is a mobile OAuth flow
    const mobileState = getCookie(c, "mobile-oauth-state");
    if (mobileState) {
      try {
        const { redirectUri, state } = JSON.parse(mobileState);

        // Generate a mobile session token
        const mobileToken = generateMobileToken();

        // Store the mobile session mapping
        await sessionDO.fetch(`https://internal/mobile-session/${mobileToken}`, {
          method: "PUT",
          body: JSON.stringify({
            sessionId,
            userId: sessionData.userId,
            username: sessionData.username,
            expiresAt: sessionData.expiresAt,
          }),
        });

        // Clear the mobile OAuth state cookie
        setCookie(c, "mobile-oauth-state", "", {
          httpOnly: true,
          secure: true,
          sameSite: "Lax",
          maxAge: 0,
          path: "/",
        });

        // Redirect to mobile app with token
        const redirectUrl = new URL(redirectUri);
        redirectUrl.searchParams.set("token", mobileToken);
        redirectUrl.searchParams.set("state", state);
        redirectUrl.searchParams.set("username", sessionData.username);

        return c.redirect(redirectUrl.toString());
      } catch (error) {
        console.error("Mobile OAuth callback error:", error);
        // Fall through to standard web flow
      }
    }

    // Set signed cookie with just session ID
    if (!c.env.CATNIP_ENCRYPTION_KEY) {
      console.error(
        "Cannot set signed cookie: CATNIP_ENCRYPTION_KEY not configured",
      );
      return c.text("Server configuration error", 500);
    }

    const currentUrl = new URL(c.req.url);
    await setSignedCookie(
      c,
      "catnip-session",
      sessionId,
      c.env.CATNIP_ENCRYPTION_KEY,
      {
        httpOnly: true,
        secure: currentUrl.hostname !== "localhost",
        sameSite: "Lax",
        maxAge: 30 * 24 * 60 * 60, // 30 days - longer than token expiry to support refresh
        path: "/",
        domain: currentUrl.hostname === "localhost" ? undefined : ".catnip.run", // Allow access from all catnip.run subdomains or localhost
      },
    );

    // Check for return URL first
    const returnTo = c.req.query("return_to");
    if (returnTo && returnTo.startsWith("http")) {
      return c.redirect(returnTo);
    }

    // Check for org preference cookie and redirect to org subdomain
    const orgPreference = getCookie(c, "catnip-org-preference");
    if (orgPreference) {
      const currentUrl = new URL(c.req.url);

      // Clear the preference cookie after use
      setCookie(c, "catnip-org-preference", "", {
        domain: currentUrl.hostname === "localhost" ? undefined : ".catnip.run",
        maxAge: 0,
        path: "/",
      });

      // Redirect to org subdomain or localhost based on current environment
      const redirectUrl =
        currentUrl.hostname === "localhost"
          ? `${currentUrl.protocol}//${currentUrl.host}/v1/codespace?org=${orgPreference}`
          : `https://${orgPreference}.catnip.run/v1/codespace`;
      return c.redirect(redirectUrl);
    }

    // Check for relative return URL
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
    const currentUrl = new URL(c.req.url);
    setCookie(c, "catnip-session", "", {
      httpOnly: true,
      secure: currentUrl.hostname !== "localhost",
      sameSite: "Lax",
      maxAge: 0,
      path: "/",
      domain: currentUrl.hostname === "localhost" ? undefined : ".catnip.run", // Clear cookie from all catnip.run subdomains or localhost
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

  // Debug endpoint for GitHub token permissions
  app.get("/v1/auth/debug", requireAuth, async (c) => {
    const accessToken = c.get("accessToken");
    const username = c.get("username");

    try {
      // Get token info
      const tokenResponse = await fetch("https://api.github.com/user", {
        headers: {
          Authorization: `Bearer ${accessToken}`,
          Accept: "application/vnd.github.v3+json",
          "User-Agent": "Catnip-Worker/1.0",
        },
      });

      const scopes =
        tokenResponse.headers.get("x-oauth-scopes")?.split(", ") || [];
      const appScopes =
        tokenResponse.headers.get("x-accepted-oauth-scopes")?.split(", ") || [];

      // Check org memberships
      const orgsResponse = await fetch("https://api.github.com/user/orgs", {
        headers: {
          Authorization: `Bearer ${accessToken}`,
          Accept: "application/vnd.github.v3+json",
          "User-Agent": "Catnip-Worker/1.0",
        },
      });

      const orgs = orgsResponse.ok
        ? ((await orgsResponse.json()) as Array<{ login: string }>)
        : [];

      return c.json({
        username,
        tokenScopes: scopes,
        appAcceptedScopes: appScopes,
        hasCodespaceScope: scopes.includes("codespace"),
        organizations: orgs.map((org) => org.login),
        troubleshooting: {
          reauthorizeUrl: "/v1/auth/logout",
          expectedScopes: ["read:user", "user:email", "repo", "codespace"],
        },
      });
    } catch (error) {
      return c.json({ error: "Failed to debug token", details: error }, 500);
    }
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
      isOAuthApp: true,
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
      const { GITHUB_TOKEN, GITHUB_USER, CODESPACE_NAME, GITHUB_REPOSITORY } =
        body;

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

      // Parse org and repo from GITHUB_REPOSITORY if available
      let githubOrg: string | undefined;
      let githubRepo: string | undefined;
      if (GITHUB_REPOSITORY) {
        const repoParts = GITHUB_REPOSITORY.split("/");
        if (repoParts.length === 2) {
          githubOrg = repoParts[0];
          githubRepo = repoParts[1];
        }
      }

      // Store credentials in Durable Object
      const codespaceStore = c.env.CODESPACE_STORE.get(
        c.env.CODESPACE_STORE.idFromName("global"),
      );

      const credentials: CodespaceCredentials = {
        githubToken: GITHUB_TOKEN,
        githubUser: GITHUB_USER,
        codespaceName: CODESPACE_NAME,
        githubRepository: GITHUB_REPOSITORY,
        githubOrg,
        githubRepo,
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
        console.error("Failed to store codespace credentials", {
          status: storeResponse.status,
          user: GITHUB_USER,
          codespaceName: CODESPACE_NAME,
        });
        return c.json({ error: "Failed to store credentials" }, 500);
      }

      console.log("Codespace credentials stored successfully", {
        user: GITHUB_USER,
        codespaceName: CODESPACE_NAME,
        updatedAt: credentials.updatedAt,
      });

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

  // Codespace access endpoint - SSE for real-time status updates
  app.get("/v1/codespace", requireAuth, async (c) => {
    const { readable, writable } = new TransformStream();
    const writer = writable.getWriter();

    const sendEvent = (event: string, data: any) => {
      const payload = `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
      void writer.write(new TextEncoder().encode(payload));
    };

    // Start the codespace connection process
    void (async () => {
      try {
        const username = c.get("username");
        const accessToken = c.get("accessToken");

        if (!username || !accessToken) {
          sendEvent("error", {
            message: "No authenticated user or access token",
          });
          void writer.close();
          return;
        }

        sendEvent("status", { message: "Finding codespace", step: "search" });

        // Extract organization from subdomain (e.g., wandb.catnip.run -> wandb)
        const hostname = new URL(c.req.url).hostname;
        const orgFromSubdomain =
          hostname.includes(".") && !hostname.startsWith("catnip.run")
            ? hostname.split(".")[0]
            : null;

        console.log(
          `Codespace request - hostname: ${hostname}, org: ${orgFromSubdomain || "user"}, user: ${username}`,
        );

        // Debug: Check token permissions and org access
        try {
          // Check token scopes
          const tokenCheckResponse = await fetch(
            "https://api.github.com/user",
            {
              headers: {
                Authorization: `Bearer ${accessToken}`,
                Accept: "application/vnd.github.v3+json",
                "User-Agent": "Catnip-Worker/1.0",
              },
            },
          );

          if (tokenCheckResponse.ok) {
            const scopes = tokenCheckResponse.headers.get("x-oauth-scopes");
            const appScopes = tokenCheckResponse.headers.get(
              "x-accepted-oauth-scopes",
            );
            console.log(`Token scopes: ${scopes}, App accepts: ${appScopes}`);
          }

          // If org specified, check org membership
          if (orgFromSubdomain) {
            const orgMembershipResponse = await fetch(
              `https://api.github.com/orgs/${orgFromSubdomain}/members/${username}`,
              {
                headers: {
                  Authorization: `Bearer ${accessToken}`,
                  Accept: "application/vnd.github.v3+json",
                  "User-Agent": "Catnip-Worker/1.0",
                },
              },
            );
            console.log(
              `Org membership check for ${orgFromSubdomain}: ${orgMembershipResponse.status}`,
            );
          }
        } catch (debugError) {
          console.warn("Debug checks failed:", debugError);
        }

        // Check if CODESPACE_STORE binding is available
        if (!c.env.CODESPACE_STORE) {
          console.error("CODESPACE_STORE binding not available");
          sendEvent("error", {
            message:
              "Codespace storage not configured. Please contact support.",
          });
          void writer.close();
          return;
        }

        // First, check CODESPACE_STORE for stored codespace credentials
        let storedCodespaces: CodespaceCredentials[] = [];
        let selectedCodespace: CodespaceCredentials | null = null;

        try {
          const codespaceStore = c.env.CODESPACE_STORE.get(
            c.env.CODESPACE_STORE.idFromName("global"),
          );

          // Check if user is requesting a specific codespace
          const requestedCodespace = c.req.query("codespace");

          console.log(
            `Codespace request debug - requestedCodespace: "${requestedCodespace}"`,
          );

          if (requestedCodespace) {
            // Try to get specific codespace first
            const specificResponse = await codespaceStore.fetch(
              `https://internal/codespace/${username}`,
            );
            if (specificResponse.ok) {
              const singleCodespace =
                (await specificResponse.json()) as CodespaceCredentials;
              if (singleCodespace.codespaceName === requestedCodespace) {
                selectedCodespace = singleCodespace;
              }
            }
          }

          if (!selectedCodespace) {
            // Get all codespaces for this user
            const allResponse = await codespaceStore.fetch(
              `https://internal/codespace/${username}?all=true`,
            );

            if (allResponse.ok) {
              storedCodespaces =
                (await allResponse.json()) as CodespaceCredentials[];

              // If a specific codespace was requested, try to find it in the list
              if (requestedCodespace) {
                console.log(
                  `Looking for codespace "${requestedCodespace}" in list of ${storedCodespaces.length} codespaces:`,
                );
                storedCodespaces.forEach((cs, i) => {
                  console.log(`  ${i}: "${cs.codespaceName}"`);
                });

                selectedCodespace =
                  storedCodespaces.find(
                    (cs) => cs.codespaceName === requestedCodespace,
                  ) || null;

                if (!selectedCodespace) {
                  console.log(
                    `Codespace "${requestedCodespace}" not found in stored codespaces`,
                  );
                  sendEvent("error", {
                    message: `Requested codespace "${requestedCodespace}" not found.`,
                  });
                  void writer.close();
                  return;
                } else {
                  console.log(
                    `Found requested codespace: ${selectedCodespace.codespaceName}`,
                  );
                }
              } else {
                // No specific codespace requested, handle multiple codespace logic
                if (storedCodespaces.length === 1) {
                  selectedCodespace = storedCodespaces[0];
                } else if (storedCodespaces.length > 1) {
                  // Filter by org if accessing via org subdomain
                  if (orgFromSubdomain) {
                    const orgCodespaces = storedCodespaces.filter(
                      (cs) =>
                        // First try exact match with stored org info
                        cs.githubOrg === orgFromSubdomain ||
                        // Fallback to name-based matching for backwards compatibility
                        cs.codespaceName.includes(orgFromSubdomain) ||
                        cs.codespaceName.includes(`${orgFromSubdomain}-`),
                    );
                    if (orgCodespaces.length === 1) {
                      selectedCodespace = orgCodespaces[0];
                    } else if (orgCodespaces.length > 1) {
                      sendEvent("multiple", {
                        message:
                          "Multiple codespaces found. Please select one.",
                        codespaces: orgCodespaces.map((cs) => ({
                          name: cs.codespaceName,
                          lastUsed: cs.updatedAt,
                          repository: cs.githubRepository,
                        })),
                        org: orgFromSubdomain,
                      });
                      void writer.close();
                      return;
                    }
                  } else {
                    // Multiple codespaces and no org filter - user needs to choose
                    sendEvent("multiple", {
                      message: "Multiple codespaces found. Please select one.",
                      codespaces: storedCodespaces.map((cs) => ({
                        name: cs.codespaceName,
                        lastUsed: cs.updatedAt,
                        repository: cs.githubRepository,
                      })),
                      org: null,
                    });
                    void writer.close();
                    return;
                  }
                }
              }
            }
          }
        } catch (error) {
          console.warn("Failed to retrieve stored codespace info:", error);
        }

        let targetCodespace: {
          id: number;
          name: string;
          state: string;
          web_url: string;
          created_at: string;
          updated_at: string;
          last_used_at: string;
        } | null = null;

        // If we have a selected codespace, try to get its current status directly
        if (selectedCodespace) {
          console.log(
            `Checking status of selected codespace: ${selectedCodespace.codespaceName}`,
          );

          try {
            const codespaceStatusResponse = await fetch(
              `https://api.github.com/user/codespaces/${selectedCodespace.codespaceName}`,
              {
                headers: {
                  Authorization: `Bearer ${accessToken}`,
                  Accept: "application/vnd.github.v3+json",
                  "User-Agent": "Catnip-Worker/1.0",
                },
              },
            );

            if (codespaceStatusResponse.ok) {
              targetCodespace = await codespaceStatusResponse.json();
              if (targetCodespace) {
                console.log(
                  `Selected codespace found and accessible: ${targetCodespace.name}, state: ${targetCodespace.state}`,
                );
              }
            } else {
              console.log(
                `Selected codespace not accessible: ${codespaceStatusResponse.status}`,
              );
              // Continue to fallback logic below
            }
          } catch (error) {
            console.warn("Failed to check selected codespace status:", error);
          }
        }

        // If no stored codespace or it's not accessible, we can't help the user
        if (!targetCodespace) {
          const errorMsg = orgFromSubdomain
            ? `No Catnip codespaces found for the "${orgFromSubdomain}" organization. Please start a codespace with Catnip feature enabled first.`
            : "No Catnip codespaces found. Please start a codespace with Catnip feature enabled first.";

          console.log(
            `No stored codespace available for user: ${username}${orgFromSubdomain ? `, org: ${orgFromSubdomain}` : ""}`,
          );

          // Check if we have codespaces but they don't match the org filter
          if (storedCodespaces.length > 0 && orgFromSubdomain) {
            console.log(
              `Found ${storedCodespaces.length} codespaces for user, but none match org "${orgFromSubdomain}"`,
            );
            storedCodespaces.forEach((cs) => {
              console.log(
                `- Codespace: ${cs.codespaceName}, stored org: ${cs.githubOrg || "unknown"}`,
              );
            });
          }

          sendEvent("setup", { message: errorMsg, org: orgFromSubdomain });
          void writer.close();
          return;
        }

        // If codespace is not running, start it using the user's OAuth token
        if (targetCodespace.state !== "Available") {
          console.log(
            `Starting codespace ${targetCodespace.name} (current state: ${targetCodespace.state})`,
          );
          sendEvent("status", {
            message: "Starting up codespace",
            step: "starting",
          });

          const startResponse = await fetch(
            `https://api.github.com/user/codespaces/${targetCodespace.name}/start`,
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
            sendEvent("error", {
              message:
                "Failed to start codespace. Please ensure you have codespace permissions.",
            });
            void writer.close();
            return;
          }

          // Give the codespace time to start and send fresh credentials
          sendEvent("status", {
            message: "Setting up codespace environment",
            step: "setup",
          });
          await new Promise((resolve) => setTimeout(resolve, 10000)); // Wait longer for startup
        }

        // Check if codespace is healthy before redirecting
        const codespaceUrl = `https://${targetCodespace.name}-6369.app.github.dev?catnip=true`;
        const healthCheckUrl = `https://${targetCodespace.name}-6369.app.github.dev`;

        // Check health endpoint - either with stored credentials or wait for fresh ones
        if (selectedCodespace) {
          console.log("Checking codespace health before redirect...");
          sendEvent("status", {
            message: "Starting catnip server",
            step: "catnip",
          });

          // Try to get the freshest credentials before health check
          let healthCheckToken = selectedCodespace.githubToken || "";
          let credentialsRefreshed = false;

          // If we don't have credentials or codespace was just started, retry credential refresh multiple times
          const maxRefreshAttempts =
            !selectedCodespace.githubToken ||
            targetCodespace.state !== "Available"
              ? 7
              : 1;

          for (let attempt = 1; attempt <= maxRefreshAttempts; attempt++) {
            try {
              console.log(
                `Attempting to refresh codespace credentials (attempt ${attempt}/${maxRefreshAttempts})...`,
              );
              const codespaceStore = c.env.CODESPACE_STORE.get(
                c.env.CODESPACE_STORE.idFromName("global"),
              );
              const refreshResponse = await codespaceStore.fetch(
                `https://internal/codespace/${username}`,
              );
              if (refreshResponse.ok) {
                const refreshedCredentials =
                  (await refreshResponse.json()) as CodespaceCredentials;
                if (refreshedCredentials.githubToken) {
                  const hadToken = !!selectedCodespace.githubToken;
                  const credentialsAge = new Date(
                    refreshedCredentials.updatedAt,
                  ).toLocaleString();
                  const tokenPreview =
                    refreshedCredentials.githubToken.substring(0, 7) + "...";

                  if (
                    !hadToken ||
                    refreshedCredentials.githubToken !==
                      selectedCodespace.githubToken
                  ) {
                    console.log(
                      hadToken
                        ? `Fresh GitHub token received for health check - Updated: ${credentialsAge}, Token: ${tokenPreview}`
                        : `GitHub token received for codespace without stored credentials - Updated: ${credentialsAge}, Token: ${tokenPreview}`,
                    );
                    credentialsRefreshed = true;
                  } else {
                    console.log(
                      `Using existing GitHub token for health check - Updated: ${credentialsAge}, Token: ${tokenPreview}`,
                    );
                  }
                  healthCheckToken = refreshedCredentials.githubToken;
                  selectedCodespace = refreshedCredentials; // Update our local copy

                  // If we got fresh credentials or we didn't have any before, we can break early
                  if (credentialsRefreshed) {
                    break;
                  }
                }
              } else {
                console.warn(
                  `Could not refresh credentials: ${refreshResponse.status}`,
                );
              }
            } catch (error) {
              console.warn(
                `Credential refresh attempt ${attempt} failed:`,
                error,
              );
            }

            // Wait between attempts (except on last attempt)
            if (attempt < maxRefreshAttempts) {
              const waitTime = !selectedCodespace.githubToken ? 6000 : 5000; // Longer wait if no initial credentials
              console.log(
                `Waiting for codespace to send fresh credentials... (${waitTime / 1000}s)`,
              );
              await new Promise((resolve) => setTimeout(resolve, waitTime));
            }
          }

          if (credentialsRefreshed) {
            console.log("Using refreshed credentials for health check");
            // Give extra time for fresh credentials to propagate
            sendEvent("status", {
              message: "Waiting for fresh credentials to propagate",
              step: "initializing",
            });
            await new Promise((resolve) => setTimeout(resolve, 8000));
          } else {
            console.log("Using original stored credentials for health check");
            // Give catnip a moment to be ready for health check
            sendEvent("status", {
              message: "Waiting for catnip to be ready",
              step: "initializing",
            });
            // Shorter wait since we already waited during credential refresh attempts
            await new Promise((resolve) => setTimeout(resolve, 3000));
          }

          sendEvent("status", {
            message: "Waiting for catnip to be ready",
            step: "health",
          });

          // If we still don't have a token after refresh attempts, try direct connect
          if (!healthCheckToken) {
            console.log(
              "No stored credentials available, attempting direct connection",
            );
            sendEvent("success", {
              message: "Connecting to codespace (credentials pending)",
              codespaceUrl,
              step: "ready",
            });
            void writer.close();
            return;
          }

          console.log(
            `Starting health check with token: ${healthCheckToken.substring(0, 7)}..., credentials age: ${new Date(selectedCodespace.updatedAt).toLocaleString()}`,
          );
          const healthResult = await checkCodespaceHealth(
            healthCheckUrl,
            healthCheckToken,
            { hasFreshCredentials: credentialsRefreshed },
          );

          if (healthResult.healthy) {
            sendEvent("success", {
              message: "Codespace is ready",
              codespaceUrl,
              step: "ready",
            });
            void writer.close();
            return;
          } else {
            // Health check failed with stored credentials
            if (healthResult.lastStatus === 401) {
              console.error(
                "Health check failed with 401 - stored credentials are invalid or expired",
                {
                  storedTokenStatus: healthResult.lastStatus,
                  storedTokenError: healthResult.lastError,
                },
              );
              sendEvent("error", {
                message:
                  "Authentication error accessing codespace. The stored credentials may be expired or the codespace may not have sent fresh credentials yet. Please wait a moment and try again, or check that Catnip is properly installed in your codespace.",
                codespaceName: targetCodespace.name,
                retryAfter: 30,
              });
              void writer.close();
              return;
            } else {
              // Other error, likely just startup related
              console.log(
                `Health check failed with status ${healthResult.lastStatus}: ${healthResult.lastError}`,
              );
              sendEvent("error", {
                message:
                  "Codespace is still starting up. Please wait a moment and try again.",
                codespaceName: targetCodespace.name,
                retryAfter: 15,
              });
              void writer.close();
              return;
            }
          }
        } else {
          // No stored credentials, assume it's healthy and let frontend handle
          sendEvent("success", {
            message: "Redirecting to codespace",
            codespaceUrl,
            step: "ready",
          });
          void writer.close();
          return;
        }
      } catch (error) {
        console.error("Codespace access error:", error);
        sendEvent("error", { message: "Internal server error" });
        void writer.close();
        return;
      }
    })();

    return new Response(readable, {
      status: 200,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Headers": "Cache-Control",
      },
    });
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
        codespaces_lifecycle_admin: "write",
        organization_codespaces: "write",
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
    // Disable container routing for main catnip.run domain (codespace-only)
    const isMainDomain =
      url.hostname === "catnip.run" || url.hostname.endsWith(".catnip.run");

    if (shouldRouteToContainer(url.pathname) && !isMainDomain) {
      const userId = c.get("userId") || "default";
      const container = await getContainer(c.env.CATNIP_CONTAINER, userId);
      return container.fetch(c.req.raw);
    }

    // Serve React app for main domain root - this will show the CodespaceAccess component
    if (isMainDomain && url.pathname === "/") {
      try {
        return await c.env.ASSETS.fetch(c.req.raw);
      } catch (e: any) {
        console.error("Failed to serve React app:", e);
        void e; // Acknowledge the error variable
        return c.text("Static asset serving not configured", 503);
      }
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
