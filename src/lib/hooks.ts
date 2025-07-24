import { use } from "react";
import { AuthContext } from "./contexts/auth";
import { WebSocketContext } from "./contexts/websocket";

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
