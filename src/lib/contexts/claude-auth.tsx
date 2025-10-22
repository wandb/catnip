import { createContext } from "react";

interface ClaudeSettings {
  theme: string;
  isAuthenticated: boolean;
  version?: string;
  hasCompletedOnboarding: boolean;
  numStartups: number;
  notificationsEnabled: boolean;
}

interface ClaudeAuthContextType {
  settings: ClaudeSettings | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  showAuthModal: boolean;
  setShowAuthModal: (show: boolean) => void;
  checkAuthStatus: () => Promise<void>;
  resetAuthState: () => Promise<void>;
}

export const ClaudeAuthContext = createContext<
  ClaudeAuthContextType | undefined
>(undefined);
