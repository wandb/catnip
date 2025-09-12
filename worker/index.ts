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
): Promise<boolean> {
  const maxAttempts = 12; // Check for up to 1 minute (12 * 5s)
  const delayMs = 5000; // 5 second intervals

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

      if (response.ok) {
        console.log(`Codespace health check passed on attempt ${attempt}`);
        return true;
      }

      console.log(`Health check attempt ${attempt} failed: ${response.status}`);
    } catch (error) {
      console.log(`Health check attempt ${attempt} error:`, error);
    }

    // Wait before next attempt (except on last attempt)
    if (attempt < maxAttempts) {
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
  }

  console.log(`Codespace health check failed after ${maxAttempts} attempts`);
  return false;
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

        // Store the org in a cookie that works across all catnip.run subdomains
        setCookie(c, "catnip-org-preference", org, {
          domain: ".catnip.run",
          httpOnly: false, // Allow JS access for redirect logic
          secure: true,
          sameSite: "Lax",
          maxAge: 60 * 60, // 1 hour
          path: "/",
        });

        // Redirect to main domain for OAuth (org stored in cookie)
        return c.redirect(`https://catnip.run/v1/auth/github`);
      }
    }

    await next();
  });

  // GitHub OAuth - handles both login initiation and callback
  // Temporarily force OAuth App mode by not passing GitHub App credentials
  app.use(
    "/v1/auth/github",
    githubAuth({
      client_id: env.GITHUB_CLIENT_ID,
      client_secret: env.GITHUB_CLIENT_SECRET,
      scope: ["read:user", "user:email", "repo", "codespace", "read:org"],
      // Use OAuth App mode for broader scope access
      oauthApp: true,
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
        domain: ".catnip.run", // Allow access from all catnip.run subdomains
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
      // Clear the preference cookie after use
      setCookie(c, "catnip-org-preference", "", {
        domain: ".catnip.run",
        maxAge: 0,
        path: "/",
      });
      return c.redirect(`https://${orgPreference}.catnip.run/v1/codespace`);
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
    setCookie(c, "catnip-session", "", {
      httpOnly: true,
      secure: true,
      sameSite: "Lax",
      maxAge: 0,
      path: "/",
      domain: ".catnip.run", // Clear cookie from all catnip.run subdomains
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

              if (storedCodespaces.length === 1) {
                selectedCodespace = storedCodespaces[0];
              } else if (storedCodespaces.length > 1) {
                // Filter by org if accessing via org subdomain
                if (orgFromSubdomain) {
                  const orgCodespaces = storedCodespaces.filter(
                    (cs) =>
                      cs.codespaceName.includes(orgFromSubdomain) ||
                      cs.codespaceName.includes(`${orgFromSubdomain}-`),
                  );
                  if (orgCodespaces.length === 1) {
                    selectedCodespace = orgCodespaces[0];
                  } else if (orgCodespaces.length > 1) {
                    sendEvent("multiple", {
                      message: "Multiple codespaces found. Please select one.",
                      codespaces: orgCodespaces.map((cs) => ({
                        name: cs.codespaceName,
                        lastUsed: cs.updatedAt,
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
                    })),
                    org: null,
                  });
                  void writer.close();
                  return;
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
            message: "Starting codespace",
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
        }

        // Check if codespace is healthy before redirecting
        const codespaceUrl = `https://${targetCodespace.name}-6369.app.github.dev?catnip=true`;
        const healthCheckUrl = `https://${targetCodespace.name}-6369.app.github.dev`;

        // If we have stored credentials, check health endpoint
        if (selectedCodespace && selectedCodespace.githubToken) {
          console.log("Checking codespace health before redirect...");
          sendEvent("status", {
            message: "Waiting for catnip to start",
            step: "health",
          });

          const isHealthy = await checkCodespaceHealth(
            healthCheckUrl,
            selectedCodespace.githubToken,
          );

          if (isHealthy) {
            sendEvent("success", {
              message: "Codespace is ready",
              codespaceUrl,
              step: "ready",
            });
            void writer.close();
            return;
          } else {
            sendEvent("error", {
              message:
                "Codespace is starting up. Please wait a moment and try again.",
              retryAfter: 10,
            });
            void writer.close();
            return;
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

    // Serve simple codespace landing page for main domain root
    if (isMainDomain && url.pathname === "/") {
      const session = c.get("session");
      const isAuthenticated = !!session;

      const html = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Catnip - Codespace Access</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            margin: 0;
            padding: 0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 40px rgba(0,0,0,0.1);
            padding: 2rem;
            max-width: 500px;
            text-align: center;
        }
        .logo {
            font-size: 3rem;
            margin-bottom: 0.5rem;
        }
        h1 {
            color: #2d3748;
            margin-bottom: 0.5rem;
            font-size: 2rem;
        }
        .subtitle {
            color: #718096;
            margin-bottom: 2rem;
            font-size: 1.1rem;
        }
        .btn {
            background: #667eea;
            color: white;
            border: none;
            border-radius: 8px;
            padding: 12px 24px;
            font-size: 1rem;
            cursor: pointer;
            text-decoration: none;
            display: inline-block;
            transition: background 0.2s;
        }
        .btn:hover {
            background: #5a6fd8;
        }
        .btn:disabled {
            background: #a0aec0;
            cursor: not-allowed;
        }
        .status {
            background: #f7fafc;
            border-radius: 8px;
            padding: 1rem;
            margin: 1rem 0;
            color: #4a5568;
        }
        .org-input {
            margin-top: 2rem;
            padding-top: 2rem;
            border-top: 1px solid #e2e8f0;
        }
        .input-group {
            display: flex;
            gap: 10px;
            margin-top: 1rem;
        }
        input {
            flex: 1;
            padding: 12px;
            border: 1px solid #cbd5e0;
            border-radius: 8px;
            font-size: 1rem;
        }
        .btn-secondary {
            background: #718096;
        }
        .btn-secondary:hover {
            background: #4a5568;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="logo">🐱</div>
        <h1>Catnip</h1>
        <div class="subtitle">Access your GitHub Codespaces</div>
        
        ${
          isAuthenticated
            ? `
        <button onclick="accessCodespace()" class="btn" id="codespace-btn">Access My Codespace</button>
        <div id="status-message" class="status" style="display:none;"></div>
        
        <div class="disclaimer" style="background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 8px; padding: 1rem; margin: 1rem 0; color: #64748b; font-size: 0.9rem; text-align: left;">
            <strong>Note:</strong> If you see the VSCode interface, click the back button to access your codespace again.
        </div>
        
        <div class="org-input">
            <p>Or access codespaces in a specific organization:</p>
            <div class="input-group">
                <input type="text" id="orgName" placeholder="Organization name (e.g., wandb)" />
                <button onclick="goToOrg()" class="btn btn-secondary">Go</button>
            </div>
        </div>
        
        <script>
            function goToOrg() {
                const org = document.getElementById('orgName').value.trim();
                if (org) {
                    accessCodespace(org);
                }
            }
            document.getElementById('orgName').addEventListener('keypress', function(e) {
                if (e.key === 'Enter') {
                    goToOrg();
                }
            });
            
            function resetUIState() {
                const btn = document.getElementById('codespace-btn');
                const statusMsg = document.getElementById('status-message');
                const orgInput = document.getElementById('orgName');
                
                // Reset button
                btn.disabled = false;
                btn.textContent = 'Access My Codespace';
                
                // Reset org input
                if (orgInput) {
                    orgInput.disabled = false;
                    orgInput.value = '';
                }
                
                // Hide status message
                statusMsg.style.display = 'none';
                statusMsg.textContent = '';
                statusMsg.style.background = '';
                statusMsg.innerHTML = '';
            }
            
            async function accessCodespace(org = null) {
                const btn = document.getElementById('codespace-btn');
                const statusMsg = document.getElementById('status-message');
                const orgInput = document.getElementById('orgName');
                
                // Disable buttons and show loading
                btn.disabled = true;
                btn.textContent = 'Connecting...';
                if (orgInput) orgInput.disabled = true;
                statusMsg.style.display = 'block';
                statusMsg.className = 'status';
                statusMsg.textContent = '🔄 Finding your codespace...';
                statusMsg.style.background = '#f0f9ff';
                
                // Use Server-Sent Events for real-time updates
                const url = org ? \`https://\${org}.catnip.run/v1/codespace\` : '/v1/codespace';
                const eventSource = new EventSource(url);
                
                eventSource.addEventListener('status', (event) => {
                    const data = JSON.parse(event.data);
                    const icon = data.step === 'search' ? '🔍' :
                               data.step === 'starting' ? '⚡' :
                               data.step === 'health' ? '⏳' : '🔄';
                    statusMsg.textContent = \`\${icon} \${data.message}\`;
                    statusMsg.style.background = '#f0f9ff';
                });
                
                eventSource.addEventListener('success', (event) => {
                    const data = JSON.parse(event.data);
                    statusMsg.textContent = '✅ ' + data.message;
                    statusMsg.style.background = '#f0fff4';
                    eventSource.close();
                    
                    setTimeout(() => {
                        // Reset UI state before redirect so back button shows clean state
                        resetUIState();
                        window.location.href = data.codespaceUrl;
                    }, 1000);
                });
                
                eventSource.addEventListener('error', (event) => {
                    const data = JSON.parse(event.data);
                    statusMsg.textContent = '❌ ' + data.message;
                    statusMsg.style.background = '#fef2f2';
                    eventSource.close();
                    
                    // Re-enable buttons
                    btn.disabled = false;
                    btn.textContent = 'Access My Codespace';
                    if (orgInput) orgInput.disabled = false;
                    
                    // Handle retry logic
                    if (data.retryAfter) {
                        setTimeout(() => {
                            accessCodespace(org);
                        }, data.retryAfter * 1000);
                    }
                });
                
                eventSource.addEventListener('setup', (event) => {
                    const data = JSON.parse(event.data);
                    showSetupInstructions(data.org);
                    eventSource.close();
                    
                    // Re-enable buttons
                    btn.disabled = false;
                    btn.textContent = 'Access My Codespace';
                    if (orgInput) orgInput.disabled = false;
                });
                
                eventSource.addEventListener('multiple', (event) => {
                    const data = JSON.parse(event.data);
                    showCodespaceSelection(data.codespaces, data.org);
                    eventSource.close();
                    
                    // Re-enable buttons
                    btn.disabled = false;
                    btn.textContent = 'Access My Codespace';
                    if (orgInput) orgInput.disabled = false;
                });
                
                // Handle connection errors
                eventSource.onerror = (error) => {
                    statusMsg.textContent = '❌ Connection failed. Please try again.';
                    statusMsg.style.background = '#fef2f2';
                    eventSource.close();
                    
                    // Re-enable buttons
                    btn.disabled = false;
                    btn.textContent = 'Access My Codespace';
                    if (orgInput) orgInput.disabled = false;
                };
            }
            
            function showCodespaceSelection(codespaces, org) {
                const statusMsg = document.getElementById('status-message');
                const orgText = org ? \` in the "\${org}" organization\` : '';
                
                const codespaceOptions = codespaces.map((cs, index) => {
                    const lastUsedDate = new Date(cs.lastUsed).toLocaleString();
                    return \`
                        <div style="border: 1px solid #e2e8f0; border-radius: 8px; padding: 1rem; margin: 0.5rem 0; cursor: pointer; transition: background 0.2s;"
                             onclick="selectCodespace('\${cs.name}', '\${org || ''}')"
                             onmouseover="this.style.background='#f7fafc'"
                             onmouseout="this.style.background='white'">
                            <div style="font-weight: 600; color: #2d3748; margin-bottom: 0.25rem;">
                                \${cs.name.replace(/-/g, ' ')}
                            </div>
                            <div style="font-size: 0.9rem; color: #718096;">
                                Last used: \${lastUsedDate}
                            </div>
                        </div>
                    \`;
                }).join('');
                
                statusMsg.innerHTML = \`
                    <div style="text-align: left; line-height: 1.5;">
                        <h3 style="margin-top: 0; color: #2d3748;">🔍 Select Codespace</h3>
                        <p>Multiple codespaces found\${orgText}. Please select one to connect:</p>
                        \${codespaceOptions}
                    </div>
                \`;
                statusMsg.style.background = '#f7fafc';
                statusMsg.style.display = 'block';
            }
            
            function selectCodespace(codespaceName, org) {
                // Reset UI
                const btn = document.getElementById('codespace-btn');
                btn.disabled = false;
                btn.textContent = 'Access My Codespace';
                
                // Call accessCodespace with specific codespace parameter
                const url = org ? \`https://\${org}.catnip.run/v1/codespace?codespace=\${encodeURIComponent(codespaceName)}\` : 
                                  \`/v1/codespace?codespace=\${encodeURIComponent(codespaceName)}\`;
                
                // Update the access function to use the specific codespace
                accessCodespaceSpecific(url, org);
            }
            
            async function accessCodespaceSpecific(url, org) {
                const btn = document.getElementById('codespace-btn');
                const statusMsg = document.getElementById('status-message');
                
                // Show loading
                btn.disabled = true;
                btn.textContent = 'Connecting...';
                statusMsg.textContent = '🔄 Connecting to selected codespace...';
                statusMsg.style.background = '#f0f9ff';
                
                // Use Server-Sent Events for real-time updates
                const eventSource = new EventSource(url);
                
                eventSource.addEventListener('status', (event) => {
                    const data = JSON.parse(event.data);
                    const icon = data.step === 'search' ? '🔍' :
                               data.step === 'starting' ? '⚡' :
                               data.step === 'health' ? '⏳' : '🔄';
                    statusMsg.textContent = \`\${icon} \${data.message}\`;
                    statusMsg.style.background = '#f0f9ff';
                });
                
                eventSource.addEventListener('success', (event) => {
                    const data = JSON.parse(event.data);
                    statusMsg.textContent = '✅ ' + data.message;
                    statusMsg.style.background = '#f0fff4';
                    eventSource.close();
                    
                    setTimeout(() => {
                        // Reset UI state before redirect so back button shows clean state
                        resetUIState();
                        window.location.href = data.codespaceUrl;
                    }, 1000);
                });
                
                eventSource.addEventListener('error', (event) => {
                    const data = JSON.parse(event.data);
                    statusMsg.textContent = '❌ ' + data.message;
                    statusMsg.style.background = '#fef2f2';
                    eventSource.close();
                    
                    // Re-enable button
                    btn.disabled = false;
                    btn.textContent = 'Access My Codespace';
                    
                    // Handle retry logic
                    if (data.retryAfter) {
                        setTimeout(() => {
                            accessCodespaceSpecific(url, org);
                        }, data.retryAfter * 1000);
                    }
                });
                
                // Handle connection errors
                eventSource.onerror = (error) => {
                    statusMsg.textContent = '❌ Connection failed. Please try again.';
                    statusMsg.style.background = '#fef2f2';
                    eventSource.close();
                    
                    // Re-enable button
                    btn.disabled = false;
                    btn.textContent = 'Access My Codespace';
                };
            }

            function showSetupInstructions(org) {
                const statusMsg = document.getElementById('status-message');
                const orgText = org ? \` in the "\${org}" organization\` : '';
                statusMsg.innerHTML = \`
                    <div style="text-align: left; line-height: 1.5;">
                        <h3 style="margin-top: 0; color: #2d3748;">📋 Setup Required</h3>
                        <p>No Catnip codespaces found\${orgText}. To use Catnip, you need to:</p>
                        <ol style="margin: 1rem 0; padding-left: 1.5rem;">
                            <li>Add this to your <code>.devcontainer/devcontainer.json</code>:
                                <pre style="background: #f7fafc; padding: 0.5rem; border-radius: 4px; margin: 0.5rem 0; font-size: 0.9rem; overflow-x: auto;">"features": {
  "ghcr.io/wandb/catnip/feature:1": {}
}</pre>
                            </li>
                            <li>Create a new codespace from your repository</li>
                            <li>Return here to access your codespace</li>
                        </ol>
                        <p><a href="https://github.com/codespaces" target="_blank" style="color: #667eea;">Open GitHub Codespaces →</a></p>
                    </div>
                \`;
                statusMsg.style.background = '#f7fafc';
                statusMsg.style.display = 'block';
            }
        </script>
        `
            : `
        <div class="status">
            Please authenticate with GitHub to access your codespaces
        </div>
        <a href="/v1/auth/github" class="btn">Login with GitHub</a>
        `
        }
    </div>
</body>
</html>`;

      return new Response(html, {
        headers: { "Content-Type": "text/html" },
      });
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
