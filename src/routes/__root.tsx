import { createRootRoute, Outlet } from "@tanstack/react-router";
import { TanStackRouterDevtools } from "@tanstack/react-router-devtools";
import { WebSocketProvider } from "@/lib/websocket-context";
import { Navbar } from "@/components/Navbar";
import { Toaster } from "@/components/ui/sonner";
import { Link } from "@tanstack/react-router";
import { useWebSocket } from "@/lib/websocket-context";

function RootLayout() {
  const { isConnected } = useWebSocket();

  return (
    <>
      <div className="min-h-screen bg-background flex">
        {/* Mobile Header - Only visible on small screens */}
        <header className="sm:hidden fixed top-0 left-0 right-0 h-14 bg-[#1a1a1a] flex items-center justify-between px-4 border-b border-gray-800 z-50">
          {/* Logo with Connection Status */}
          <div className="flex items-center gap-3">
            <Link to="/" className="flex items-center relative">
              <img src="/logo@2x.png" alt="Catnip Logo" className="w-8 h-8" />
              {/* Connection Status */}
              <div
                className={`absolute -top-1 -right-1 h-2 w-2 rounded-full ${
                  isConnected
                    ? "bg-green-500 shadow-green-500/50 animate-pulse"
                    : "bg-red-500"
                }`}
                title={isConnected ? "Connected" : "Disconnected"}
              />
            </Link>
            <span className="text-sm font-medium text-gray-200">Catnip</span>
          </div>
        </header>

        {/* Navbar Component */}
        <Navbar />

        {/* Main Content - Responsive margins and height */}
        <main className="flex-1 pt-14 sm:pt-0">
          <div className="h-[calc(100vh-4rem)] sm:h-screen">
            <Outlet />
          </div>
        </main>
      </div>
      
      {/* Toast notifications */}
      <Toaster />
      
      <TanStackRouterDevtools position="bottom-right" />
    </>
  );
}

function RootComponent() {
  return (
    <WebSocketProvider>
      <RootLayout />
    </WebSocketProvider>
  );
}

export const Route = createRootRoute({
  component: RootComponent,
});