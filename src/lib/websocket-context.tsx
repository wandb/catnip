import { useState, type ReactNode } from "react";
import { WebSocketContext } from "./contexts/websocket";

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const [isConnected, setIsConnected] = useState(false);

  return (
    <WebSocketContext value={{ isConnected, setIsConnected }}>
      {children}
    </WebSocketContext>
  );
}
