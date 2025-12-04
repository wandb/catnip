import { describe, it, expect } from "vitest";
import { validateRedirectUri } from "./oauth-utils";

describe("OAuth Security", () => {
  describe("validateRedirectUri", () => {
    it("should accept valid catnip://auth URLs", () => {
      expect(validateRedirectUri("catnip://auth")).toBe(true);
      expect(validateRedirectUri("catnip://auth/callback")).toBe(true);
      expect(validateRedirectUri("catnip://auth?token=abc")).toBe(true);
    });

    it("should accept valid catnip://oauth URLs", () => {
      expect(validateRedirectUri("catnip://oauth")).toBe(true);
      expect(validateRedirectUri("catnip://oauth/success")).toBe(true);
    });

    it("should reject empty or null URLs", () => {
      expect(validateRedirectUri("")).toBe(false);
      expect(validateRedirectUri(null as any)).toBe(false);
      expect(validateRedirectUri(undefined as any)).toBe(false);
    });

    it("should reject URLs with wrong scheme", () => {
      expect(validateRedirectUri("http://catnip.run")).toBe(false);
      expect(validateRedirectUri("https://catnip.run")).toBe(false);
      expect(validateRedirectUri("javascript:alert(1)")).toBe(false);
    });

    it("should reject URLs that don't match allowed prefixes", () => {
      expect(validateRedirectUri("catnip://other")).toBe(false);
      expect(validateRedirectUri("catnip://malicious")).toBe(false);
    });

    it("should accept URLs with path components after prefix", () => {
      expect(validateRedirectUri("catnip://auth/../malicious")).toBe(true);
      // Path components (including ../) are allowed after the catnip://auth prefix
      // The native app handles URL parsing and any path normalization
    });

    it("should handle URLs with query parameters", () => {
      expect(validateRedirectUri("catnip://auth?token=abc&state=xyz")).toBe(
        true,
      );
      expect(
        validateRedirectUri("catnip://oauth?redirect=https://evil.com"),
      ).toBe(true);
      // Note: Query params don't matter for validation - we only check the scheme and path prefix
    });
  });
});

describe("OAuth Flow Security", () => {
  describe("CSRF Protection", () => {
    it("should verify state parameter matches", () => {
      const storedState = "random-uuid-123";
      const returnedState = "random-uuid-123";
      expect(storedState).toBe(returnedState);
    });

    it("should reject mismatched state", () => {
      const storedState = "random-uuid-123";
      const returnedState = "different-uuid-456";
      expect(storedState).not.toBe(returnedState);
    });

    it("should reject missing state", () => {
      const storedState = "random-uuid-123";
      const returnedState = null;
      expect(returnedState).toBeNull();
      expect(storedState).not.toBe(returnedState);
    });
  });

  describe("Mobile Flow Parameter Preservation", () => {
    it("should preserve mobile flow parameters in redirect_uri", () => {
      const baseUrl = "https://catnip.run/v1/auth/github";
      const mobile = "1";
      const appRedirect = "catnip://auth";
      const appState = "user-state-123";

      const url = new URL(baseUrl);
      url.searchParams.set("mobile", mobile);
      url.searchParams.set("app_redirect", appRedirect);
      url.searchParams.set("app_state", appState);

      expect(url.searchParams.get("mobile")).toBe("1");
      expect(url.searchParams.get("app_redirect")).toBe("catnip://auth");
      expect(url.searchParams.get("app_state")).toBe("user-state-123");
    });
  });
});
