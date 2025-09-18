// Mobile OAuth relay endpoints
import { Context } from "hono";
import { setCookie } from "hono/cookie";

interface MobileSessionData {
  sessionId: string;
  userId: string;
  username: string;
  mobileToken: string;
  createdAt: number;
  expiresAt: number;
}

/**
 * Generate a secure mobile session token
 */
export function generateMobileToken(): string {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return btoa(String.fromCharCode(...array))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
}

/**
 * Mobile OAuth initiation endpoint
 * This endpoint is called by the mobile app to start the OAuth flow
 */
export async function initiateMobileOAuth(c: Context) {
  const redirectUri = c.req.query("redirect_uri") || "catnip://auth";
  const state = c.req.query("state") || crypto.randomUUID();

  // Store the mobile OAuth state in a temporary cookie
  setCookie(c, "mobile-oauth-state", JSON.stringify({
    redirectUri,
    state,
    initiated: Date.now()
  }), {
    httpOnly: true,
    secure: true,
    sameSite: "Lax",
    maxAge: 10 * 60, // 10 minutes
    path: "/"
  });

  // Redirect to the standard GitHub OAuth endpoint
  const currentUrl = new URL(c.req.url);
  const githubAuthUrl = `${currentUrl.protocol}//${currentUrl.host}/v1/auth/github?mobile=true`;

  return c.redirect(githubAuthUrl);
}

/**
 * Mobile OAuth callback handler
 * This is called after successful GitHub OAuth to relay the session to the mobile app
 */
export async function handleMobileOAuthCallback(c: Context, sessionId: string, sessionData: any) {
  const mobileState = c.req.cookie("mobile-oauth-state");

  if (!mobileState) {
    // Not a mobile OAuth flow, continue normally
    return null;
  }

  try {
    const { redirectUri, state } = JSON.parse(mobileState.value);

    // Generate a mobile-specific session token
    const mobileToken = generateMobileToken();

    // Store the mobile session mapping in the Durable Object
    const sessionDO = c.env.SESSIONS.get(c.env.SESSIONS.idFromName("global"));

    // Create mobile session record
    const mobileSession: MobileSessionData = {
      sessionId,
      userId: sessionData.userId,
      username: sessionData.username,
      mobileToken,
      createdAt: Date.now(),
      expiresAt: sessionData.expiresAt
    };

    // Store mobile session mapping
    await sessionDO.fetch(`https://internal/mobile-session/${mobileToken}`, {
      method: "PUT",
      body: JSON.stringify(mobileSession)
    });

    // Clear the mobile OAuth state cookie
    setCookie(c, "mobile-oauth-state", "", {
      httpOnly: true,
      secure: true,
      sameSite: "Lax",
      maxAge: 0,
      path: "/"
    });

    // Build the redirect URL with the mobile token
    const redirectUrl = new URL(redirectUri);
    redirectUrl.searchParams.set("token", mobileToken);
    redirectUrl.searchParams.set("state", state);
    redirectUrl.searchParams.set("username", sessionData.username);

    // Redirect to the mobile app
    return c.redirect(redirectUrl.toString());
  } catch (error) {
    console.error("Mobile OAuth callback error:", error);
    return null;
  }
}

/**
 * Mobile session validation middleware
 * Validates mobile session tokens sent in Authorization header
 */
export async function validateMobileSession(c: Context): Promise<any> {
  const authHeader = c.req.header("Authorization");

  if (!authHeader?.startsWith("Bearer ")) {
    return null;
  }

  const mobileToken = authHeader.substring(7);

  try {
    // Get the mobile session from Durable Object
    const sessionDO = c.env.SESSIONS.get(c.env.SESSIONS.idFromName("global"));
    const response = await sessionDO.fetch(`https://internal/mobile-session/${mobileToken}`);

    if (!response.ok) {
      return null;
    }

    const mobileSession = await response.json() as MobileSessionData;

    // Check if mobile session is expired
    if (Date.now() > mobileSession.expiresAt) {
      // Delete expired session
      await sessionDO.fetch(`https://internal/mobile-session/${mobileToken}`, {
        method: "DELETE"
      });
      return null;
    }

    // Get the actual session data
    const sessionResponse = await sessionDO.fetch(`https://internal/session/${mobileSession.sessionId}`);

    if (!sessionResponse.ok) {
      return null;
    }

    const sessionData = await sessionResponse.json();

    // Set the session data in context for use by other handlers
    c.set("session", sessionData);
    c.set("sessionId", mobileSession.sessionId);
    c.set("userId", sessionData.userId);
    c.set("username", sessionData.username);
    c.set("accessToken", sessionData.accessToken);
    c.set("mobileToken", mobileToken);

    return sessionData;
  } catch (error) {
    console.error("Mobile session validation error:", error);
    return null;
  }
}

/**
 * Mobile logout endpoint
 * Revokes the mobile session token
 */
export async function handleMobileLogout(c: Context) {
  const mobileToken = c.get("mobileToken");

  if (mobileToken) {
    const sessionDO = c.env.SESSIONS.get(c.env.SESSIONS.idFromName("global"));
    await sessionDO.fetch(`https://internal/mobile-session/${mobileToken}`, {
      method: "DELETE"
    });
  }

  return c.json({ success: true });
}