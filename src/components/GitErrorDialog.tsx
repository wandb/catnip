import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AlertTriangle, GitBranch } from "lucide-react";

interface GitErrorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  error: string;
  title?: string;
  onRetry?: () => void;
  onForceAction?: () => void;
  forceActionLabel?: string;
  isRetrying?: boolean;
}

export function GitErrorDialog({
  open,
  onOpenChange,
  error,
  title = "Git Operation Failed",
  onRetry,
  onForceAction,
  forceActionLabel = "Force Push",
  isRetrying = false,
}: GitErrorDialogProps) {
  // Detect non-fast-forward error
  const isNonFastForward =
    error.includes("non-fast-forward") ||
    error.includes("rejected") ||
    error.includes("behind its remote counterpart");

  // Extract the git error details
  const getErrorDetails = (error: string) => {
    // Try to extract the meaningful part of the git error
    const lines = error.split("\n").filter((line) => line.trim());

    // Look for git error lines (starting with ! or error:)
    const gitErrorLines = lines.filter(
      (line) =>
        line.includes("! [rejected]") ||
        line.includes("error:") ||
        line.includes("hint:"),
    );

    if (gitErrorLines.length > 0) {
      return gitErrorLines.join("\n");
    }

    return error;
  };

  const errorDetails = getErrorDetails(error);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-red-500" />
            <DialogTitle>{title}</DialogTitle>
          </div>
          {isNonFastForward && (
            <DialogDescription>
              Your branch is behind the remote branch. This usually happens when
              the remote branch has been updated since you started working.
            </DialogDescription>
          )}
        </DialogHeader>

        <div className="space-y-4">
          <div className="bg-muted p-4 rounded-md">
            <h4 className="font-medium mb-2 flex items-center gap-2">
              <GitBranch className="h-4 w-4" />
              Git Error Details
            </h4>
            <pre className="text-sm whitespace-pre-wrap text-muted-foreground">
              {errorDetails}
            </pre>
          </div>

          {isNonFastForward && (
            <div className="bg-blue-50 dark:bg-blue-950/20 p-4 rounded-md border border-blue-200 dark:border-blue-800">
              <h4 className="font-medium text-blue-900 dark:text-blue-100 mb-2">
                What happened?
              </h4>
              <p className="text-sm text-blue-800 dark:text-blue-200 mb-3">
                The remote branch has commits that your local branch doesn't
                have. This typically happens when:
              </p>
              <ul className="list-disc list-inside text-sm text-blue-800 dark:text-blue-200 space-y-1">
                <li>Someone else pushed changes to the same branch</li>
                <li>You're working on a shared branch that has been updated</li>
                <li>The branch was reset or force pushed previously</li>
              </ul>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>

          {onRetry && (
            <Button onClick={onRetry} disabled={isRetrying} variant="secondary">
              {isRetrying ? "Retrying..." : "Try Again"}
            </Button>
          )}

          {isNonFastForward && onForceAction && (
            <Button
              onClick={onForceAction}
              disabled={isRetrying}
              variant="destructive"
            >
              {isRetrying ? "Force Pushing..." : forceActionLabel}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
