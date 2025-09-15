import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  AlertCircle,
  Rocket,
  Settings,
  Cat,
  Wrench,
  Clock,
  CheckCircle,
  Search,
  ArrowLeft,
} from "lucide-react";

interface CodespaceInfo {
  name: string;
  lastUsed: number;
  repository?: string;
}

interface StatusEvent {
  step:
    | "search"
    | "starting"
    | "setup"
    | "catnip"
    | "initializing"
    | "health"
    | "ready";
  message: string;
}

interface CodespaceAccessProps {
  isAuthenticated: boolean;
  onLogin: () => void;
}

const stepIcons = {
  search: Search,
  starting: Rocket,
  setup: Settings,
  catnip: Cat,
  initializing: Wrench,
  health: Clock,
  ready: CheckCircle,
};

export function CodespaceAccess({
  isAuthenticated,
  onLogin,
}: CodespaceAccessProps) {
  const [isConnecting, setIsConnecting] = useState(false);
  const [statusMessage, setStatusMessage] = useState("");
  const [statusStep, setStatusStep] = useState<StatusEvent["step"] | null>(
    null,
  );
  const [error, setError] = useState("");
  const [orgName, setOrgName] = useState("");
  const [codespaces, setCodespaces] = useState<CodespaceInfo[]>([]);
  const [showSelection, setShowSelection] = useState(false);
  const [showSetup, setShowSetup] = useState(false);
  const [selectedOrg, setSelectedOrg] = useState<string | null>(null);

  const resetState = () => {
    setIsConnecting(false);
    setStatusMessage("");
    setStatusStep(null);
    setError("");
    setCodespaces([]);
    setShowSelection(false);
    setShowSetup(false);
  };

  const accessCodespace = async (org?: string, codespaceName?: string) => {
    if (!codespaceName) {
      resetState();
    } else {
      setError("");
      setCodespaces([]);
      setShowSetup(false);

      // Update browser URL with codespace parameters
      const url = new URL(window.location.href);
      url.searchParams.set("cs", codespaceName);
      if (org) {
        url.searchParams.set("org", org);
      }
      window.history.pushState({}, "", url.toString());
    }
    setIsConnecting(true);
    setStatusMessage("🔄 Finding your codespace...");
    setStatusStep("search");

    const baseUrl = org ? `https://${org}.catnip.run` : "";
    const url = codespaceName
      ? `${baseUrl}/v1/codespace?codespace=${encodeURIComponent(codespaceName)}`
      : `${baseUrl}/v1/codespace`;

    const eventSource = new EventSource(url);

    eventSource.addEventListener("status", (event) => {
      const data = JSON.parse((event as MessageEvent).data) as StatusEvent;
      setStatusMessage(data.message);
      setStatusStep(data.step);
    });

    eventSource.addEventListener("success", (event) => {
      const data = JSON.parse((event as MessageEvent).data);
      setStatusMessage("✅ " + data.message);
      setStatusStep("ready");
      eventSource.close();

      setTimeout(() => {
        resetState();
        window.location.href = data.codespaceUrl;
      }, 1000);
    });

    eventSource.addEventListener("error", (event: MessageEvent) => {
      const data = JSON.parse(event.data);

      let codespaceUrl = "#";
      const targetCodespace = codespaceName || data.codespaceName;
      if (targetCodespace) {
        codespaceUrl = `https://${targetCodespace}.github.dev`;
      }

      const errorMessage =
        codespaceUrl !== "#"
          ? `❌ Ensure you've added 6369 to the forwardPorts directory in your devcontainer.json. You can also try <a href="${codespaceUrl}" target="_blank" rel="noopener noreferrer" class="text-blue-400 hover:text-blue-300 underline">accessing the codespace directly</a>.`
          : `❌ Ensure you've added 6369 to the forwardPorts directory in your devcontainer.json. Please try again.`;
      setError(errorMessage);
      setIsConnecting(false);
      eventSource.close();
    });

    eventSource.addEventListener("setup", (event: MessageEvent) => {
      const data = JSON.parse(event.data);
      setShowSetup(true);
      setSelectedOrg(data.org);
      setError(data.message);
      setIsConnecting(false);
      eventSource.close();
    });

    eventSource.addEventListener("multiple", (event: MessageEvent) => {
      const data = JSON.parse(event.data);
      const urlParams = new URLSearchParams(window.location.search);
      const codespaceName = urlParams.get("cs");

      if (!codespaceName) {
        setCodespaces(data.codespaces);
        setShowSelection(true);
        setSelectedOrg(data.org);
        setIsConnecting(false);
        eventSource.close();
      } else {
        // If we requested a specific codespace but got multiple, that's an error
        setError(
          `❌ Requested codespace "${codespaceName}" not found in your available codespaces.`,
        );
        setIsConnecting(false);
        eventSource.close();
      }
    });

    eventSource.onerror = () => {
      const urlParams = new URLSearchParams(window.location.search);
      const targetCodespace = codespaceName || urlParams.get("cs");

      let errorMessage =
        "❌ Connection failed. Ensure you've added 6369 to the forwardPorts directory in your devcontainer.json.";
      if (targetCodespace) {
        const codespaceUrl = `https://${targetCodespace}.github.dev`;
        errorMessage += ` You can also try <a href="${codespaceUrl}" target="_blank" rel="noopener noreferrer" class="text-blue-400 hover:text-blue-300 underline">accessing the codespace directly</a>.`;
      } else {
        errorMessage += " Please try again.";
      }

      setError(errorMessage);
      setIsConnecting(false);
      setStatusMessage("");
      setStatusStep(null);
      eventSource.close();
    };
  };

  const goToOrg = () => {
    if (orgName.trim()) {
      window.location.href = `https://${orgName.trim()}.catnip.run`;
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      goToOrg();
    }
  };

  useEffect(() => {
    if (isAuthenticated) {
      const urlParams = new URLSearchParams(window.location.search);
      const codespaceName = urlParams.get("cs");
      const orgParam = urlParams.get("org");

      if (codespaceName) {
        void accessCodespace(orgParam || undefined, codespaceName);
      } else if (orgParam) {
        setOrgName(orgParam);
      }
    }
  }, [isAuthenticated]);

  if (!isAuthenticated) {
    return (
      <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center p-4">
        <Card className="w-full max-w-md bg-[#1a1a1a] border-gray-800">
          <CardHeader className="text-center">
            <div className="w-16 h-16 mx-auto mb-4">
              <img src="/logo.png" alt="Catnip Logo" className="w-16 h-16" />
            </div>
            <CardTitle className="text-2xl text-white">Catnip</CardTitle>
            <CardDescription className="text-gray-400">
              Access your GitHub Codespaces
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="p-4 bg-blue-950/30 border border-blue-800/30 rounded-lg text-blue-200 text-sm">
              Logging into GitHub allows us to start codespaces you have added
              catnip to
            </div>
            <Button
              onClick={onLogin}
              className="w-full bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white"
            >
              Login with GitHub
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (showSetup) {
    return (
      <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center p-4">
        <Card className="w-full max-w-2xl bg-[#1a1a1a] border-gray-800">
          <CardHeader>
            <Button
              variant="ghost"
              size="sm"
              onClick={resetState}
              className="w-fit mb-2 text-gray-400 hover:text-white"
            >
              <ArrowLeft className="w-4 h-4 mr-2" />
              Back
            </Button>
            <CardTitle className="text-white flex items-center gap-2">
              <AlertCircle className="w-5 h-5" />
              Setup Required
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-gray-300">
              No Catnip codespaces found
              {selectedOrg ? ` in the "${selectedOrg}" organization` : ""}. To
              use Catnip, you need to:
            </p>
            <ol className="list-decimal list-inside space-y-2 text-gray-300 ml-4">
              <li>
                Add this to your{" "}
                <code className="bg-gray-800 px-2 py-1 rounded text-sm">
                  .devcontainer/devcontainer.json
                </code>
                :
                <pre className="bg-gray-900 p-3 rounded-lg mt-2 text-sm overflow-x-auto">
                  {`"features": {
  "ghcr.io/wandb/catnip/feature:1": {}
}`}
                </pre>
              </li>
              <li>Create a new codespace from your repository</li>
              <li>Return here to access your codespace</li>
            </ol>
            <Button
              asChild
              className="w-full bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white"
            >
              <a
                href="https://github.com/codespaces"
                target="_blank"
                rel="noopener noreferrer"
              >
                Open GitHub Codespaces →
              </a>
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (showSelection) {
    return (
      <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center p-4">
        <Card className="w-full max-w-2xl bg-[#1a1a1a] border-gray-800">
          <CardHeader>
            <Button
              variant="ghost"
              size="sm"
              onClick={resetState}
              className="w-fit mb-2 text-gray-400 hover:text-white"
            >
              <ArrowLeft className="w-4 h-4 mr-2" />
              Back
            </Button>
            <CardTitle className="text-white flex items-center gap-2">
              <Search className="w-5 h-5" />
              Select Codespace
            </CardTitle>
            <CardDescription className="text-gray-400">
              Multiple codespaces found
              {selectedOrg ? ` in the "${selectedOrg}" organization` : ""}.
              Please select one to connect:
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {codespaces.map((cs, index) => {
              const url = new URL(window.location.href);
              url.searchParams.set("cs", cs.name);
              if (selectedOrg) {
                url.searchParams.set("org", selectedOrg);
              }

              return (
                <a key={index} href={url.toString()} className="block">
                  <Card className="bg-gray-900 border-gray-700 cursor-pointer transition-colors hover:bg-gray-800">
                    <CardContent className="p-3">
                      <div className="font-semibold text-white mb-1">
                        {cs.name.replace(/-/g, " ")}
                      </div>
                      {cs.repository && (
                        <div className="text-sm text-blue-400 mb-1">
                          {cs.repository}
                        </div>
                      )}
                      <div className="text-sm text-gray-400">
                        Last used: {new Date(cs.lastUsed).toLocaleString()}
                      </div>
                    </CardContent>
                  </Card>
                </a>
              );
            })}
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center p-4">
      <Card className="w-full max-w-md bg-[#1a1a1a] border-gray-800">
        <CardHeader className="text-center">
          <div className="w-16 h-16 mx-auto mb-4">
            <img src="/logo.png" alt="Catnip Logo" className="w-16 h-16" />
          </div>
          <CardTitle className="text-2xl text-white">Catnip</CardTitle>
          <CardDescription className="text-gray-400">
            {orgName.trim()
              ? `Access GitHub Codespaces in ${orgName.trim()}`
              : "Access your GitHub Codespaces"}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Button
            onClick={() => accessCodespace()}
            disabled={isConnecting}
            className="w-full bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 disabled:opacity-50 text-white flex items-center justify-center"
          >
            {isConnecting && (
              <div className="w-4 h-4 border-2 border-white/20 border-t-white rounded-full animate-spin mr-2 flex-shrink-0" />
            )}
            {isConnecting ? "Connecting..." : "Access My Codespace"}
          </Button>

          {statusMessage && (
            <Card className="bg-blue-950/50 border-blue-800/50">
              <CardContent className="px-3 py-2 flex items-center gap-2">
                {statusStep && (
                  <div className="flex-shrink-0">
                    {(() => {
                      const IconComponent = stepIcons[statusStep];
                      return (
                        <IconComponent className="w-4 h-4 text-blue-400" />
                      );
                    })()}
                  </div>
                )}
                <span className="text-blue-100 font-medium text-sm">
                  {statusMessage}
                </span>
              </CardContent>
            </Card>
          )}

          {error && (
            <Card className="bg-red-950/50 border-red-800/50">
              <CardContent className="p-3">
                <span
                  className="text-red-100 font-medium text-sm"
                  dangerouslySetInnerHTML={{ __html: error }}
                />
              </CardContent>
            </Card>
          )}

          <div className="bg-amber-950/30 border border-amber-800/30 rounded-lg p-4 text-sm text-amber-200">
            <strong>Note:</strong> If you see the VSCode interface, click the
            back button to re-initiate access.
          </div>

          <div className="border-t border-gray-800 pt-4">
            <p className="text-gray-400 text-sm mb-3">
              Or access codespaces in a specific organization:
            </p>
            <div className="flex gap-2">
              <Input
                type="text"
                placeholder="Organization name (e.g., wandb)"
                value={orgName}
                onChange={(e) => setOrgName(e.target.value)}
                onKeyPress={handleKeyPress}
                disabled={isConnecting}
                className="bg-gray-900 border-gray-700 text-white placeholder-gray-500 focus:border-purple-500"
              />
              <Button
                onClick={goToOrg}
                disabled={isConnecting || !orgName.trim()}
                variant="secondary"
                className="bg-gray-700 hover:bg-gray-600"
              >
                Go
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
