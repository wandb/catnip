import { createRootRoute, Link, Outlet } from "@tanstack/react-router";
import { TanStackRouterDevtools } from "@tanstack/react-router-devtools";
import { WebSocketProvider, useWebSocket } from "@/lib/websocket-context";
import { useState, useEffect, useRef } from "react";
import { Home, Terminal, Settings, RotateCcw, GitBranch, Menu, X, Github } from "lucide-react";
import { GitHubAuthModal } from "@/components/GitHubAuthModal";

function RootLayout() {
  const { isConnected } = useWebSocket();
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [authModalOpen, setAuthModalOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const handleReset = () => {
    // Call the reset function if it exists (on terminal page)
    if ((window as any).resetTerminal) {
      (window as any).resetTerminal();
    }
    setDropdownOpen(false);
  };

  const handleGitHubLogin = () => {
    setAuthModalOpen(true);
    setDropdownOpen(false);
  };

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(event.target as Node)
      ) {
        setDropdownOpen(false);
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
    };
  }, []);

  return (
    <>
      <div className="min-h-screen bg-background">
        {/* Mobile Header - Only visible on small screens */}
        <header className="md:hidden fixed top-0 left-0 right-0 h-14 bg-[#1a1a1a] flex items-center justify-between px-4 border-b border-gray-800 z-50">
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
          
          {/* Mobile menu button */}
          <button
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            className="p-2 text-muted-foreground hover:text-primary-foreground transition-colors"
          >
            {mobileMenuOpen ? <X size={20} /> : <Menu size={20} />}
          </button>
        </header>

        {/* Mobile Menu Overlay */}
        {mobileMenuOpen && (
          <div className="md:hidden fixed inset-0 z-40 bg-black/50" onClick={() => setMobileMenuOpen(false)}>
            <div className="fixed top-14 right-0 w-64 h-full bg-[#1a1a1a] border-l border-gray-800">
              <div className="flex flex-col p-4 space-y-2">
                <Link
                  to="/"
                  className="flex items-center gap-3 px-3 py-2 text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors rounded"
                  onClick={() => setMobileMenuOpen(false)}
                >
                  <Home size={20} />
                  <span>Home</span>
                </Link>
                <Link
                  to="/terminal"
                  className="flex items-center gap-3 px-3 py-2 text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors rounded"
                  onClick={() => setMobileMenuOpen(false)}
                >
                  <Terminal size={20} />
                  <span>Terminal</span>
                </Link>
                <Link
                  to="/git"
                  className="flex items-center gap-3 px-3 py-2 text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors rounded"
                  onClick={() => setMobileMenuOpen(false)}
                >
                  <GitBranch size={20} />
                  <span>Git</span>
                </Link>
                
                <div className="border-t border-gray-700 my-2" />
                
                <button
                  onClick={() => { handleGitHubLogin(); setMobileMenuOpen(false); }}
                  className="flex items-center gap-3 px-3 py-2 text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors rounded text-left w-full"
                >
                  <Github size={20} />
                  <span>GitHub Login</span>
                </button>
                
                <button
                  onClick={() => { handleReset(); setMobileMenuOpen(false); }}
                  className="flex items-center gap-3 px-3 py-2 text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors rounded text-left w-full"
                  disabled={!isConnected}
                >
                  <RotateCcw size={20} />
                  <span>Reset Terminal</span>
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Desktop Vertical Sidebar - Hidden on mobile */}
        <nav className="hidden md:flex fixed top-0 left-0 w-16 h-full bg-[#1a1a1a] flex-col z-50">
          {/* Cat Logo with Connection Status */}
          <div className="flex items-center justify-center h-16 border-b border-gray-800 relative">
            <Link to="/" className="text-2xl">
              <img src="/logo@2x.png" alt="Catnip Logo" className="w-10 h-10" />
            </Link>
            {/* Connection Status - positioned at cat's right ear */}
            <div
              className={`absolute top-2 right-2 h-2 w-2 rounded-full ${
                isConnected
                  ? "bg-green-500 shadow-green-500/50 animate-pulse"
                  : "bg-red-500"
              }`}
              title={isConnected ? "Connected" : "Disconnected"}
            />
          </div>

          {/* Navigation Items */}
          <div className="flex-1 py-4">
            <div className="flex flex-col space-y-2">
              <Link
                to="/"
                className="flex items-center justify-center h-12 w-12 text-muted-foreground hover:text-primary-foreground transition-colors rounded mx-2"
                title="Home"
              >
                <Home size={20} />
              </Link>
              <Link
                to="/terminal"
                className="flex items-center justify-center h-12 w-12 text-muted-foreground hover:text-primary-foreground transition-colors rounded mx-2"
                title="Terminal"
              >
                <Terminal size={20} />
              </Link>
              <Link
                to="/git"
                className="flex items-center justify-center h-12 w-12 text-muted-foreground hover:text-primary-foreground transition-colors rounded mx-2"
                title="Git"
              >
                <GitBranch size={20} />
              </Link>
            </div>
          </div>

          {/* Settings Menu at Bottom */}
          <div className="relative p-2" ref={dropdownRef}>
            <button
              onClick={() => setDropdownOpen(!dropdownOpen)}
              className="flex items-center justify-center h-12 w-12 text-muted-foreground hover:text-primary-foreground transition-colors rounded"
              title="Settings"
            >
              <Settings size={20} />
            </button>

            {dropdownOpen && (
              <div className="absolute left-16 bottom-0 w-48 bg-gray-800 border border-gray-700 rounded-md shadow-lg z-50">
                <div className="py-1">
                  <button
                    onClick={handleGitHubLogin}
                    className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-gray-700 hover:text-white transition-colors flex items-center gap-2"
                  >
                    <Github size={16} />
                    GitHub Login
                  </button>
                  <div className="border-t border-gray-700 my-1" />
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

        {/* Main Content - Responsive margins and height */}
        <main className="h-screen pt-14 md:pt-0 md:ml-16">
          <div className="h-full">
            <Outlet />
          </div>
        </main>
      </div>
      
      {/* GitHub Auth Modal */}
      <GitHubAuthModal open={authModalOpen} onOpenChange={setAuthModalOpen} />
      
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
