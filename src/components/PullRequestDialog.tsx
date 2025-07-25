import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { RefreshCw, Eye } from "lucide-react";
import { toast } from "sonner";
import { GitErrorDialog } from "./GitErrorDialog";

interface PullRequestDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  worktreeId: string;
  branchName: string;
  title: string;
  description: string;
  isUpdate: boolean;
  onTitleChange: (title: string) => void;
  onDescriptionChange: (description: string) => void;
  onRefreshPrStatuses: () => Promise<void>;
}

interface PullRequestResponse {
  number: number;
  title: string;
  url: string;
}

interface ErrorResponse {
  error: string;
}

export function PullRequestDialog({
  open,
  onOpenChange,
  worktreeId,
  branchName,
  title,
  description,
  isUpdate,
  onTitleChange,
  onDescriptionChange,
  onRefreshPrStatuses,
}: PullRequestDialogProps) {
  const [loading, setLoading] = useState(false);
  const [errorDialog, setErrorDialog] = useState<{
    open: boolean;
    error: string;
    title?: string;
  }>({
    open: false,
    error: "",
  });

  const handleForceSubmit = async () => {
    setLoading(true);
    setErrorDialog({ open: false, error: "" }); // Close error dialog

    try {
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/pr`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          title,
          body: description,
          force_push: true, // Add force push flag
        }),
      });

      if (response.ok) {
        const prData = (await response.json()) as PullRequestResponse;

        // Success toast with PR link
        toast.success(
          <div className="flex items-center gap-2 w-full">
            <div className="flex-1">
              <div className="font-medium">
                {isUpdate ? "Pull request updated!" : "Pull request created!"}
              </div>
              <div className="text-sm text-muted-foreground mt-1">
                Force pushed and {isUpdate ? "updated" : "created"} PR
                successfully
              </div>
            </div>
            <button
              onClick={() => window.open(prData.url, "_blank")}
              className="p-1 hover:bg-muted rounded transition-colors"
              title="Open pull request"
            >
              <Eye className="w-4 h-4" />
            </button>
          </div>,
          {
            duration: 10000,
          },
        );

        // Refresh PR statuses after creation/update
        await onRefreshPrStatuses();

        // Close the dialog after successful creation/update
        onOpenChange(false);
      } else {
        // Handle error
        let errorMessage = "Unknown error";
        try {
          const errorData = (await response.json()) as ErrorResponse;
          errorMessage = errorData.error ?? "Unknown error";
        } catch {
          errorMessage = response.statusText || `HTTP ${response.status}`;
        }

        setErrorDialog({
          open: true,
          error: errorMessage,
          title: `Failed to Force ${isUpdate ? "Update" : "Create"} Pull Request`,
        });

        await onRefreshPrStatuses();
      }
    } catch (error) {
      console.error("Failed to force create/update pull request:", error);

      setErrorDialog({
        open: true,
        error: String(error),
        title: `Failed to Force ${isUpdate ? "Update" : "Create"} Pull Request`,
      });

      await onRefreshPrStatuses();
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/pr`, {
        method: isUpdate ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          title,
          body: description,
        }),
      });

      if (response.ok) {
        const prData = (await response.json()) as PullRequestResponse;

        // Success toast with PR link
        toast.success(
          <div className="flex items-center gap-2 w-full">
            <div className="flex-1">
              <div className="font-medium">
                {isUpdate ? "Pull request updated!" : "Pull request created!"}
              </div>
              <div className="text-sm text-muted-foreground mt-1">
                PR #{prData.number}: {prData.title}
              </div>
            </div>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                window.open(prData.url, "_blank");
              }}
              className="p-1 hover:bg-muted rounded transition-colors"
              title="Open pull request"
            >
              <Eye className="w-4 h-4" />
            </button>
          </div>,
          {
            duration: 10000,
          },
        );

        // Refresh PR statuses after creation/update
        await onRefreshPrStatuses();

        // Close the dialog after successful creation/update
        onOpenChange(false);
      } else {
        let errorMessage = "Unknown error";
        try {
          const errorData = (await response.json()) as ErrorResponse;
          errorMessage = errorData.error ?? "Unknown error";
        } catch {
          // If JSON parsing fails, use status text or response text
          errorMessage = response.statusText || `HTTP ${response.status}`;
        }

        // Show error dialog instead of toast
        setErrorDialog({
          open: true,
          error: errorMessage,
          title: `Failed to ${isUpdate ? "Update" : "Create"} Pull Request`,
        });

        // Refresh PR statuses even on failure to prevent stale button state
        await onRefreshPrStatuses();
      }
    } catch (error) {
      console.error("Failed to create/update pull request:", error);

      // Show error dialog instead of toast
      setErrorDialog({
        open: true,
        error: String(error),
        title: `Failed to ${isUpdate ? "Update" : "Create"} Pull Request`,
      });

      // Refresh PR statuses even on error to prevent stale button state
      await onRefreshPrStatuses();
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {isUpdate ? "Update Pull Request" : "Create Pull Request"}
          </DialogTitle>
          <DialogDescription>
            {isUpdate
              ? `Update the pull request for worktree ${branchName}`
              : `Create a pull request for worktree ${branchName}`}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-6 py-4">
          <div className="grid gap-2">
            <Label htmlFor="pr-title" className="text-sm font-medium">
              Title
            </Label>
            <Input
              id="pr-title"
              value={title}
              onChange={(e) => onTitleChange(e.target.value)}
              className="w-full"
              placeholder="Enter a descriptive title for your pull request"
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pr-body" className="text-sm font-medium">
              Description
            </Label>
            <textarea
              id="pr-body"
              value={description}
              onChange={(e) => onDescriptionChange(e.target.value)}
              className="w-full min-h-[300px] rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 md:text-sm resize-vertical"
              placeholder="Enter pull request description..."
            />
          </div>
        </div>
        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => void handleSubmit()} disabled={loading}>
            {loading ? (
              <>
                <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                {isUpdate ? "Updating PR..." : "Creating PR..."}
              </>
            ) : isUpdate ? (
              "Update PR"
            ) : (
              "Create PR"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>

      <GitErrorDialog
        open={errorDialog.open}
        onOpenChange={(open) => setErrorDialog((prev) => ({ ...prev, open }))}
        error={errorDialog.error}
        title={errorDialog.title}
        onRetry={() => void handleSubmit()}
        onForceAction={() => void handleForceSubmit()}
        forceActionLabel="Force Push"
        isRetrying={loading}
      />
    </Dialog>
  );
}
