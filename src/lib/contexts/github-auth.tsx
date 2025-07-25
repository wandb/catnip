import { createContext } from "react";

interface GitHubUser {
  username: string;
  scopes: string[];
}

interface GitHubAuthStatus {
  status:
    | "none"
    | "pending"
    | "waiting"
    | "success"
    | "error"
    | "authenticated";
  error?: string;
  user?: GitHubUser;
}

interface GitHubAuthContextType {
  authStatus: GitHubAuthStatus;
  isAuthenticated: boolean;
  isLoading: boolean;
  showAuthModal: boolean;
  setShowAuthModal: (show: boolean) => void;
  checkAuthStatus: () => Promise<void>;
  resetAuthState: () => Promise<void>;
}

export const GitHubAuthContext = createContext<
  GitHubAuthContextType | undefined
>(undefined);
