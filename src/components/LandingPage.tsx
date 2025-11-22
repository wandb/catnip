import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Menu,
  X,
  Github,
  Smartphone,
  Zap,
  Cloud,
  Terminal,
} from "lucide-react";
import { MobileAppBanner } from "@/components/MobileAppBanner";

interface LandingPageProps {
  onLogin: () => void;
}

export function LandingPage({ onLogin }: LandingPageProps) {
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="min-h-screen bg-gradient-to-b from-[#0a0a0a] to-[#1a1a1a]">
      {/* Mobile App Banner */}
      <MobileAppBanner />

      {/* Header with Hamburger Menu */}
      <header className="fixed top-0 left-0 right-0 z-50 bg-[#0a0a0a]/80 backdrop-blur-sm border-b border-gray-800">
        <div className="container mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <img src="/logo@2x.png" alt="Catnip Logo" className="w-10 h-10" />
            <span className="text-xl font-bold text-white">Catnip</span>
          </div>

          {/* Hamburger Menu Button */}
          <button
            onClick={() => setMenuOpen(!menuOpen)}
            className="p-2 text-gray-400 hover:text-white transition-colors"
            aria-label="Menu"
          >
            {menuOpen ? (
              <X className="w-6 h-6" />
            ) : (
              <Menu className="w-6 h-6" />
            )}
          </button>
        </div>

        {/* Dropdown Menu */}
        {menuOpen && (
          <div className="absolute top-full right-0 w-48 bg-[#1a1a1a] border-l border-b border-gray-800 shadow-lg">
            <button
              onClick={() => {
                setMenuOpen(false);
                onLogin();
              }}
              className="w-full px-4 py-3 text-left text-white hover:bg-gray-800 transition-colors flex items-center gap-2"
            >
              <Github className="w-4 h-4" />
              Log In
            </button>
          </div>
        )}
      </header>

      {/* Main Content */}
      <main className="pt-24 pb-16">
        <div className="container mx-auto px-4">
          {/* Hero Section */}
          <div className="text-center max-w-4xl mx-auto mb-16">
            <div className="w-24 h-24 mx-auto mb-6">
              <img src="/logo@2x.png" alt="Catnip Logo" className="w-24 h-24" />
            </div>
            <h1 className="text-5xl md:text-6xl font-bold text-white mb-6">
              Your Development Environment,
              <br />
              <span className="bg-gradient-to-r from-purple-400 to-blue-400 bg-clip-text text-transparent">
                Anywhere
              </span>
            </h1>
            <p className="text-xl text-gray-400 mb-8 max-w-2xl mx-auto">
              Catnip brings your GitHub Codespaces to life with a powerful,
              agent-friendly development environment. Code on the go with our
              native mobile app or access from any browser.
            </p>
            <Button
              onClick={onLogin}
              size="lg"
              className="bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white text-lg px-8 py-6"
            >
              <Github className="w-5 h-5 mr-2" />
              Get Started with GitHub
            </Button>
          </div>

          {/* Features Grid */}
          <div className="grid md:grid-cols-3 gap-8 max-w-6xl mx-auto mb-16">
            <Card className="bg-[#1a1a1a] border-gray-800">
              <CardContent className="p-6">
                <div className="w-12 h-12 bg-purple-600/20 rounded-lg flex items-center justify-center mb-4">
                  <Smartphone className="w-6 h-6 text-purple-400" />
                </div>
                <h3 className="text-xl font-semibold text-white mb-2">
                  Mobile First
                </h3>
                <p className="text-gray-400">
                  Native iOS app brings the full power of your codespace to your
                  iPhone or iPad. Code anywhere, anytime.
                </p>
              </CardContent>
            </Card>

            <Card className="bg-[#1a1a1a] border-gray-800">
              <CardContent className="p-6">
                <div className="w-12 h-12 bg-blue-600/20 rounded-lg flex items-center justify-center mb-4">
                  <Cloud className="w-6 h-6 text-blue-400" />
                </div>
                <h3 className="text-xl font-semibold text-white mb-2">
                  GitHub Codespaces
                </h3>
                <p className="text-gray-400">
                  Seamlessly connects to your existing GitHub Codespaces. No
                  migration, no hassle.
                </p>
              </CardContent>
            </Card>

            <Card className="bg-[#1a1a1a] border-gray-800">
              <CardContent className="p-6">
                <div className="w-12 h-12 bg-green-600/20 rounded-lg flex items-center justify-center mb-4">
                  <Zap className="w-6 h-6 text-green-400" />
                </div>
                <h3 className="text-xl font-semibold text-white mb-2">
                  Lightning Fast
                </h3>
                <p className="text-gray-400">
                  Optimized for performance with instant reconnection and
                  efficient resource usage.
                </p>
              </CardContent>
            </Card>
          </div>

          {/* How It Works */}
          <div className="max-w-4xl mx-auto text-center mb-16">
            <h2 className="text-3xl font-bold text-white mb-8">How It Works</h2>
            <div className="space-y-6 text-left">
              <div className="flex gap-4">
                <div className="flex-shrink-0 w-8 h-8 bg-purple-600 rounded-full flex items-center justify-center text-white font-bold">
                  1
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-white mb-1">
                    Add Catnip to Your Devcontainer
                  </h3>
                  <p className="text-gray-400">
                    Add the Catnip feature to your{" "}
                    <code className="bg-gray-800 px-2 py-1 rounded text-sm">
                      .devcontainer/devcontainer.json
                    </code>{" "}
                    file.
                  </p>
                  <pre className="bg-gray-900 p-3 rounded-lg mt-2 text-sm overflow-x-auto text-gray-300">
                    {`"features": {
  "ghcr.io/wandb/catnip/feature:1": {}
}`}
                  </pre>
                </div>
              </div>

              <div className="flex gap-4">
                <div className="flex-shrink-0 w-8 h-8 bg-blue-600 rounded-full flex items-center justify-center text-white font-bold">
                  2
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-white mb-1">
                    Create or Restart Your Codespace
                  </h3>
                  <p className="text-gray-400">
                    Launch a new GitHub Codespace or rebuild your existing one.
                    Catnip will be automatically configured.
                  </p>
                </div>
              </div>

              <div className="flex gap-4">
                <div className="flex-shrink-0 w-8 h-8 bg-green-600 rounded-full flex items-center justify-center text-white font-bold">
                  3
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-white mb-1">
                    Access from Anywhere
                  </h3>
                  <p className="text-gray-400">
                    Log in with GitHub and connect to your codespace from your
                    browser or mobile app.
                  </p>
                </div>
              </div>
            </div>
          </div>

          {/* CTA Section */}
          <div className="max-w-2xl mx-auto text-center">
            <Card className="bg-gradient-to-r from-purple-900/30 to-blue-900/30 border-purple-800/50">
              <CardContent className="p-8">
                <Terminal className="w-12 h-12 mx-auto mb-4 text-purple-400" />
                <h2 className="text-2xl font-bold text-white mb-4">
                  Ready to Get Started?
                </h2>
                <p className="text-gray-300 mb-6">
                  Join developers who are coding from anywhere with Catnip.
                </p>
                <Button
                  onClick={onLogin}
                  size="lg"
                  className="bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white"
                >
                  <Github className="w-5 h-5 mr-2" />
                  Log In with GitHub
                </Button>
              </CardContent>
            </Card>
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t border-gray-800 py-8">
        <div className="container mx-auto px-4 text-center text-gray-500 text-sm">
          <p>
            Built with{" "}
            <span className="text-purple-400">React + TypeScript</span> •
            Powered by <span className="text-blue-400">Cloudflare Workers</span>
          </p>
        </div>
      </footer>
    </div>
  );
}
