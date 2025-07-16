import { createContext } from "react";

interface WebSocketContextType {
  isConnected: boolean;
  setIsConnected: (connected: boolean) => void;
}

export const WebSocketContext = createContext<WebSocketContextType | undefined>(
  undefined,
);
