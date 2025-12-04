// OAuth security constants
export const ALLOWED_REDIRECT_SCHEMES = ["catnip://"];
export const ALLOWED_REDIRECT_PREFIXES = ["catnip://auth", "catnip://oauth"];
export const OAUTH_RATE_LIMIT_MAX_REQUESTS = 10; // Maximum requests per window
export const OAUTH_RATE_LIMIT_WINDOW_MS = 5 * 60 * 1000; // 5 minutes

/**
 * Validates that a redirect URI matches our allowlist of safe schemes and prefixes.
 * Only catnip:// URLs with specific prefixes are allowed to prevent open redirects.
 *
 * @param redirectUri - The URI to validate
 * @returns true if the URI is valid and safe, false otherwise
 */
export function validateRedirectUri(redirectUri: string): boolean {
  if (!redirectUri) return false;
  return ALLOWED_REDIRECT_PREFIXES.some((prefix) =>
    redirectUri.startsWith(prefix),
  );
}
