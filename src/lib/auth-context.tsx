import { useEffect, useState, type ReactNode } from "react";
import { AuthContext } from "./contexts/auth";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [username, setUsername] = useState<string>();
  const [userId, setUserId] = useState<string>();
  const [catnipProxy, setCatnipProxy] = useState<string>();
  const [authRequired, setAuthRequired] = useState(false);

  const checkAuth = async () => {
    try {
      // First check settings to see if we're in proxy mode
      const settingsRes = await fetch("/v1/settings");
      if (settingsRes.ok) {
        const settings = await settingsRes.json();
        setCatnipProxy(settings.catnipProxy);
        setAuthRequired(settings.authRequired);

        // Only check auth status if auth is required
        if (settings.authRequired) {
          const authRes = await fetch("/v1/auth/status");
          if (authRes.ok) {
            const authData = await authRes.json();
            setIsAuthenticated(authData.authenticated);
            setUsername(authData.username);
            setUserId(authData.userId);
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
