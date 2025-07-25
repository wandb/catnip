import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

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

const GitHubAuthContext = createContext<GitHubAuthContextType | undefined>(
  undefined,
);

export function GitHubAuthProvider({ children }: { children: ReactNode }) {
  const [authStatus, setAuthStatus] = useState<GitHubAuthStatus>({
    status: "none",
  });
  const [isLoading, setIsLoading] = useState(true);
  const [showAuthModal, setShowAuthModal] = useState(false);

  const checkAuthStatus = async () => {
    try {
      const response = await fetch("/v1/auth/github/status");
      if (response.ok) {
        const data: GitHubAuthStatus = await response.json();
        setAuthStatus(data);
      }
    } catch (error) {
      console.error("Failed to check GitHub auth status:", error);
      setAuthStatus({ status: "none" });
    } finally {
      setIsLoading(false);
    }
  };

  const resetAuthState = async () => {
    try {
      await fetch("/v1/auth/github/reset", { method: "POST" });
      setAuthStatus({ status: "none" });
    } catch (error) {
      console.error("Failed to reset GitHub auth state:", error);
    }
  };

  const isAuthenticated = authStatus.status === "authenticated";

  useEffect(() => {
    void checkAuthStatus();
  }, []);

  // Auto-show modal logic
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      const dismissed = localStorage.getItem("github-auth-dismissed");
      if (!dismissed) {
        setShowAuthModal(true);
      }
    }
  }, [isLoading, isAuthenticated]);

  return (
    <GitHubAuthContext.Provider
      value={{
        authStatus,
        isAuthenticated,
        isLoading,
        showAuthModal,
        setShowAuthModal,
        checkAuthStatus,
        resetAuthState,
      }}
    >
      {children}
    </GitHubAuthContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useGitHubAuth() {
  const context = useContext(GitHubAuthContext);
  if (context === undefined) {
    throw new Error("useGitHubAuth must be used within a GitHubAuthProvider");
  }
  return context;
}
