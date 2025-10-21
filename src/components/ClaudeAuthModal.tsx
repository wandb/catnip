import { useState, useEffect } from "react";
import { useClaudeAuth } from "@/lib/hooks";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Loader2, ExternalLink, CheckCircle2, XCircle } from "lucide-react";

interface ClaudeAuthModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface OnboardingStatus {
  state: string;
  oauth_url?: string;
  message?: string;
  error_message?: string;
}

export function ClaudeAuthModal({ open, onOpenChange }: ClaudeAuthModalProps) {
  const { resetAuthState } = useClaudeAuth();
  const [status, setStatus] = useState<OnboardingStatus>({ state: "idle" });
  const [loading, setLoading] = useState(false);
  const [polling, setPolling] = useState(false);
  const [code, setCode] = useState("");
  const [submittingCode, setSubmittingCode] = useState(false);

  // Start onboarding flow when modal opens
  useEffect(() => {
    if (open && status.state === "idle") {
      void startOnboarding();
    }
  }, [open]);

  // Poll for status when onboarding is in progress
  useEffect(() => {
    let interval: number | null = null;

    if (polling && status.state !== "complete" && status.state !== "error") {
      interval = window.setInterval(async () => {
        try {
          const response = await fetch("/v1/claude/onboarding/status");
          const data: OnboardingStatus = await response.json();
          setStatus(data);

          // Stop polling when we reach a terminal state or auth_waiting
          if (
            data.state === "complete" ||
            data.state === "error" ||
            data.state === "auth_waiting"
          ) {
            setPolling(false);
          }
        } catch (error) {
          console.error("Failed to check onboarding status:", error);
        }
      }, 1000); // Poll every second
    }

    return () => {
      if (interval) {
        clearInterval(interval);
      }
    };
  }, [polling, status.state]);

  const startOnboarding = async () => {
    setLoading(true);
    setStatus({ state: "idle" });
    setCode("");

    try {
      const response = await fetch("/v1/claude/onboarding/start", {
        method: "POST",
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(
          errorData.error ||
            `HTTP ${response.status}: Failed to start onboarding`,
        );
      }

      // Start polling for status
      setPolling(true);
    } catch (error) {
      let errorMessage = "Failed to start onboarding";

      if (error instanceof Error) {
        errorMessage = error.message;
      }

      setStatus({
        state: "error",
        error_message: errorMessage,
      });
    } finally {
      setLoading(false);
    }
  };

  const openOAuthURL = () => {
    if (status.oauth_url) {
      window.open(status.oauth_url, "_blank");
    }
  };

  const submitCode = async () => {
    if (!code.trim()) {
      return;
    }

    setSubmittingCode(true);

    try {
      const response = await fetch("/v1/claude/onboarding/submit-code", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ code: code.trim() }),
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(
          errorData.details || errorData.error || "Failed to submit code",
        );
      }

      // Resume polling to wait for completion
      setPolling(true);
    } catch (error) {
      let errorMessage = "Failed to submit authentication code";

      if (error instanceof Error) {
        errorMessage = error.message;
      }

      setStatus({
        state: "error",
        error_message: errorMessage,
      });
    } finally {
      setSubmittingCode(false);
    }
  };

  const handleClose = async (dismissed = false) => {
    // If user dismisses modal (not successful auth), remember their choice and reset backend state
    if (dismissed) {
      localStorage.setItem("claude-auth-dismissed", "true");
      await resetAuthState();
    }

    onOpenChange(false);
    // Reset state after closing
    setTimeout(() => {
      setStatus({ state: "idle" });
      setPolling(false);
      setCode("");
    }, 300);
  };

  const handleSuccessClose = () => {
    // For successful auth, just close the modal without resetting anything
    onOpenChange(false);
    // Reset only the local modal state
    setTimeout(() => {
      setStatus({ state: "idle" });
      setPolling(false);
      setCode("");
    }, 300);
  };

  const isWaitingForAuth =
    status.state === "auth_waiting" || status.state === "auth_url";
  const isProcessing =
    polling ||
    loading ||
    (status.state !== "idle" &&
      status.state !== "auth_waiting" &&
      status.state !== "complete" &&
      status.state !== "error");

  return (
    <Dialog open={open} onOpenChange={(open) => !open && handleClose(true)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Claude Authentication</DialogTitle>
          <DialogDescription>
            Authenticate with Claude to enable AI-powered coding features. If
            you don't want to connect to Claude, you can dismiss this modal.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {isProcessing && !isWaitingForAuth && (
            <div className="flex flex-col items-center justify-center py-8 space-y-3">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              <p className="text-sm text-muted-foreground">
                {status.message || "Setting up authentication..."}
              </p>
            </div>
          )}

          {isWaitingForAuth && status.oauth_url && (
            <div className="space-y-4">
              <div className="rounded-lg border bg-muted p-4">
                <p className="mb-3 text-sm text-muted-foreground">
                  Click the button below to open the Claude authentication page
                  in your browser:
                </p>
                <Button onClick={openOAuthURL} className="w-full">
                  <ExternalLink className="mr-2 h-4 w-4" />
                  Login with Claude
                </Button>
              </div>

              <div className="space-y-2">
                <label
                  htmlFor="auth-code"
                  className="text-sm font-medium text-foreground"
                >
                  After logging in, paste your authentication code here:
                </label>
                <Input
                  id="auth-code"
                  type="text"
                  placeholder="Enter authentication code"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !submittingCode) {
                      void submitCode();
                    }
                  }}
                  disabled={submittingCode}
                />
                <Button
                  onClick={submitCode}
                  disabled={!code.trim() || submittingCode}
                  className="w-full"
                >
                  {submittingCode ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Submitting...
                    </>
                  ) : (
                    "Submit Code"
                  )}
                </Button>
              </div>
            </div>
          )}

          {status.state === "complete" && (
            <div className="space-y-4">
              <Alert className="border-green-600 bg-green-50 dark:bg-green-950">
                <CheckCircle2 className="h-4 w-4 text-green-600" />
                <AlertDescription className="text-green-800 dark:text-green-200">
                  Successfully authenticated with Claude!
                </AlertDescription>
              </Alert>
              <Button onClick={handleSuccessClose} className="w-full">
                Close
              </Button>
            </div>
          )}

          {status.state === "error" && (
            <div className="space-y-4">
              <Alert className="border-destructive">
                <XCircle className="h-4 w-4 text-destructive" />
                <AlertDescription className="text-destructive">
                  {status.error_message || "Authentication failed"}
                </AlertDescription>
              </Alert>
              <div className="flex gap-2">
                <Button
                  onClick={startOnboarding}
                  variant="outline"
                  className="flex-1"
                >
                  Try Again
                </Button>
                <Button
                  onClick={() => handleClose(true)}
                  variant="outline"
                  className="flex-1"
                >
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
