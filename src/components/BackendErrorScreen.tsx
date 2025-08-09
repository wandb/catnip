import { AlertCircle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAppStore } from "@/stores/appStore";

export function BackendErrorScreen() {
  const { loadError, loadInitialData } = useAppStore();

  const handleRetry = async () => {
    await loadInitialData();
  };

  return (
    <div className="flex h-screen items-center justify-center bg-background">
      <div className="max-w-md text-center space-y-6 p-8">
        <div className="flex justify-center">
          <div className="rounded-full bg-destructive/10 p-4">
            <AlertCircle className="h-12 w-12 text-destructive" />
          </div>
        </div>

        <div className="space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            Backend Connection Failed
          </h1>
          <p className="text-muted-foreground">
            Unable to connect to the backend server. The server may be starting
            up or experiencing issues.
          </p>
          {loadError && (
            <p className="text-sm text-muted-foreground mt-2">
              Error: {loadError}
            </p>
          )}
        </div>

        <div className="space-y-3">
          <Button onClick={handleRetry} className="w-full" size="lg">
            <RefreshCw className="mr-2 h-4 w-4" />
            Retry Connection
          </Button>

          <div className="text-sm text-muted-foreground">
            <p>If the problem persists, try:</p>
            <ul className="mt-2 space-y-1 text-left">
              <li>• Restarting the container</li>
              <li>• Checking the container logs</li>
              <li>• Verifying the backend is compiled</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
