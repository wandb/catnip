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
import { toast } from "sonner";

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
  const [hasClickedOAuthLink, setHasClickedOAuthLink] = useState(false);
  const [hasSubmittedCode, setHasSubmittedCode] = useState(false);

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

          // Stop polling when we reach terminal states
          // If code has been submitted, keep polling through all intermediate states
          // until we reach complete or error
          if (data.state === "complete" || data.state === "error") {
            setPolling(false);
          } else if (data.state === "auth_waiting" && !hasSubmittedCode) {
            // Only stop on auth_waiting if we haven't submitted code yet
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
  }, [polling, status.state, hasSubmittedCode]);

  // Auto-dismiss modal on successful authentication
  useEffect(() => {
    if (status.state === "complete") {
      // Wait 2 seconds to let user see the success message, then auto-dismiss
      const timer = setTimeout(() => {
        handleSuccessClose();
      }, 2000);

      return () => clearTimeout(timer);
    }
  }, [status.state]);

  const startOnboarding = async () => {
    setLoading(true);
    setStatus({ state: "idle" });
    setCode("");
    setHasSubmittedCode(false);

    try {
      const response = await fetch("/v1/claude/onboarding/start", {
        method: "POST",
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(
          data.error || `HTTP ${response.status}: Failed to start onboarding`,
        );
      }

      // Check if we got a "resumed" response (onboarding already in progress)
      if (data.status === "resumed") {
        console.log("Onboarding already in progress, resuming polling...");

        // Get current status
        const statusResponse = await fetch("/v1/claude/onboarding/status");
        if (statusResponse.ok) {
          const currentStatus: OnboardingStatus = await statusResponse.json();
          setStatus(currentStatus);
        }

        // Resume polling
        setPolling(true);
        setLoading(false);
        return;
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
      setHasClickedOAuthLink(true);
    }
  };

  const submitCode = async () => {
    if (!code.trim()) {
      console.log("Submit code: code is empty");
      return;
    }

    console.log("Submitting code:", code.trim());
    setSubmittingCode(true);
    setHasSubmittedCode(true); // Mark that we've submitted code

    try {
      const response = await fetch("/v1/claude/onboarding/submit-code", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ code: code.trim() }),
      });

      console.log("Submit code response:", response.status);

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(
          errorData.details || errorData.error || "Failed to submit code",
        );
      }

      // Resume polling to wait for completion
      setPolling(true);
    } catch (error) {
      console.error("Submit code error:", error);
      let errorMessage = "Failed to submit authentication code";

      if (error instanceof Error) {
        errorMessage = error.message;
      }

      setStatus({
        state: "error",
        error_message: errorMessage,
      });
      setHasSubmittedCode(false); // Reset on error so user can retry
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
      setHasSubmittedCode(false);
    }, 300);
  };

  const handleSuccessClose = () => {
    // For successful auth, show toast and close the modal
    toast.success("Connected to Claude", {
      description: "Your sessions are refreshing.",
    });
    onOpenChange(false);
    // Reset only the local modal state
    setTimeout(() => {
      setStatus({ state: "idle" });
      setPolling(false);
      setCode("");
      setHasClickedOAuthLink(false);
      setHasSubmittedCode(false);
    }, 300);
  };

  // Treat critical error messages as error state even if state hasn't transitioned
  const hasCriticalError = status.error_message?.includes(
    "Connection to authentication process lost",
  );
  const isEffectiveErrorState = status.state === "error" || hasCriticalError;

  const isWaitingForAuth =
    (status.state === "auth_waiting" || status.state === "auth_url") &&
    !hasCriticalError;
  const isProcessing =
    polling ||
    loading ||
    (status.state !== "idle" &&
      status.state !== "auth_waiting" &&
      status.state !== "complete" &&
      status.state !== "error" &&
      !hasCriticalError);

  return (
    <Dialog open={open} onOpenChange={(open) => !open && handleClose(true)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Login to Claude</DialogTitle>
          <DialogDescription>
            Connect your Claude account to start vibing.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {isProcessing && !isWaitingForAuth && (
            <div className="flex flex-col items-center justify-center py-8 space-y-3">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              <p className="text-sm text-muted-foreground">
                {status.message || "Setting up..."}
              </p>
            </div>
          )}

          {isWaitingForAuth && status.oauth_url && (
            <div className="space-y-4">
              {status.error_message && (
                <Alert className="border-yellow-600 bg-yellow-50 dark:bg-yellow-950">
                  <AlertDescription className="text-yellow-800 dark:text-yellow-200">
                    {status.error_message}
                  </AlertDescription>
                </Alert>
              )}

              {!hasClickedOAuthLink ? (
                <div className="rounded-lg border bg-muted p-4">
                  <Button onClick={openOAuthURL} className="w-full">
                    <ExternalLink className="mr-2 h-4 w-4" />
                    Open Login Page
                  </Button>
                </div>
              ) : (
                <div className="rounded-lg border bg-muted p-4">
                  <p className="text-sm text-muted-foreground">
                    ✓ Login page opened. Complete authentication and paste your
                    code below.
                  </p>
                </div>
              )}

              {hasClickedOAuthLink && (
                <div className="space-y-2">
                  <label
                    htmlFor="auth-code"
                    className="text-sm font-medium text-foreground"
                  >
                    Authentication code:
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
              )}
            </div>
          )}

          {status.state === "complete" && (
            <div className="space-y-4">
              <Alert className="border-green-600 bg-green-50 dark:bg-green-950">
                <CheckCircle2 className="h-4 w-4 text-green-600" />
                <AlertDescription className="text-green-800 dark:text-green-200">
                  Connected successfully!
                </AlertDescription>
              </Alert>
              <Button onClick={handleSuccessClose} className="w-full">
                Close
              </Button>
            </div>
          )}

          {isEffectiveErrorState && (
            <div className="space-y-4">
              <Alert className="border-destructive">
                <XCircle className="h-4 w-4 text-destructive" />
                <AlertDescription className="text-destructive">
                  {status.error_message || "Authentication failed"}
                </AlertDescription>
              </Alert>

              {status.error_message?.includes("run 'claude' directly") && (
                <div className="rounded-lg border bg-muted p-4 space-y-2">
                  <p className="text-sm font-medium">Manual Authentication:</p>
                  <ol className="text-sm text-muted-foreground space-y-1 list-decimal list-inside">
                    <li>Open a terminal in your project directory</li>
                    <li>
                      Run:{" "}
                      <code className="bg-background px-1 py-0.5 rounded">
                        claude
                      </code>
                    </li>
                    <li>Follow the authentication prompts</li>
                    <li>Reload this page once authenticated</li>
                  </ol>
                </div>
              )}

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
