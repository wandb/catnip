import { createRootRoute, Outlet, useLocation } from "@tanstack/react-router";
import { TanStackRouterDevtools } from "@tanstack/react-router-devtools";
import { WebSocketProvider } from "@/lib/websocket-context";
import { Navbar } from "@/components/Navbar";
import { Toaster } from "@/components/ui/sonner";
import { Link } from "@tanstack/react-router";
import { useWebSocket } from "@/lib/hooks";
import { Suspense } from "react";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { AuthProvider } from "@/lib/auth-context";
import { useAuth } from "@/lib/hooks";
import { LoginModal } from "@/components/LoginModal";
import { GitHubAuthProvider } from "@/lib/github-auth-context";
import { useGitHubAuth } from "@/lib/hooks";
import { GitHubAuthModal } from "@/components/GitHubAuthModal";
import { LoadingSpinner } from "@/components/LoadingSpinner";

function RootLayout() {
  const location = useLocation();
  const { isConnected } = useWebSocket();
  const { isAuthenticated, isLoading, authRequired } = useAuth();
  const { showAuthModal, setShowAuthModal } = useGitHubAuth();

  // Check if we're on a workspace route
  const isWorkspaceRoute = location.pathname.startsWith("/workspace");

  // Show login modal if auth is required and user is not authenticated
  const showLoginModal = !isLoading && authRequired && !isAuthenticated;

  return (
    <>
      <div className="min-h-screen bg-background">
        {!isWorkspaceRoute && (
          <>
            {/* Mobile Header - Only visible on small screens */}
            <header className="sm:hidden fixed top-0 left-0 right-0 h-14 bg-[#1a1a1a] flex items-center justify-between px-4 border-b border-gray-800 z-50">
              {/* Logo with Connection Status */}
              <div className="flex items-center gap-3">
                <Link to="/" className="flex items-center relative">
                  <img
                    src="/logo@2x.png"
                    alt="Catnip Logo"
                    className="w-8 h-8"
                  />
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
                <span className="text-sm font-medium text-foreground">
                  Catnip
                </span>
              </div>
            </header>

            {/* Navbar Component */}
            <Navbar />
          </>
        )}

        {/* Main Content - Conditional margins based on route */}
        <main
          className={isWorkspaceRoute ? "h-screen" : "pt-14 sm:pt-0 sm:pl-16"}
        >
          <div
            className={
              isWorkspaceRoute ? "h-full" : "h-[calc(100vh-4rem)] sm:h-screen"
            }
          >
            <ErrorBoundary>
              <Suspense
                fallback={
                  <div className="flex items-center justify-center h-full">
                    <LoadingSpinner size="lg" />
                  </div>
                }
              >
                <Outlet />
              </Suspense>
            </ErrorBoundary>
          </div>
        </main>
      </div>

      {/* Toast notifications */}
      <Toaster />

      {/* Login Modal */}
      <LoginModal open={showLoginModal} />

      {/* GitHub Auth Modal */}
      <GitHubAuthModal open={showAuthModal} onOpenChange={setShowAuthModal} />

      <TanStackRouterDevtools position="bottom-right" />
    </>
  );
}

function RootComponent() {
  return (
    <AuthProvider>
      <WebSocketProvider>
        <GitHubAuthProvider>
          <RootLayout />
        </GitHubAuthProvider>
      </WebSocketProvider>
    </AuthProvider>
  );
}

export const Route = createRootRoute({
  component: RootComponent,
});
