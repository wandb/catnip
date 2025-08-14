import { useEffect, useState, type ReactNode } from "react";
import { AuthContext } from "./contexts/auth";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [username, setUsername] = useState<string>();
  const [userId, setUserId] = useState<string>();
  const [catnipProxy, setCatnipProxy] = useState<string>();
  const [authRequired, setAuthRequired] = useState(false);

  const exchangeTokenIfPresent = async () => {
    // Check for token in URL query parameters
    const urlParams = new URLSearchParams(window.location.search);
    const token = urlParams.get("token");

    if (token) {
      try {
        console.log("Found token in URL, exchanging for session cookie...");
        const response = await fetch(
          `/v1/auth/token?token=${encodeURIComponent(token)}`,
          {
            method: "POST",
          },
        );

        if (response.ok) {
          const result = await response.json();
          console.log("Token exchange successful:", result);

          // Remove token from URL without refreshing the page
          const newUrl = new URL(window.location.href);
          newUrl.searchParams.delete("token");
          window.history.replaceState({}, document.title, newUrl.toString());

          return true; // Exchange successful
        } else {
          console.error("Token exchange failed:", await response.text());
        }
      } catch (error) {
        console.error("Error during token exchange:", error);
      }
    }

    return false; // No token or exchange failed
  };

  const checkAuth = async () => {
    try {
      // First try to exchange any token present in the URL
      await exchangeTokenIfPresent();

      // Then check settings to see if we're in auth mode
      const settingsRes = await fetch("/v1/settings");
      if (settingsRes.ok) {
        const settings = await settingsRes.json();
        setCatnipProxy(settings.catnipProxy);
        setAuthRequired(settings.authRequired);

        // Only check auth status if auth is required
        if (settings.authRequired) {
          const authRes = await fetch("/v1/auth/github/status");
          if (authRes.ok) {
            const authData = await authRes.json();

            // Check if user is authenticated (has a valid session cookie or GitHub auth)
            const isValidAuth =
              authData.status === "authenticated" ||
              authData.status === "success";
            setIsAuthenticated(isValidAuth);

            if (authData.user) {
              setUsername(authData.user.username);
              setUserId(authData.user.username); // Using username as userId for now
            }
          } else {
            setIsAuthenticated(false);
          }
        } else {
          // If auth is not required (local mode), consider user authenticated
          setIsAuthenticated(true);
        }
      }
    } catch (error) {
      console.error("Failed to check auth status:", error);
      setIsAuthenticated(false);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void checkAuth();
  }, []);

  return (
    <AuthContext
      value={{
        isAuthenticated,
        isLoading,
        username,
        userId,
        catnipProxy,
        authRequired,
        checkAuth,
      }}
    >
      {children}
    </AuthContext>
  );
}
