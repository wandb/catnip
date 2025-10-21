import { use } from "react";
import { AuthContext } from "./contexts/auth";
import { WebSocketContext } from "./contexts/websocket";
import { GitHubAuthContext } from "./contexts/github-auth";
import { ClaudeAuthContext } from "./contexts/claude-auth";

export function useAuth() {
  const context = use(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return context;
}

export function useWebSocket() {
  const context = use(WebSocketContext);
  if (context === undefined) {
    throw new Error("useWebSocket must be used within a WebSocketProvider");
  }
  return context;
}

export function useGitHubAuth() {
  const context = use(GitHubAuthContext);
  if (context === undefined) {
    throw new Error("useGitHubAuth must be used within a GitHubAuthProvider");
  }
  return context;
}

export function useClaudeAuth() {
  const context = use(ClaudeAuthContext);
  if (context === undefined) {
    throw new Error("useClaudeAuth must be used within a ClaudeAuthProvider");
  }
  return context;
}
