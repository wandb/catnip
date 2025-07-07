import { createContext, useContext, useState, ReactNode } from 'react'

interface WebSocketContextType {
  isConnected: boolean
  setIsConnected: (connected: boolean) => void
}

const WebSocketContext = createContext<WebSocketContextType | undefined>(undefined)

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const [isConnected, setIsConnected] = useState(false)

  return (
    <WebSocketContext.Provider value={{ isConnected, setIsConnected }}>
      {children}
    </WebSocketContext.Provider>
  )
}

export function useWebSocket() {
  const context = useContext(WebSocketContext)
  if (context === undefined) {
    throw new Error('useWebSocket must be used within a WebSocketProvider')
  }
  return context
}