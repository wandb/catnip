import { useEffect, useState, type ReactNode } from "react";
import { ClaudeAuthContext } from "./contexts/claude-auth";

interface ClaudeSettings {
  theme: string;
  isAuthenticated: boolean;
  version?: string;
  hasCompletedOnboarding: boolean;
  numStartups: number;
  notificationsEnabled: boolean;
}

export function ClaudeAuthProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<ClaudeSettings | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [showAuthModal, setShowAuthModal] = useState(false);

  const checkAuthStatus = async () => {
    try {
      const response = await fetch("/v1/claude/settings");
      if (response.ok) {
        const data: ClaudeSettings = await response.json();
        setSettings(data);
      } else {
        // If settings endpoint fails, assume not authenticated
        setSettings({
          theme: "dark",
          isAuthenticated: false,
          hasCompletedOnboarding: false,
          numStartups: 0,
          notificationsEnabled: false,
        });
      }
    } catch (error) {
      console.error("Failed to check Claude auth status:", error);
      setSettings({
        theme: "dark",
        isAuthenticated: false,
        hasCompletedOnboarding: false,
        numStartups: 0,
        notificationsEnabled: false,
      });
    } finally {
      setIsLoading(false);
    }
  };

  const resetAuthState = async () => {
    try {
      await fetch("/v1/claude/onboarding/cancel", { method: "POST" });
      // Refresh settings after reset
      await checkAuthStatus();
    } catch (error) {
      console.error("Failed to reset Claude auth state:", error);
    }
  };

  const isAuthenticated = settings?.isAuthenticated ?? false;

  useEffect(() => {
    void checkAuthStatus();
  }, []);

  // Auto-show modal logic
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      const dismissed = localStorage.getItem("claude-auth-dismissed");
      if (!dismissed) {
        setShowAuthModal(true);
      }
    }
  }, [isLoading, isAuthenticated]);

  return (
    <ClaudeAuthContext
      value={{
        settings,
        isAuthenticated,
        isLoading,
        showAuthModal,
        setShowAuthModal,
        checkAuthStatus,
        resetAuthState,
      }}
    >
      {children}
    </ClaudeAuthContext>
  );
}
