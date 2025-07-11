import { Link, useRouter } from "@tanstack/react-router";
import { useState, useEffect, useRef } from "react";
import { Home, Terminal, Settings, RotateCcw, GitBranch, Menu, X, Github, FileText, ExternalLink, Globe } from "lucide-react";
import { GitHubAuthModal } from "@/components/GitHubAuthModal";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useWebSocket } from "@/lib/websocket-context";

interface ServiceInfo {
  port: number;
  service_type: string;
  health: string;
  title?: string;
}

export function Navbar() {
  const { isConnected } = useWebSocket();
  const router = useRouter();
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [authModalOpen, setAuthModalOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [ports, setPorts] = useState<Record<number, ServiceInfo>>({});
  const dropdownRef = useRef<HTMLDivElement>(null);
  
  // Get current route params
  const currentPath = router.state.location.pathname;
  const isPreviewRoute = currentPath.startsWith('/preview/');
  const currentPort = isPreviewRoute ? parseInt(currentPath.split('/')[2]) : null;

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

  // Fetch ports data
  useEffect(() => {
    const fetchPorts = async () => {
      try {
        const response = await fetch('/v1/ports');
        if (response.ok) {
          const data = await response.json();
          setPorts(data.ports || {});
        }
      } catch (error) {
        console.error('Failed to fetch ports:', error);
      }
    };

    fetchPorts();
    const interval = setInterval(fetchPorts, 2000);
    return () => clearInterval(interval);
  }, []);

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
      {/* Mobile Menu Toggle */}
      <div className="sm:hidden fixed top-4 right-4 z-50">
        <button
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          className="p-2 text-muted-foreground hover:text-primary-foreground bg-[#1a1a1a] rounded"
        >
          {mobileMenuOpen ? <X size={24} /> : <Menu size={24} />}
        </button>
      </div>

      {/* Mobile Navigation Menu */}
      {mobileMenuOpen && (
        <div className="sm:hidden fixed inset-0 z-40">
          <div className="fixed inset-0 bg-black/50" onClick={() => setMobileMenuOpen(false)} />
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
              <Link
                to="/docs"
                className="flex items-center gap-3 px-3 py-2 text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors rounded"
                onClick={() => setMobileMenuOpen(false)}
              >
                <FileText size={20} />
                <span>Docs</span>
              </Link>
              
              {/* Ports Section in Mobile Menu */}
              {Object.keys(ports).length > 0 && (
                <>
                  <div className="border-t border-gray-700 my-2" />
                  <div className="text-xs text-muted-foreground px-3 py-1">Active Ports</div>
                  {Object.values(ports)
                    .filter(p => p.service_type === 'http' && p.health === 'healthy')
                    .map((service) => (
                      <Link
                        key={service.port}
                        to="/preview/$port"
                        params={{ port: service.port.toString() }}
                        className="flex items-center gap-3 px-3 py-2 text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors rounded"
                        onClick={() => setMobileMenuOpen(false)}
                      >
                        <Globe size={20} />
                        <div className="flex flex-col">
                          <span>Port {service.port}</span>
                          {service.title && (
                            <span className="text-xs text-muted-foreground">{service.title}</span>
                          )}
                        </div>
                      </Link>
                    ))}
                </>
              )}
              
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
              >
                <RotateCcw size={20} />
                <span>Reset Terminal</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Desktop Sidebar Navigation */}
      <nav className="hidden sm:block w-16 bg-[#1a1a1a] flex-shrink-0 border-r border-gray-800">
        <div className="flex flex-col h-full">
          {/* Logo */}
          <div className="p-3 flex justify-center relative">
            <Link
              to="/"
              className="hover:scale-110 transition-transform"
              title="Catnip"
            >
              <img src="/logo@2x.png" alt="Catnip Logo" className="w-10 h-10" />
            </Link>

            {/* Connection Status - positioned at logo's top right */}
            <div
              className={`absolute top-1 right-1 h-2 w-2 rounded-full ${
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
              <Link
                to="/docs"
                className="flex items-center justify-center h-12 w-12 text-muted-foreground hover:text-primary-foreground transition-colors rounded mx-2"
                title="Documentation"
              >
                <FileText size={20} />
              </Link>
              
              {/* Ports Dropdown */}
              {Object.keys(ports).length > 0 && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button
                      className="flex items-center justify-center h-12 w-12 text-muted-foreground hover:text-primary-foreground transition-colors rounded mx-2"
                      title="Ports"
                    >
                      <Globe size={20} />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" side="right" className="w-56">
                    {Object.values(ports)
                      .filter(p => p.service_type === 'http' && p.health === 'healthy')
                      .map((service) => (
                        <DropdownMenuItem 
                          key={service.port} 
                          asChild
                          className={currentPort === service.port ? 'bg-accent' : ''}
                        >
                          <Link to="/preview/$port" params={{ port: service.port.toString() }}>
                            <div className="flex flex-col w-full">
                              <div className="flex items-center gap-2">
                                <span className="font-medium">Port {service.port}</span>
                                {currentPort === service.port && (
                                  <span className="text-xs text-muted-foreground">(current)</span>
                                )}
                              </div>
                              {service.title && (
                                <span className="text-xs text-muted-foreground">{service.title}</span>
                              )}
                            </div>
                          </Link>
                        </DropdownMenuItem>
                      ))}
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
              
              {/* Open in New Tab (only on preview routes) */}
              {isPreviewRoute && currentPort && (
                <button
                  className="flex items-center justify-center h-12 w-12 text-muted-foreground hover:text-primary-foreground transition-colors rounded mx-2"
                  title="Open in New Tab"
                  onClick={() => window.open(`/${currentPort}/`, '_blank')}
                >
                  <ExternalLink size={20} />
                </button>
              )}
            </div>
          </div>

          {/* Settings Menu at Bottom */}
          <div className="relative px-2 pb-2" ref={dropdownRef}>
            <button
              onClick={() => setDropdownOpen(!dropdownOpen)}
              className="w-12 h-12 flex items-center justify-center text-muted-foreground hover:text-primary-foreground transition-colors rounded"
              title="Settings"
            >
              <Settings size={20} />
            </button>

            {/* Dropdown Menu */}
            {dropdownOpen && (
              <div className="absolute bottom-14 left-0 right-0 mx-2 bg-[#0a0a0a] border border-gray-800 rounded-lg shadow-lg overflow-hidden">
                <button
                  onClick={handleGitHubLogin}
                  className="w-full px-4 py-2 text-left text-sm text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors flex items-center gap-2"
                >
                  <Github size={16} />
                  GitHub Login
                </button>
                <button
                  onClick={handleReset}
                  className="w-full px-4 py-2 text-left text-sm text-muted-foreground hover:text-primary-foreground hover:bg-gray-800 transition-colors flex items-center gap-2"
                >
                  <RotateCcw size={16} />
                  Reset Terminal
                </button>
              </div>
            )}
          </div>
        </div>
      </nav>

      {/* GitHub Auth Modal */}
      <GitHubAuthModal 
        isOpen={authModalOpen}
        onClose={() => setAuthModalOpen(false)}
      />
    </>
  );
}