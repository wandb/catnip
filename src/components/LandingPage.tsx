import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Github,
  Smartphone,
  Zap,
  Cloud,
  Terminal,
  Copy,
  Check,
} from "lucide-react";

const CODE_SNIPPET = `"features": {
  "ghcr.io/wandb/catnip/feature:1": {}
},
"forwardPorts": [6369],`;

interface LandingPageProps {
  onLogin: () => void;
}

export function LandingPage({ onLogin }: LandingPageProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(CODE_SNIPPET);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="min-h-screen bg-gradient-to-b from-[#0a0a0a] to-[#1a1a1a]">
      {/* Main Content */}
      <main className="pt-12 pb-16">
        <div className="container mx-auto px-4">
          {/* Hero Section */}
          <div className="text-center max-w-4xl mx-auto mb-16">
            <div className="w-24 h-24 mx-auto mb-6">
              <img src="/logo@2x.png" alt="Catnip Logo" className="w-24 h-24" />
            </div>
            <h1 className="text-5xl md:text-6xl font-bold text-white mb-6">
              Claude Code
              <br />
              <span className="bg-gradient-to-r from-purple-400 to-blue-400 bg-clip-text text-transparent">
                Everywhere
              </span>
            </h1>
            <p className="text-xl text-gray-400 mb-8 max-w-2xl mx-auto">
              Catnip let's you access the full Claude Code TUI from anywhere.
              Code on the go with our native mobile app or access from any
              browser. Powered by GitHub Codespaces.
            </p>
            <div className="flex flex-col sm:flex-row items-center justify-center gap-3 sm:gap-4">
              <Button
                asChild
                size="lg"
                className="bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white text-lg px-8 py-6"
              >
                <a
                  href="https://apps.apple.com/us/app/w-b-catnip/id6755161660"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <Smartphone className="w-5 h-5 mr-2" />
                  Install the iOS App
                </a>
              </Button>
              <span className="text-gray-500 text-sm">or</span>
              <Button
                onClick={onLogin}
                size="lg"
                variant="outline"
                className="border-gray-700 bg-transparent hover:bg-gray-800 text-gray-300 hover:text-white text-lg px-8 py-6"
              >
                <Github className="w-5 h-5 mr-2" />
                Get Started with GitHub
              </Button>
            </div>
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
                  <a
                    href="https://apps.apple.com/us/app/w-b-catnip/id6755161660"
                    className="text-purple-400 hover:underline"
                  >
                    Our native iOS app
                  </a>{" "}
                  brings the full power of Claude Code to your iPhone or iPad.
                  Code anywhere, anytime.
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
                  Seamlessly connects to your existing{" "}
                  <a
                    href="https://github.com/features/codespaces"
                    className="text-blue-400 hover:underline"
                  >
                    GitHub Codespaces
                  </a>
                  . No migration, no hassle.
                </p>
              </CardContent>
            </Card>

            <Card className="bg-[#1a1a1a] border-gray-800">
              <CardContent className="p-6">
                <div className="w-12 h-12 bg-green-600/20 rounded-lg flex items-center justify-center mb-4">
                  <Zap className="w-6 h-6 text-green-400" />
                </div>
                <h3 className="text-xl font-semibold text-white mb-2">
                  Open Source
                </h3>
                <p className="text-gray-400">
                  Everything is{" "}
                  <a
                    href="https://github.com/wandb/catnip"
                    className="text-green-400 hover:underline"
                  >
                    open source
                  </a>{" "}
                  and free to use. No hidden fees or subscriptions.
                </p>
              </CardContent>
            </Card>
          </div>

          {/* Video Preview - Device Mockups */}
          <div className="flex items-end justify-center gap-8 md:gap-12 mb-16 px-4">
            {/* iPad Pro Frame - hidden on mobile */}
            <div className="hidden md:block max-w-lg flex-shrink-0">
              <div className="relative bg-[#0d0d0d] rounded-[1.5rem] p-1.5 shadow-2xl ring-1 ring-white/10">
                {/* Camera dot */}
                <div className="absolute top-2 left-1/2 -translate-x-1/2 w-2 h-2 rounded-full bg-[#1a1a1a]" />

                {/* Screen */}
                <div className="relative rounded-[1.25rem] overflow-hidden">
                  <video className="w-full" autoPlay loop muted playsInline>
                    <source src="/catnip-preview-ipad.mp4" type="video/mp4" />
                  </video>
                </div>

                {/* Home indicator */}
                <div className="absolute bottom-1 left-1/2 -translate-x-1/2 w-20 h-1 rounded-full bg-white/20" />
              </div>
            </div>

            {/* iPhone Frame */}
            <div className="w-64 md:w-56 flex-shrink-0">
              <div className="relative bg-[#0d0d0d] rounded-[2rem] p-1 shadow-2xl ring-1 ring-white/10">
                {/* Screen */}
                <div className="relative rounded-[1.75rem] overflow-hidden">
                  <video className="w-full" autoPlay loop muted playsInline>
                    <source src="/catnip-preview-iphone.mp4" type="video/mp4" />
                  </video>
                </div>

                {/* Home indicator */}
                <div className="absolute bottom-1.5 left-1/2 -translate-x-1/2 w-12 h-1 rounded-full bg-white/20" />
              </div>
            </div>
          </div>

          {/* How It Works */}
          <div className="max-w-4xl mx-auto text-center mb-16">
            <h2 className="text-3xl font-bold text-white mb-8">How It Works</h2>
            <p className="text-gray-400 mb-8 italic">
              Don't worry about doing these steps yourself. Our{" "}
              <a
                href="https://apps.apple.com/us/app/w-b-catnip/id6755161660"
                className="text-blue-400 underline"
              >
                mobile app
              </a>{" "}
              can do it for any of your existing repositories!
            </p>
            <div className="space-y-6 text-left">
              <div className="flex gap-4">
                <div className="flex-shrink-0 w-8 h-8 bg-purple-600 rounded-full flex items-center justify-center text-white font-bold">
                  1
                </div>
                <div className="min-w-0 flex-1">
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
                  <div className="relative mt-2 max-w-full">
                    <button
                      onClick={handleCopy}
                      className="absolute top-2 right-2 p-1.5 rounded-md bg-white/10 hover:bg-white/20 transition-colors"
                      aria-label="Copy code"
                    >
                      {copied ? (
                        <Check className="w-4 h-4 text-green-400" />
                      ) : (
                        <Copy className="w-4 h-4 text-gray-400" />
                      )}
                    </button>
                    <pre className="bg-gradient-to-r from-purple-900/40 to-blue-900/40 border border-purple-800/30 p-4 pr-12 rounded-lg text-sm overflow-x-auto font-mono">
                      <code>
                        <span className="text-purple-400">"features"</span>
                        <span className="text-gray-400">: {"{"}</span>
                        {"\n  "}
                        <span className="text-blue-400">
                          "ghcr.io/wandb/catnip/feature:1"
                        </span>
                        <span className="text-gray-400">: {"{}"}</span>
                        {"\n"}
                        <span className="text-gray-400">{"},"}</span>
                        {"\n"}
                        <span className="text-purple-400">"forwardPorts"</span>
                        <span className="text-gray-400">: [</span>
                        <span className="text-green-400">6369</span>
                        <span className="text-gray-400">],</span>
                      </code>
                    </pre>
                  </div>
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

              <p className="text-gray-400 mt-4 pl-12">
                To learn more about how everything works, check out our{" "}
                <a
                  href="https://github.com/wandb/catnip?tab=readme-ov-file#github-codespaces--devcontainers"
                  className="text-blue-400 hover:underline"
                >
                  GitHub Repository
                </a>
                .
              </p>
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
                  Scratch that itch! Start using Claude Code everywhere.
                </p>
                <div className="flex flex-col sm:flex-row items-center justify-center gap-3 sm:gap-4">
                  <Button
                    asChild
                    size="lg"
                    className="bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white"
                  >
                    <a
                      href="https://apps.apple.com/us/app/w-b-catnip/id6755161660"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <Smartphone className="w-5 h-5 mr-2" />
                      Install the iOS App
                    </a>
                  </Button>
                  <span className="text-gray-500 text-sm">or</span>
                  <Button
                    onClick={onLogin}
                    size="lg"
                    variant="outline"
                    className="border-gray-700 bg-transparent hover:bg-gray-800 text-gray-300 hover:text-white"
                  >
                    <Github className="w-5 h-5 mr-2" />
                    Get Started with GitHub
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t border-gray-800 py-8">
        <div className="container mx-auto px-4 text-center text-gray-500 text-sm">
          <p>
            Built by{" "}
            <a
              href="https://wandb.ai"
              className="text-purple-400 hover:underline"
            >
              Weights & Biases
            </a>{" "}
            • Powered by{" "}
            <a
              href="https://github.com/features/codespaces"
              className="text-blue-400 hover:underline"
            >
              GitHub Codespaces
            </a>
          </p>
        </div>
      </footer>
    </div>
  );
}
