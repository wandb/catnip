import { createContext } from "react";

interface AuthContextType {
  isAuthenticated: boolean;
  isLoading: boolean;
  username?: string;
  userId?: string;
  catnipProxy?: string;
  authRequired: boolean;
  checkAuth: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextType | undefined>(
  undefined,
);
