import { createRootRoute, Link, Outlet } from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { WebSocketProvider, useWebSocket } from '@/lib/websocket-context'
import { useState, useEffect, useRef } from 'react'
import { Home, Terminal, Settings, RotateCcw } from 'lucide-react'

function RootLayout() {
  const { isConnected } = useWebSocket()
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  const handleReset = () => {
    // Call the reset function if it exists (on terminal page)
    if ((window as any).resetTerminal) {
      (window as any).resetTerminal()
    }
    setDropdownOpen(false)
  }

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setDropdownOpen(false)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [])

  return (
    <>
      <div className="min-h-screen bg-background flex">
        {/* Vertical Sidebar */}
        <nav className="w-16 bg-[#1a1a1a] flex flex-col">
          {/* Cat Logo with Connection Status */}
          <div className="flex items-center justify-center h-16 border-b border-gray-800 relative">
            <Link to="/" className="text-2xl">
              🐱
            </Link>
            {/* Connection Status - positioned at cat's right ear */}
            <div
              className={`absolute top-2 right-2 h-2 w-2 rounded-full ${
                isConnected
                  ? 'bg-green-500 shadow-green-500/50 animate-pulse'
                  : 'bg-red-500'
              }`}
              title={isConnected ? 'Connected' : 'Disconnected'}
            />
          </div>
          
          {/* Navigation Items */}
          <div className="flex-1 py-4">
            <div className="flex flex-col space-y-2">
              <Link
                to="/"
                className="flex items-center justify-center h-12 text-gray-300 hover:text-white hover:bg-gray-800 transition-colors rounded mx-2"
                title="Home"
              >
                <Home size={20} />
              </Link>
              <Link
                to="/terminal"
                className="flex items-center justify-center h-12 text-gray-300 hover:text-white hover:bg-gray-800 transition-colors rounded mx-2"
                title="Terminal"
              >
                <Terminal size={20} />
              </Link>
            </div>
          </div>
          
          {/* Settings Menu at Bottom */}
          <div className="relative p-2" ref={dropdownRef}>
            <button
              onClick={() => setDropdownOpen(!dropdownOpen)}
              className="flex items-center justify-center h-12 w-12 text-gray-300 hover:text-white hover:bg-gray-800 transition-colors rounded"
              title="Settings"
            >
              <Settings size={20} />
            </button>
            
            {dropdownOpen && (
              <div className="absolute left-16 bottom-0 w-48 bg-gray-800 border border-gray-700 rounded-md shadow-lg z-50">
                <div className="py-1">
                  <button
                    onClick={handleReset}
                    className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-gray-700 hover:text-white transition-colors flex items-center gap-2"
                    disabled={!isConnected}
                  >
                    <RotateCcw size={16} />
                    Reset Terminal
                  </button>
                </div>
              </div>
            )}
          </div>
        </nav>
        
        {/* Main Content */}
        <main className="flex-1">
          <Outlet />
        </main>
      </div>
      <TanStackRouterDevtools position="bottom-right" />
    </>
  )
}

function RootComponent() {
  return (
    <WebSocketProvider>
      <RootLayout />
    </WebSocketProvider>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})