import { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
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
import { type Worktree, type PullRequestInfo } from "@/lib/git-api";
import { type WorktreeSummary } from "@/lib/worktree-summary";

interface PullRequestDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  worktree: Worktree;
  prStatus?: PullRequestInfo;
  summary?: WorktreeSummary;
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
  worktree,
  prStatus,
  summary,
  onRefreshPrStatuses,
}: PullRequestDialogProps) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [isUpdate, setIsUpdate] = useState(false);
  const [isGenerating, setIsGenerating] = useState(false);
  const lastClaudeCallRef = useRef<number>(0);
  const [loading, setLoading] = useState(false);
  const [errorDialog, setErrorDialog] = useState<{
    open: boolean;
    error: string;
    title?: string;
  }>({
    open: false,
    error: "",
  });

  // Generate PR content when dialog opens (only for new PRs)
  useEffect(() => {
    if (open && worktree) {
      // Check if this is an existing PR - if so, skip generation and use existing data
      const isExistingPR = prStatus?.exists || !!worktree.pull_request_url;

      if (isExistingPR) {
        // Set update mode and use existing PR data
        setIsUpdate(true);

        // Prioritize persisted data from worktree, then prStatus, then fallback
        if (worktree.pull_request_title && worktree.pull_request_body) {
          // Use persisted data from worktree (this is what we want!)
          setTitle(worktree.pull_request_title);
          setDescription(worktree.pull_request_body);
        } else if (prStatus?.title) {
          // Fallback to prStatus if available
          setTitle(prStatus.title);
          setDescription(prStatus.body || "");
        } else {
          // Final fallback for existing PR without any stored data
          setTitle(`Update ${worktree.branch}`);
          setDescription(`Updated changes for ${worktree.branch} branch`);
        }
        setIsGenerating(false);
      } else {
        // New PR - generate content with Claude
        void generatePrContent();
      }
    }
  }, [open, worktree?.id, prStatus?.exists, worktree.pull_request_url]);

  const generatePrContent = async () => {
    console.log("🚀 Generating new PR content for:", {
      worktreeId: worktree.id,
      branchName: worktree.branch,
    });
    console.log("📋 Summary:", summary);
    console.log("⏰ lastClaudeCall:", lastClaudeCallRef.current);

    // This function is only called for new PRs now
    setIsUpdate(false);

    // Check throttling - only allow Claude call once every 10 seconds
    const now = Date.now();
    const shouldCallClaude = now - lastClaudeCallRef.current > 10000; // 10 seconds
    console.log(
      "🤖 shouldCallClaude:",
      shouldCallClaude,
      "time since last call:",
      now - lastClaudeCallRef.current,
    );

    if (!shouldCallClaude) {
      console.log("⏸️ Throttled - using fallback data");
      // Use fallback data without calling Claude
      const fallbackTitle =
        summary?.status === "completed" && summary.title
          ? summary.title
          : `Pull request from ${worktree.branch}`;

      const fallbackDescription =
        summary?.status === "completed" && summary.summary
          ? summary.summary
          : `Automated pull request created from worktree ${worktree.branch}`;

      console.log("📝 Fallback data:", { fallbackTitle, fallbackDescription });

      setTitle(fallbackTitle);
      setDescription(fallbackDescription);
      setIsGenerating(false);
      return;
    }

    // Open dialog with loading state and call Claude
    console.log("🔄 Calling Claude API for PR generation");
    setTitle("");
    setDescription("");
    setIsGenerating(true);

    // Update throttle timestamp
    lastClaudeCallRef.current = now;

    try {
      // Prepare prompt for Claude - it already has the session context
      const prompt = `I need you to generate a pull request title and description for the branch "${worktree.branch}" based on all the changes we've made in this session.

Please respond with JSON in the following format:
\`\`\`json
{
  "title": "Brief, descriptive title of the changes",
  "description": "Focused description of what was changed and why, formatted in markdown"
}
\`\`\`

Make the title concise but descriptive. Keep the description focused but informative - use 1-3 paragraphs explaining:
- What was changed
- Why it was changed
- Any key implementation notes

Avoid overly lengthy explanations or step-by-step implementation details.`;

      // Call Claude API
      const requestBody = {
        prompt: prompt,
        working_directory: `/workspace/${worktree.name}`,
        resume: true,
        max_turns: 1,
      };
      console.log("📤 Sending to Claude API:", requestBody);

      const response = await fetch("/v1/claude/messages", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(requestBody),
      });

      if (response.ok) {
        const data = await response.json();
        console.log("✅ Claude API response received:", data);

        // Extract JSON from Claude's response
        let parsedData = { title: "", description: "" };
        try {
          // The response is in data.response field
          const responseText = data.response || data.message || "";
          console.log("🔍 Parsing response text:", responseText);

          // Look for JSON in code fence (handle newlines properly)
          const jsonMatch = responseText.match(/```json\s*([\s\S]*?)\s*```/m);
          if (jsonMatch) {
            console.log("🎯 Extracted JSON from code fence:", jsonMatch[1]);
            parsedData = JSON.parse(jsonMatch[1]);
          } else {
            console.log(
              "🔍 No code fence found, trying to parse whole response as JSON",
            );
            // Try parsing the whole response as JSON
            parsedData = JSON.parse(responseText);
          }
        } catch (e) {
          console.error("Failed to parse Claude's response as JSON:", e);
          // Fallback to using the raw response
          parsedData = {
            title: `PR: ${worktree.branch}`,
            description:
              data.response || data.message || "Generated PR content",
          };
        }

        // Update dialog with generated content
        setTitle(parsedData.title || `Pull request from ${worktree.branch}`);
        setDescription(
          parsedData.description || `Changes from worktree ${worktree.branch}`,
        );
        setIsGenerating(false);
      } else {
        console.error("❌ Claude API failed with status:", response.status);
        const errorText = await response.text();
        console.error("❌ Error details:", errorText);

        // Fallback to summary or defaults
        const fallbackTitle =
          summary?.status === "completed" && summary.title
            ? summary.title
            : `Pull request from ${worktree.branch}`;

        const fallbackDescription =
          summary?.status === "completed" && summary.summary
            ? summary.summary
            : `Automated pull request created from worktree ${worktree.branch}`;

        setTitle(fallbackTitle);
        setDescription(fallbackDescription);
        setIsGenerating(false);
      }
    } catch (error) {
      console.error("Error generating PR details:", error);
      // Fallback to summary or defaults
      const fallbackTitle =
        summary?.status === "completed" && summary.title
          ? summary.title
          : `Pull request from ${worktree.branch}`;

      const fallbackDescription =
        summary?.status === "completed" && summary.summary
          ? summary.summary
          : `Automated pull request created from worktree ${worktree.branch}`;

      setTitle(fallbackTitle);
      setDescription(fallbackDescription);
      setIsGenerating(false);
    }
  };

  const handleForceSubmit = async () => {
    setLoading(true);
    setErrorDialog({ open: false, error: "" }); // Close error dialog

    try {
      const response = await fetch(`/v1/git/worktrees/${worktree.id}/pr`, {
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
      const response = await fetch(`/v1/git/worktrees/${worktree.id}/pr`, {
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
              ? `Update the pull request for worktree ${worktree.branch}`
              : `Create a pull request for worktree ${worktree.branch}`}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-6 py-4">
          <div className="grid gap-2">
            <Label htmlFor="pr-title" className="text-sm font-medium">
              Title
            </Label>
            {isGenerating && !title ? (
              <div className="space-y-2">
                <div className="text-sm text-muted-foreground italic">
                  Claude is generating a PR summary...
                </div>
                <Skeleton className="h-10 w-full" />
              </div>
            ) : (
              <Input
                id="pr-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                className="w-full"
                placeholder="Enter a descriptive title for your pull request"
              />
            )}
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pr-body" className="text-sm font-medium">
              Description
            </Label>
            {isGenerating && !description ? (
              <div className="space-y-3">
                <div className="text-sm text-muted-foreground italic">
                  Generating detailed description...
                </div>
                <div className="space-y-2">
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-5/6" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-2/3" />
                </div>
              </div>
            ) : (
              <textarea
                id="pr-body"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                className="w-full min-h-[300px] rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 md:text-sm resize-vertical"
                placeholder="Enter pull request description..."
              />
            )}
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
