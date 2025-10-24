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
  mobileToken: string;
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

function shouldRouteToContainer(_pathname: string): boolean {
  // TEMPORARY: Container support disabled to avoid building/uploading container
  return false;

  // Original logic (re-enable when needed):
  // return CONTAINER_ROUTES.some((pattern) => pattern.test(_pathname));
}

// Check if codespace health endpoint is responding
async function checkCodespaceHealth(
  codespaceUrl: string,
  githubToken: string,
  options: { hasFreshCredentials?: boolean; isAlreadyRunning?: boolean } = {},
): Promise<{ healthy: boolean; lastStatus?: number; lastError?: string }> {
  // Reduce attempts if codespace is already running - should succeed quickly
  const maxAttempts = options.isAlreadyRunning ? 4 : 8; // 20s vs 40s max
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
        signal: AbortSignal.timeout(10000), // 10 second timeout per request
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

// Verify codespaces still exist and clean up deleted ones
// Returns only codespaces that still exist in GitHub
async function verifyAndCleanCodespaces(
  codespaces: CodespaceCredentials[],
  accessToken: string,
  username: string,
  codespaceStore: DurableObjectStub,
): Promise<CodespaceCredentials[]> {
  if (codespaces.length === 0) return [];

  console.log(
    `🔍 Verifying ${codespaces.length} codespace(s) for user ${username}`,
  );

  // Check all codespaces in parallel for performance
  const verificationPromises = codespaces.map(async (cs) => {
    try {
      const response = await fetch(
        `https://api.github.com/user/codespaces/${cs.codespaceName}`,
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        },
      );

      if (response.status === 404) {
        // Cancel the response body since we don't need it
        await response.body?.cancel();
        // Codespace deleted - remove from store
        console.log(
          `🗑️ Codespace ${cs.codespaceName} no longer exists, removing from store`,
        );
        try {
          const deleteResponse = await codespaceStore.fetch(
            `https://internal/codespace/${username}/${cs.codespaceName}`,
            { method: "DELETE" },
          );

          // Cancel the response body since we don't need it
          await deleteResponse.body?.cancel();

          if (deleteResponse.status === 404) {
            console.log(
              `ℹ️  Codespace ${cs.codespaceName} was already removed from store`,
            );
          } else if (deleteResponse.ok) {
            console.log(
              `✅ Successfully deleted ${cs.codespaceName} from store`,
            );
          } else {
            console.warn(
              `⚠️  Unexpected response deleting ${cs.codespaceName} from store: ${deleteResponse.status}`,
            );
          }
        } catch (deleteError) {
          console.warn(
            `Failed to delete ${cs.codespaceName} from store:`,
            deleteError,
          );
        }
        return null; // Mark for removal from list
      }

      if (response.ok) {
        // Cancel the response body since we don't need it
        await response.body?.cancel();
        console.log(`✅ Codespace ${cs.codespaceName} still exists`);
        return cs; // Still exists
      }

      // Other error (401, 403, 500, etc.) - keep codespace in list
      // We don't want to remove due to temporary issues
      // Cancel the response body since we don't need it
      await response.body?.cancel();
      console.warn(
        `⚠️ Could not verify ${cs.codespaceName}: ${response.status}, keeping in list`,
      );
      return cs;
    } catch (error) {
      console.warn(`⚠️ Failed to verify codespace ${cs.codespaceName}:`, error);
      // Keep on error (don't remove due to network issues)
      return cs;
    }
  });

  const results = await Promise.all(verificationPromises);
  const validCodespaces = results.filter(
    (cs) => cs !== null,
  ) as CodespaceCredentials[];

  console.log(
    `✅ Verification complete: ${validCodespaces.length}/${codespaces.length} codespace(s) still exist`,
  );

  return validCodespaces;
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
          const mobileSession = (await mobileResponse.json()) as {
            sessionId: string;
          };

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
        await sessionDO.fetch(
          `https://internal/mobile-session/${mobileToken}`,
          {
            method: "PUT",
            body: JSON.stringify({
              sessionId,
              userId: sessionData.userId,
              username: sessionData.username,
              expiresAt: sessionData.expiresAt,
            }),
          },
        );

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

  // Mobile OAuth initiation endpoint
  app.get("/v1/auth/github/mobile", async (c) => {
    const redirectUri = c.req.query("redirect_uri") || "catnip://auth";
    const state = c.req.query("state") || crypto.randomUUID();

    // Store mobile OAuth state in cookie
    setCookie(
      c,
      "mobile-oauth-state",
      JSON.stringify({
        redirectUri,
        state,
        initiated: Date.now(),
      }),
      {
        httpOnly: true,
        secure: true,
        sameSite: "Lax",
        maxAge: 10 * 60, // 10 minutes
        path: "/",
      },
    );

    // Redirect to standard OAuth flow
    const currentUrl = new URL(c.req.url);
    const githubAuthUrl = `${currentUrl.protocol}//${currentUrl.host}/v1/auth/github`;

    return c.redirect(githubAuthUrl);
  });

  // Mobile logout endpoint
  app.post("/v1/auth/mobile/logout", async (c) => {
    const mobileToken = c.get("mobileToken");

    if (mobileToken) {
      const sessionDO = c.env.SESSIONS.get(c.env.SESSIONS.idFromName("global"));
      await sessionDO.fetch(`https://internal/mobile-session/${mobileToken}`, {
        method: "DELETE",
      });
    }

    return c.json({ success: true });
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

  // List user repositories with devcontainer status
  app.get("/v1/repositories", requireAuth, async (c) => {
    const accessToken = c.get("accessToken");
    const page = parseInt(c.req.query("page") || "1");
    const perPage = parseInt(c.req.query("per_page") || "30");
    const org = c.req.query("org");

    try {
      let allRepos: Array<{
        id: number;
        name: string;
        full_name: string;
        default_branch: string;
        permissions?: { admin: boolean; push: boolean };
        private: boolean;
        fork: boolean;
        archived: boolean;
        pushed_at: string;
      }> = [];

      if (org) {
        // Fetch repositories from a specific organization
        console.log(`Fetching repositories for organization: ${org}`);
        const reposUrl = `https://api.github.com/orgs/${org}/repos?page=${page}&per_page=${perPage}&sort=pushed`;
        const reposResponse = await fetch(reposUrl, {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        });

        if (!reposResponse.ok) {
          console.error(
            "Failed to fetch org repositories:",
            reposResponse.status,
            await reposResponse.text(),
          );
          return c.json({ error: "Failed to fetch repositories" }, 500);
        }

        allRepos = await reposResponse.json();
      } else {
        // Fetch user's personal repositories
        console.log("Fetching user repositories");
        const userReposUrl = `https://api.github.com/user/repos?page=${page}&per_page=${perPage}&sort=pushed&affiliation=owner,collaborator`;
        const userReposResponse = await fetch(userReposUrl, {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        });

        if (!userReposResponse.ok) {
          console.error(
            "Failed to fetch user repositories:",
            userReposResponse.status,
            await userReposResponse.text(),
          );
          return c.json({ error: "Failed to fetch repositories" }, 500);
        }

        allRepos = await userReposResponse.json();

        // Fetch user's organizations (up to 3)
        console.log("Fetching user organizations");
        const orgsResponse = await fetch("https://api.github.com/user/orgs", {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        });

        if (orgsResponse.ok) {
          const orgs = (await orgsResponse.json()) as Array<{ login: string }>;
          const orgReposPromises = orgs.slice(0, 3).map(async (org) => {
            console.log(`Fetching repositories for org: ${org.login}`);
            const orgReposUrl = `https://api.github.com/orgs/${org.login}/repos?per_page=${perPage}&sort=pushed`;
            const orgReposResponse = await fetch(orgReposUrl, {
              headers: {
                Authorization: `Bearer ${accessToken}`,
                Accept: "application/vnd.github.v3+json",
                "User-Agent": "Catnip-Worker/1.0",
              },
            });

            if (orgReposResponse.ok) {
              return (await orgReposResponse.json()) as typeof allRepos;
            }
            return [];
          });

          const orgReposResults = await Promise.all(orgReposPromises);
          // Flatten and deduplicate by id
          const orgRepos = orgReposResults.flat();
          const repoIds = new Set(allRepos.map((r) => r.id));
          const uniqueOrgRepos = orgRepos.filter((r) => !repoIds.has(r.id));
          allRepos = [...allRepos, ...uniqueOrgRepos];
          console.log(
            `Total repositories after merging orgs: ${allRepos.length}`,
          );
        }
      }

      // Sort all repos by pushed_at DESC to ensure consistent ordering
      const repos = allRepos.sort(
        (a, b) =>
          new Date(b.pushed_at).getTime() - new Date(a.pushed_at).getTime(),
      );

      // Check each repo for devcontainer and filter by permissions
      const reposWithStatus = await Promise.all(
        repos
          .filter((repo) => !repo.archived && repo.permissions?.push)
          .map(async (repo) => {
            try {
              const devcontainerResponse = await fetch(
                `https://api.github.com/repos/${repo.full_name}/contents/.devcontainer/devcontainer.json`,
                {
                  headers: {
                    Authorization: `Bearer ${accessToken}`,
                    Accept: "application/vnd.github.v3+json",
                    "User-Agent": "Catnip-Worker/1.0",
                  },
                },
              );

              const hasDevcontainer = devcontainerResponse.ok;
              let hasCatnipFeature = false;

              // If devcontainer exists, check if it has catnip feature
              if (hasDevcontainer) {
                try {
                  const contentData = (await devcontainerResponse.json()) as {
                    content?: string;
                  };
                  if (contentData.content) {
                    const content = atob(contentData.content);
                    // Check for both official feature and local development path
                    hasCatnipFeature =
                      content.includes("ghcr.io/wandb/catnip/feature") ||
                      content.includes("./features/feature");
                  }
                } catch (e) {
                  console.warn(
                    `Failed to parse devcontainer for ${repo.full_name}:`,
                    e,
                  );
                }
              } else {
                // Consume response body to prevent stalled HTTP responses
                // This is required in Cloudflare Workers to avoid hitting connection limits
                devcontainerResponse.body?.cancel();
              }

              return {
                id: repo.id,
                name: repo.name,
                full_name: repo.full_name,
                default_branch: repo.default_branch,
                private: repo.private,
                fork: repo.fork,
                has_devcontainer: hasDevcontainer,
                has_catnip_feature: hasCatnipFeature,
              };
            } catch (error) {
              console.warn(
                `Failed to check devcontainer for ${repo.full_name}:`,
                error,
              );
              return {
                id: repo.id,
                name: repo.name,
                full_name: repo.full_name,
                default_branch: repo.default_branch,
                private: repo.private,
                fork: repo.fork,
                has_devcontainer: false,
                has_catnip_feature: false,
              };
            }
          }),
      );

      return c.json({
        repositories: reposWithStatus,
        page,
        per_page: perPage,
      });
    } catch (error) {
      console.error("Repository listing error:", error);
      return c.json({ error: "Internal server error" }, 500);
    }
  });

  // Get user status (codespaces only - repositories are checked client-side)
  app.get("/v1/user/status", requireAuth, async (c) => {
    const username = c.get("username");

    try {
      // Check if user has any codespaces (cheap - already stored in Durable Object)
      let hasAnyCodespaces = false;
      try {
        const codespaceStore = c.env.CODESPACE_STORE.get(
          c.env.CODESPACE_STORE.idFromName("global"),
        );
        const allResponse = await codespaceStore.fetch(
          `https://internal/codespace/${username}?all=true`,
        );
        if (allResponse.ok) {
          const storedCodespaces =
            (await allResponse.json()) as CodespaceCredentials[];
          hasAnyCodespaces = storedCodespaces.length > 0;
        }
      } catch (error) {
        console.warn("Failed to check codespaces:", error);
        // Continue with hasAnyCodespaces = false
      }

      return c.json({
        has_any_codespaces: hasAnyCodespaces,
      });
    } catch (error) {
      console.error("User status error:", error);
      return c.json({ error: "Internal server error" }, 500);
    }
  });

  // Install Catnip feature in a repository
  app.post("/v1/codespace/install", requireAuth, async (c) => {
    const accessToken = c.get("accessToken");
    const username = c.get("username");

    try {
      const body = await c.req.json();
      const { repository, baseBranch, startCodespace = false } = body;

      if (!repository) {
        return c.json({ error: "Repository is required" }, 400);
      }

      console.log(
        `Installing Catnip feature in ${repository} for user ${username}`,
      );

      // 1. Get repository info
      const repoResponse = await fetch(
        `https://api.github.com/repos/${repository}`,
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        },
      );

      if (!repoResponse.ok) {
        const errorText = await repoResponse.text();
        console.error(
          "Failed to fetch repository:",
          repoResponse.status,
          errorText,
        );
        return c.json(
          { error: "Repository not found or access denied" },
          repoResponse.status === 404 ? 404 : 500,
        );
      }

      const repoData = (await repoResponse.json()) as {
        default_branch: string;
        name: string;
      };
      const targetBaseBranch = baseBranch || repoData.default_branch;

      // 2. Check for existing devcontainer.json
      const devcontainerPath = ".devcontainer/devcontainer.json";
      const devcontainerResponse = await fetch(
        `https://api.github.com/repos/${repository}/contents/${devcontainerPath}?ref=${targetBaseBranch}`,
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        },
      );

      let existingContent: string | null = null;
      let existingSha: string | null = null;
      let devcontainerJson: any;

      if (devcontainerResponse.ok) {
        const contentData = (await devcontainerResponse.json()) as {
          content: string;
          sha: string;
        };
        existingContent = atob(contentData.content);
        existingSha = contentData.sha;

        // Check if catnip is already installed
        if (existingContent.includes("ghcr.io/wandb/catnip/feature")) {
          return c.json(
            {
              error: "Catnip feature is already installed in this repository",
              already_installed: true,
            },
            400,
          );
        }

        // Parse existing devcontainer
        try {
          devcontainerJson = JSON.parse(existingContent);
        } catch (e) {
          console.error("Failed to parse existing devcontainer.json:", e);
          return c.json(
            { error: "Existing devcontainer.json is not valid JSON" },
            400,
          );
        }
      } else {
        // Create new devcontainer config
        // Use base Ubuntu image to avoid disk space issues (universal:2 is 30-40GB)
        devcontainerJson = {
          name: "Development Container",
          image: "mcr.microsoft.com/devcontainers/base:ubuntu",
          features: {},
        };
      }

      // Add catnip feature
      if (!devcontainerJson.features) {
        devcontainerJson.features = {};
      }
      devcontainerJson.features["ghcr.io/wandb/catnip/feature:1"] = {};

      // Add port forwarding for Catnip server (port 6369)
      if (!devcontainerJson.forwardPorts) {
        devcontainerJson.forwardPorts = [];
      }
      // Only add port 6369 if it's not already in the list
      if (!devcontainerJson.forwardPorts.includes(6369)) {
        devcontainerJson.forwardPorts.push(6369);
      }

      const newContent = JSON.stringify(devcontainerJson, null, 2) + "\n";

      // 3. Create new branch
      const timestamp = Date.now();
      const branchName = `install-catnip-${timestamp}`;

      // Get base branch SHA
      const baseBranchResponse = await fetch(
        `https://api.github.com/repos/${repository}/git/ref/heads/${targetBaseBranch}`,
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        },
      );

      if (!baseBranchResponse.ok) {
        console.error(
          "Failed to get base branch:",
          baseBranchResponse.status,
          await baseBranchResponse.text(),
        );
        return c.json({ error: "Failed to get base branch" }, 500);
      }

      const baseBranchData = (await baseBranchResponse.json()) as {
        object: { sha: string };
      };
      const baseSha = baseBranchData.object.sha;

      // Create new branch
      const createBranchResponse = await fetch(
        `https://api.github.com/repos/${repository}/git/refs`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            ref: `refs/heads/${branchName}`,
            sha: baseSha,
          }),
        },
      );

      if (!createBranchResponse.ok) {
        console.error(
          "Failed to create branch:",
          createBranchResponse.status,
          await createBranchResponse.text(),
        );
        return c.json({ error: "Failed to create branch" }, 500);
      }

      // 4. Create or update devcontainer.json
      const commitMessage = existingContent
        ? "Add Catnip feature to devcontainer"
        : "Create devcontainer with Catnip feature";

      const updateFileResponse = await fetch(
        `https://api.github.com/repos/${repository}/contents/${devcontainerPath}`,
        {
          method: "PUT",
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            message: commitMessage,
            content: btoa(newContent),
            branch: branchName,
            ...(existingSha && { sha: existingSha }),
          }),
        },
      );

      if (!updateFileResponse.ok) {
        console.error(
          "Failed to update file:",
          updateFileResponse.status,
          await updateFileResponse.text(),
        );
        return c.json({ error: "Failed to commit changes" }, 500);
      }

      // 5. Create pull request
      const prTitle = "Add Catnip Feature";
      const prBody = `# Add Catnip Feature

This PR adds the [Catnip](https://github.com/wandb/catnip) feature to your devcontainer configuration.

Catnip enables agentic coding made fun and productive, accessible from your mobile device.

## Changes
- ${existingContent ? "Updated" : "Created"} \`.devcontainer/devcontainer.json\` to include the Catnip feature
${!existingContent ? "- Using minimal Ubuntu base image to avoid disk space issues (you can customize the image if needed)" : ""}

## Next Steps
1. Review and merge this PR
2. Create a new codespace from this branch or restart your existing codespace
3. Open the Catnip mobile app to connect

${!existingContent ? "## Customization\nIf you need specific development tools, you can change the base image in \`.devcontainer/devcontainer.json\` to:\n- \`mcr.microsoft.com/devcontainers/python:3.12\` for Python development\n- \`mcr.microsoft.com/devcontainers/javascript-node:20\` for Node.js development\n- Or any other [devcontainer image](https://mcr.microsoft.com/catalog?search=devcontainers)\n\n" : ""}---
🤖 This PR was created automatically by Catnip`;

      const createPrResponse = await fetch(
        `https://api.github.com/repos/${repository}/pulls`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            title: prTitle,
            body: prBody,
            head: branchName,
            base: targetBaseBranch,
          }),
        },
      );

      if (!createPrResponse.ok) {
        const errorText = await createPrResponse.text();
        console.error(
          "Failed to create PR:",
          createPrResponse.status,
          errorText,
        );
        return c.json(
          {
            error: "Failed to create pull request",
            details: errorText,
            branch: branchName,
          },
          500,
        );
      }

      const prData = (await createPrResponse.json()) as {
        html_url: string;
        number: number;
      };

      console.log(
        `Successfully created PR #${prData.number} for ${repository}`,
      );

      // 6. Optionally start a codespace
      let codespaceInfo = null;
      if (startCodespace) {
        try {
          const createCodespaceResponse = await fetch(
            `https://api.github.com/repos/${repository}/codespaces`,
            {
              method: "POST",
              headers: {
                Authorization: `Bearer ${accessToken}`,
                Accept: "application/vnd.github.v3+json",
                "User-Agent": "Catnip-Worker/1.0",
                "Content-Type": "application/json",
              },
              body: JSON.stringify({
                ref: branchName,
                location: "WestUs2",
              }),
            },
          );

          if (createCodespaceResponse.ok) {
            const codespaceData = (await createCodespaceResponse.json()) as {
              name: string;
              web_url: string;
            };
            codespaceInfo = {
              name: codespaceData.name,
              url: codespaceData.web_url,
            };
            console.log(
              `Started codespace ${codespaceData.name} for ${repository}`,
            );
          } else {
            console.warn(
              "Failed to start codespace:",
              createCodespaceResponse.status,
              await createCodespaceResponse.text(),
            );
          }
        } catch (error) {
          console.warn("Error starting codespace:", error);
        }
      }

      return c.json({
        success: true,
        pr_url: prData.html_url,
        pr_number: prData.number,
        branch: branchName,
        repository,
        codespace: codespaceInfo,
      });
    } catch (error) {
      console.error("Catnip installation error:", error);
      return c.json({ error: "Internal server error" }, 500);
    }
  });

  // Create a new codespace for a repository
  app.post("/v1/codespace/create", requireAuth, async (c) => {
    const accessToken = c.get("accessToken");
    const username = c.get("username");

    try {
      const body = await c.req.json();
      const { repository, ref } = body;

      if (!repository) {
        return c.json({ error: "Repository is required" }, 400);
      }

      console.log(
        `Creating codespace for ${repository}${ref ? ` on branch ${ref}` : ""} for user ${username}`,
      );

      // 1. Get repository info to determine default branch if ref not specified
      const repoResponse = await fetch(
        `https://api.github.com/repos/${repository}`,
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        },
      );

      if (!repoResponse.ok) {
        const errorText = await repoResponse.text();
        console.error(
          "Failed to fetch repository:",
          repoResponse.status,
          errorText,
        );
        return c.json(
          { error: "Repository not found or access denied" },
          repoResponse.status === 404 ? 404 : 500,
        );
      }

      const repoData = (await repoResponse.json()) as {
        default_branch: string;
        name: string;
      };
      const targetRef = ref || repoData.default_branch;

      console.log(`Creating codespace from ref: ${targetRef}`);

      // 2. Create the codespace
      const createCodespaceResponse = await fetch(
        `https://api.github.com/repos/${repository}/codespaces`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            ref: targetRef,
            location: "WestUs2",
          }),
        },
      );

      if (!createCodespaceResponse.ok) {
        const errorText = await createCodespaceResponse.text();
        console.error(
          "Failed to create codespace:",
          createCodespaceResponse.status,
          errorText,
        );

        // Parse error for better user messaging
        let errorMessage = "Failed to create codespace";
        try {
          const errorData = JSON.parse(errorText);
          if (
            errorData.message &&
            errorData.message.includes("codespace limit")
          ) {
            errorMessage =
              "You've reached your codespace limit. Delete unused codespaces in GitHub.";
          } else if (errorData.message) {
            errorMessage = errorData.message;
          }
        } catch (e) {
          // Use default error message
        }

        // Map status code to known HTTP status codes for type safety
        const statusCode =
          createCodespaceResponse.status === 403
            ? 403
            : createCodespaceResponse.status === 404
              ? 404
              : createCodespaceResponse.status === 422
                ? 422
                : 500;

        return c.json({ error: errorMessage }, statusCode);
      }

      const codespaceData = (await createCodespaceResponse.json()) as {
        id: number;
        name: string;
        state: string;
        web_url: string;
        created_at: string;
      };

      console.log(
        `Created codespace ${codespaceData.name} with state: ${codespaceData.state}`,
      );

      // Return immediately - client will poll for status
      // This avoids Cloudflare Worker 60-second timeout
      return c.json({
        success: true,
        codespace: {
          id: codespaceData.id,
          name: codespaceData.name,
          state: codespaceData.state,
          url: codespaceData.web_url,
          created_at: codespaceData.created_at,
        },
      });
    } catch (error) {
      console.error("Codespace creation error:", error);
      return c.json({ error: "Internal server error" }, 500);
    }
  });

  // Get codespace status endpoint (for polling)
  app.get("/v1/codespace/status/:name", requireAuth, async (c) => {
    const accessToken = c.get("accessToken");
    const username = c.get("username");
    const codespaceName = c.req.param("name");

    if (!codespaceName) {
      return c.json({ error: "Codespace name is required" }, 400);
    }

    try {
      console.log(`Checking status for codespace: ${codespaceName}`);

      const statusResponse = await fetch(
        `https://api.github.com/user/codespaces/${codespaceName}`,
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
            Accept: "application/vnd.github.v3+json",
            "User-Agent": "Catnip-Worker/1.0",
          },
        },
      );

      if (!statusResponse.ok) {
        const errorText = await statusResponse.text();
        console.error(
          `Failed to get codespace status: ${statusResponse.status}`,
          errorText,
        );

        if (statusResponse.status === 404) {
          return c.json({ error: "Codespace not found" }, 404);
        }

        return c.json({ error: "Failed to retrieve codespace status" }, 500);
      }

      const codespaceData = (await statusResponse.json()) as {
        id: number;
        name: string;
        state: string;
        web_url: string;
        created_at: string;
      };

      console.log(`Codespace ${codespaceName} status: ${codespaceData.state}`);

      // Check if we have credentials stored for this codespace
      let hasCredentials = false;
      try {
        const codespaceStore = c.env.CODESPACE_STORE.get(
          c.env.CODESPACE_STORE.idFromName("global"),
        );
        const credentialsResponse = await codespaceStore.fetch(
          `https://internal/codespace/${username}/${codespaceName}`,
        );
        if (credentialsResponse.ok) {
          const credentials =
            (await credentialsResponse.json()) as CodespaceCredentials;
          hasCredentials = !!credentials.githubToken;
          console.log(
            `Credentials check for ${codespaceName}: ${hasCredentials ? "found" : "not found"}`,
          );
        }
      } catch (error) {
        console.warn(
          `Failed to check credentials for ${codespaceName}:`,
          error,
        );
        // Continue without credentials info - don't fail the whole request
      }

      return c.json({
        success: true,
        codespace: {
          id: codespaceData.id,
          name: codespaceData.name,
          state: codespaceData.state,
          url: codespaceData.web_url,
          created_at: codespaceData.created_at,
        },
        has_credentials: hasCredentials,
      });
    } catch (error) {
      console.error("Codespace status check error:", error);
      return c.json({ error: "Internal server error" }, 500);
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

      // Handle codespace deletion events
      if (eventName === "codespace" && event.action === "deleted") {
        try {
          const codespaceName = event.codespace?.name;
          const username = event.codespace?.owner?.login;

          if (codespaceName && username) {
            console.log(
              `🗑️ Webhook: Codespace ${codespaceName} deleted for user ${username}`,
            );

            // Remove from our store
            const codespaceStore = c.env.CODESPACE_STORE.get(
              c.env.CODESPACE_STORE.idFromName("global"),
            );

            const deleteResponse = await codespaceStore.fetch(
              `https://internal/codespace/${username}/${codespaceName}`,
              { method: "DELETE" },
            );

            if (deleteResponse.ok) {
              console.log(
                `✅ Successfully removed deleted codespace ${codespaceName} from store`,
              );
            } else {
              console.warn(
                `⚠️ Failed to remove codespace ${codespaceName}: ${deleteResponse.status}`,
              );
            }
          } else {
            console.warn("⚠️ Codespace deletion webhook missing name or owner");
          }
        } catch (error) {
          console.error("Error handling codespace deletion webhook:", error);
          // Don't fail the webhook - return success anyway
        }
      }

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
              const rawCodespaces =
                (await allResponse.json()) as CodespaceCredentials[];

              // Verify codespaces still exist and clean up deleted ones
              storedCodespaces = await verifyAndCleanCodespaces(
                rawCodespaces,
                accessToken,
                username,
                codespaceStore,
              );

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
        let codespaceWasAlreadyRunning = false;

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
                codespaceWasAlreadyRunning =
                  targetCodespace.state === "Available";
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
            const errorText = await startResponse.text();

            // 409 "already running" is fine - the codespace is starting/running
            // This can happen during state transitions (e.g., ShuttingDown -> Starting)
            if (startResponse.status === 409) {
              console.log(
                "Codespace already running or starting (409), continuing with health check:",
                errorText,
              );
              // Continue to health check - don't return
            } else {
              // Other errors are real failures
              console.error(
                "Failed to start codespace:",
                startResponse.status,
                errorText,
              );
              sendEvent("error", {
                message:
                  "Failed to start codespace. Please ensure you have codespace permissions.",
              });
              void writer.close();
              return;
            }
          }

          // Give the codespace time to start and send fresh credentials
          sendEvent("status", {
            message: "Setting up codespace environment",
            step: "setup",
          });
          await new Promise((resolve) => setTimeout(resolve, 8000)); // Wait for startup
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
            // Give time for fresh credentials to propagate
            sendEvent("status", {
              message: "Waiting for fresh credentials to propagate",
              step: "initializing",
            });
            // Reduced wait: 3s if already running, 5s if just started
            const propagationWait = codespaceWasAlreadyRunning ? 3000 : 5000;
            await new Promise((resolve) =>
              setTimeout(resolve, propagationWait),
            );
          } else {
            console.log("Using original stored credentials for health check");
            // Give catnip a moment to be ready for health check
            sendEvent("status", {
              message: "Waiting for catnip to be ready",
              step: "initializing",
            });
            // Minimal wait if already running, short wait if just started
            const readyWait = codespaceWasAlreadyRunning ? 500 : 2000;
            await new Promise((resolve) => setTimeout(resolve, readyWait));
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
            {
              hasFreshCredentials: credentialsRefreshed,
              isAlreadyRunning: codespaceWasAlreadyRunning,
            },
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
        "codespace",
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
    const username = c.get("username");
    const accessToken = c.get("accessToken");

    // If we have session data from mobile middleware, we're authenticated
    if (session || (username && accessToken)) {
      // Check if expired for regular sessions
      if (session && Date.now() > session.expiresAt) {
        throw new HTTPException(401, { message: "Session expired" });
      }
      return next();
    }

    throw new HTTPException(401, { message: "Authentication required" });
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
    const isMainDomain =
      url.hostname === "catnip.run" || url.hostname.endsWith(".catnip.run");

    // Check for mobile app requests with codespace name header
    const codespaceName = c.req.header("X-Codespace-Name");
    // Mobile requests should check route patterns directly, not shouldRouteToContainer
    // (which is disabled for web to avoid building/uploading containers)
    const matchesContainerRoute = CONTAINER_ROUTES.some((pattern) =>
      pattern.test(url.pathname),
    );
    const isMobileCodespaceRequest =
      isMainDomain && codespaceName && matchesContainerRoute;

    if (isMobileCodespaceRequest) {
      console.log(
        `🐱 [Mobile] Proxying request for ${codespaceName}: ${url.pathname}`,
      );

      try {
        // Get stored codespace credentials
        const codespaceStore = c.env.CODESPACE_STORE.get(
          c.env.CODESPACE_STORE.idFromName("global"),
        );

        const username = c.get("username");
        if (!username) {
          console.error(
            "🐱 [Mobile] ❌ No username found for mobile codespace request",
          );
          return c.text("Authentication required", 401);
        }

        console.log(`🐱 [Mobile] Authenticated user: ${username}`);

        // Get specific codespace credentials by username and codespace name
        const credentialsResponse = await codespaceStore.fetch(
          `https://internal/codespace/${username}/${codespaceName}`,
        );

        if (!credentialsResponse.ok) {
          console.error(
            `🐱 [Mobile] ❌ No credentials found for user: ${username}, codespace: ${codespaceName}`,
          );
          return c.text("Codespace credentials not found", 404);
        }

        const credentials = (await credentialsResponse.json()) as {
          codespaceName: string;
          githubToken: string;
        };

        console.log(
          `🐱 [Mobile] ✅ Found credentials for ${username}/${codespaceName}`,
        );

        // Check if we have a valid token
        if (!credentials.githubToken || credentials.githubToken === "") {
          console.error(
            `🐱 [Mobile] ❌ No valid GitHub token for ${username}/${codespaceName}`,
          );
          return c.text(
            "Codespace credentials expired - please reconnect",
            401,
          );
        }

        console.log(
          `🐱 [Mobile] Token preview: ${credentials.githubToken.substring(0, 7)}...`,
        );

        // Proxy to codespace - check if this is a WebSocket upgrade
        const isWebSocket =
          c.req.header("Upgrade")?.toLowerCase() === "websocket";

        // Always use https:// - WebSocket upgrade is handled by the Upgrade headers
        const codespaceUrl = `https://${codespaceName}-6369.app.github.dev`;
        const proxyUrl = `${codespaceUrl}${url.pathname}${url.search}`;

        console.log(
          `🐱 [Mobile] Proxying to: ${proxyUrl}${isWebSocket ? " (WebSocket)" : ""}`,
        );

        // For WebSocket upgrades, create a WebSocketPair to tunnel the connection
        if (isWebSocket) {
          console.log("🐱 [Mobile] Setting up WebSocket tunnel");

          // Create a WebSocketPair - one end for the client, one for our backend connection
          const pair = new WebSocketPair();
          const [client, server] = Object.values(pair);

          // Accept the client side of the pair
          server.accept();

          // Build clean headers for backend - only include WebSocket upgrade headers
          const backendHeaders = new Headers();

          // Copy WebSocket upgrade headers from original request
          const upgradeHeaders = [
            "Upgrade",
            "Connection",
            "Sec-WebSocket-Key",
            "Sec-WebSocket-Version",
            "Sec-WebSocket-Extensions",
            "Sec-WebSocket-Protocol",
          ];
          for (const header of upgradeHeaders) {
            const value = c.req.header(header);
            if (value) {
              backendHeaders.set(header, value);
            }
          }

          // Add GitHub authentication header (backend expects this, not Authorization)
          backendHeaders.set("X-Github-Token", credentials.githubToken);
          backendHeaders.set("User-Agent", "Catnip-Mobile/1.0");

          console.log(
            "🐱 [Mobile] Backend headers:",
            Array.from(backendHeaders.entries())
              .map(
                ([k, v]) =>
                  `${k}: ${k === "X-Github-Token" ? v.substring(0, 7) + "..." : v}`,
              )
              .join(", "),
          );

          const backendResponse = await fetch(proxyUrl, {
            headers: backendHeaders,
          });

          console.log(
            `🐱 [Mobile] Backend fetch response status: ${backendResponse.status}`,
          );

          if (backendResponse.status !== 101) {
            console.error(
              `🐱 [Mobile] ❌ Backend WebSocket upgrade failed: ${backendResponse.status}`,
            );
            server.close(1011, "Backend connection failed");
            return c.text("Failed to connect to backend", 502);
          }

          const backendWebSocket = backendResponse.webSocket;
          if (!backendWebSocket) {
            console.error(
              "🐱 [Mobile] ❌ Backend upgrade succeeded but no webSocket property",
            );
            server.close(1011, "Backend connection failed");
            return c.text("Backend WebSocket unavailable", 502);
          }

          console.log("🐱 [Mobile] Got backend WebSocket");

          // IMPORTANT: WebSockets from fetch() also need accept() in Cloudflare Workers
          backendWebSocket.accept();
          console.log("🐱 [Mobile] Accepted backend WebSocket");

          // Pipe messages from client (via server) to backend
          // Use coupleWebSocket to automatically forward messages bidirectionally
          server.addEventListener("message", (event: MessageEvent) => {
            try {
              if (
                backendWebSocket.readyState === WebSocket.OPEN ||
                backendWebSocket.readyState === WebSocket.CONNECTING
              ) {
                backendWebSocket.send(event.data);
              }
            } catch (error) {
              console.error(
                "🐱 [Mobile] ❌ Error forwarding to backend:",
                error,
              );
            }
          });

          // Pipe messages from backend to client (via server)
          backendWebSocket.addEventListener(
            "message",
            (event: MessageEvent) => {
              try {
                if (
                  server.readyState === WebSocket.OPEN ||
                  server.readyState === WebSocket.CONNECTING
                ) {
                  server.send(event.data);
                }
              } catch (error) {
                console.error(
                  "🐱 [Mobile] ❌ Error forwarding to client:",
                  error,
                );
              }
            },
          );

          // Handle close events
          server.addEventListener("close", (event: CloseEvent) => {
            console.log(
              `🐱 [Mobile] Client WebSocket closed: code=${event.code}, reason=${event.reason}`,
            );
            try {
              backendWebSocket.close();
            } catch (error) {
              console.error(
                "🐱 [Mobile] Error closing backend WebSocket:",
                error,
              );
            }
          });

          backendWebSocket.addEventListener("close", (event: CloseEvent) => {
            console.log(
              `🐱 [Mobile] Backend WebSocket closed: code=${event.code}, reason=${event.reason || "(no reason)"}`,
            );
            try {
              server.close();
            } catch (error) {
              console.error(
                "🐱 [Mobile] Error closing server WebSocket:",
                error,
              );
            }
          });

          // Handle errors
          server.addEventListener("error", (event: ErrorEvent) => {
            console.error("🐱 [Mobile] Client WebSocket error:", event.error);
            backendWebSocket.close();
          });

          backendWebSocket.addEventListener("error", (event: ErrorEvent) => {
            console.error("🐱 [Mobile] Backend WebSocket error:", event.error);
            server.close();
          });

          console.log("🐱 [Mobile] ✅ WebSocket tunnel established");

          // Return the client side of the pair
          return new Response(null, {
            status: 101,
            webSocket: client,
          });
        }

        // For regular HTTP requests, proxy normally
        const proxyHeaders = new Headers(c.req.raw.headers);
        proxyHeaders.set("X-Github-Token", credentials.githubToken);
        proxyHeaders.set("User-Agent", "Catnip-Mobile/1.0");
        proxyHeaders.delete("X-Codespace-Name"); // Remove our custom header

        const proxyResponse = await fetch(proxyUrl, {
          method: c.req.method,
          headers: proxyHeaders,
          body:
            c.req.method !== "GET" && c.req.method !== "HEAD"
              ? c.req.raw.body
              : undefined,
        });

        console.log(
          `🐱 [Mobile] ✅ Proxy response status: ${proxyResponse.status}`,
        );
        return proxyResponse;
      } catch (error) {
        console.error("🐱 [Mobile] ❌ Error proxying to codespace:", error);
        return c.text("Failed to connect to codespace", 500);
      }
    }

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
