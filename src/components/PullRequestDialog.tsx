import { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import { useMediaQuery } from "@/hooks/use-media-query";
import { RefreshCw, Eye } from "lucide-react";
import { toast } from "sonner";
import { GitErrorDialog } from "./GitErrorDialog";
import {
  type Worktree,
  type PullRequestInfo,
  type LocalRepository,
  gitApi,
} from "@/lib/git-api";
import { type WorktreeSummary } from "@/lib/worktree-summary";
import { cn } from "@/lib/utils";

interface PullRequestDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  worktree: Worktree;
  repository?: LocalRepository;
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

function PullRequestForm({
  className,
  title,
  description,
  isGenerating,
  onTitleChange,
  onDescriptionChange,
}: {
  className?: string;
  title: string;
  description: string;
  isGenerating: boolean;
  onTitleChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
}) {
  return (
    <div className={cn("grid gap-6 py-4", className)}>
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
            onChange={(e) => onTitleChange(e.target.value)}
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
            onChange={(e) => onDescriptionChange(e.target.value)}
            className="w-full min-h-[300px] rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 md:text-sm resize-vertical"
            placeholder="Enter pull request description..."
          />
        )}
      </div>
    </div>
  );
}

export function PullRequestDialog({
  open,
  onOpenChange,
  worktree,
  repository,
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
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDesktop = useMediaQuery("(min-width: 768px)");
  const [errorDialog, setErrorDialog] = useState<{
    open: boolean;
    error: string;
    title?: string;
  }>({
    open: false,
    error: "",
  });

  const [showCreateRepoDialog, setShowCreateRepoDialog] = useState(false);
  const [repoCreationForm, setRepoCreationForm] = useState({
    name: "",
    description: "",
    isPrivate: true,
  });

  // Generate PR content when dialog opens (only for new PRs)
  useEffect(() => {
    if (open && worktree && !showCreateRepoDialog) {
      // Check if this is a local repo without GitHub remote
      // Local repos are identified by file:// URLs or repos without GitHub remote
      if (
        repository &&
        !repository.has_github_remote &&
        (repository.url.startsWith("file://") ||
          worktree.repo_id.startsWith("local/"))
      ) {
        // Initialize repo creation form with sensible defaults
        const repoName = worktree.repo_id.startsWith("local/")
          ? worktree.repo_id.replace("local/", "")
          : worktree.repo_id.split("/").pop() || worktree.repo_id;

        setRepoCreationForm({
          name: repoName,
          description: repository.description || `Repository for ${repoName}`,
          isPrivate: true, // Default to private for security
        });
        setShowCreateRepoDialog(true);
        return;
      }

      // Check if this is an existing PR - if so, skip generation and use existing data
      const isExistingPR = prStatus?.exists || !!worktree.pull_request_url;

      if (isExistingPR) {
        // Set update mode and fetch fresh PR data from GitHub
        setIsUpdate(true);
        setIsGenerating(true);

        // Fetch current PR info from GitHub to get the latest title/body
        void fetchCurrentPrInfo();
      } else {
        // New PR - generate content with Claude
        void generatePrContent();
      }
    }
  }, [
    open,
    worktree?.id,
    prStatus?.exists,
    worktree.pull_request_url,
    repository?.has_github_remote,
    showCreateRepoDialog,
  ]);

  const fetchCurrentPrInfo = async () => {
    try {
      const prInfo = await gitApi.getPullRequestInfo(worktree.id);

      if (prInfo?.title && prInfo?.body) {
        // Use fresh data from GitHub
        setTitle(prInfo.title);
        setDescription(prInfo.body);
      } else if (worktree.pull_request_title && worktree.pull_request_body) {
        // Fallback to persisted data, but avoid the error message
        if (
          worktree.pull_request_body.includes("No text content found") ||
          worktree.pull_request_body.includes(
            "Claude returned an empty response",
          )
        ) {
          // Skip the error message and use a clean fallback
          setTitle(worktree.pull_request_title);
          setDescription(`Updated changes for ${worktree.branch} branch`);
        } else {
          setTitle(worktree.pull_request_title);
          setDescription(worktree.pull_request_body);
        }
      } else {
        // Final fallback
        setTitle(`Update ${worktree.branch}`);
        setDescription(`Updated changes for ${worktree.branch} branch`);
      }
    } catch (error) {
      console.error("❌ Failed to fetch PR info:", error);
      // Fallback to persisted data if available
      if (
        worktree.pull_request_title &&
        worktree.pull_request_body &&
        !worktree.pull_request_body.includes("No text content found") &&
        !worktree.pull_request_body.includes(
          "Claude returned an empty response",
        )
      ) {
        setTitle(worktree.pull_request_title);
        setDescription(worktree.pull_request_body);
      } else {
        setTitle(`Update ${worktree.branch}`);
        setDescription(`Updated changes for ${worktree.branch} branch`);
      }
    } finally {
      setIsGenerating(false);
    }
  };

  const generatePrContent = async () => {
    // This function is only called for new PRs now
    setIsUpdate(false);

    // Check throttling - only allow Claude call once every 10 seconds
    const now = Date.now();
    const shouldCallClaude = now - lastClaudeCallRef.current > 10000; // 10 seconds

    if (!shouldCallClaude) {
      // Use fallback data without calling Claude
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
      return;
    }

    // Open dialog with loading state and call Claude
    setTitle("");
    setDescription("");
    setIsGenerating(true);

    // Create abort controller for this generation request
    const abortController = new AbortController();
    abortControllerRef.current = abortController;

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
        resume: true, // Resume session to get context (backend defaults to fork=true and haiku model)
        max_turns: 1,
        suppress_events: true, // This is an automated operation - suppress stop events
        disable_tools: true, // Don't use tools, just rely on session context
      };

      const response = await fetch("/v1/claude/messages", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(requestBody),
        signal: abortController.signal,
      });

      if (response.ok) {
        const data = await response.json();

        // Extract JSON from Claude's response
        let parsedData = { title: "", description: "" };
        try {
          // The response is in data.response field
          const responseText = data.response || data.message || "";

          // Look for JSON in code fence (handle newlines properly)
          const jsonMatch = responseText.match(/```json\s*([\s\S]*?)\s*```/m);
          if (jsonMatch) {
            parsedData = JSON.parse(jsonMatch[1]);
          } else {
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
        console.error("Claude API failed with status:", response.status);

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
      // Check if this was an abort - if so, don't log error or set fallback
      if (error instanceof DOMException && error.name === "AbortError") {
        console.log("PR content generation was cancelled");
        return; // Don't set fallback values, keep current state
      }

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
    } finally {
      // Clean up abort controller reference
      abortControllerRef.current = null;
    }
  };

  const handleCancelGeneration = () => {
    // Abort the ongoing generation request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }

    // Set fallback values
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
  };

  const handleCreateGitHubRepository = async () => {
    if (!repository) return;

    setLoading(true);
    try {
      const result = await gitApi.createGitHubRepository(
        repository.id,
        repoCreationForm.name,
        repoCreationForm.description,
        repoCreationForm.isPrivate,
      );

      toast.success(
        <div className="flex items-center gap-2">
          <div className="flex-1">
            <div className="font-medium">GitHub repository created!</div>
            <div className="text-sm text-muted-foreground mt-1">
              {result.message}
            </div>
          </div>
        </div>,
        { duration: 5000 },
      );

      // Close the repo creation dialog
      setShowCreateRepoDialog(false);

      // Close the parent PR dialog as well since we'll need to reopen it with fresh data
      onOpenChange(false);

      // Note: Repository data should be updated via SSE events automatically
      // User can click "Create PR" again once the repository is refreshed
    } catch (error) {
      console.error("Failed to create GitHub repository:", error);
      setErrorDialog({
        open: true,
        error: String(error),
        title: "Failed to Create GitHub Repository",
      });
    } finally {
      setLoading(false);
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

  if (isDesktop) {
    return (
      <>
        {showCreateRepoDialog && (
          <Dialog
            open={showCreateRepoDialog}
            onOpenChange={(open) => {
              setShowCreateRepoDialog(open);
              if (!open) {
                // Also close the parent dialog when closing the create repo dialog
                onOpenChange(false);
              }
            }}
          >
            <DialogContent className="max-w-lg">
              <DialogHeader>
                <DialogTitle>Create GitHub Repository</DialogTitle>
                <DialogDescription>
                  This local repository doesn't have a GitHub remote. Create a
                  GitHub repository to enable pull requests.
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-4 py-4">
                <div className="grid gap-2">
                  <Label htmlFor="repo-name">Repository Name</Label>
                  <Input
                    id="repo-name"
                    value={repoCreationForm.name}
                    onChange={(e) =>
                      setRepoCreationForm((prev) => ({
                        ...prev,
                        name: e.target.value,
                      }))
                    }
                    placeholder="my-awesome-project"
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="repo-description">Description</Label>
                  <Input
                    id="repo-description"
                    value={repoCreationForm.description}
                    onChange={(e) =>
                      setRepoCreationForm((prev) => ({
                        ...prev,
                        description: e.target.value,
                      }))
                    }
                    placeholder="A brief description of your project"
                  />
                </div>
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="repo-private"
                    checked={repoCreationForm.isPrivate}
                    onCheckedChange={(checked: boolean) =>
                      setRepoCreationForm((prev) => ({
                        ...prev,
                        isPrivate: checked,
                      }))
                    }
                  />
                  <Label htmlFor="repo-private" className="text-sm font-normal">
                    Make repository private
                  </Label>
                </div>
              </div>
              <DialogFooter>
                <Button
                  variant="outline"
                  onClick={() => {
                    setShowCreateRepoDialog(false);
                    onOpenChange(false); // Also close parent dialog
                  }}
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleCreateGitHubRepository}
                  disabled={loading || !repoCreationForm.name}
                >
                  {loading ? (
                    <>
                      <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                      Creating...
                    </>
                  ) : (
                    "Create Repository"
                  )}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        )}
        <Dialog
          open={open && !showCreateRepoDialog}
          onOpenChange={onOpenChange}
        >
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

            {/* Error Alert */}
            {errorDialog.open && (
              <div className="bg-red-600 border border-red-700 text-red-100 px-4 py-3 rounded-md mb-4">
                <div className="flex items-start gap-2">
                  <div className="flex-shrink-0">⚠️</div>
                  <div className="flex-1">
                    <div className="font-medium text-sm">
                      {errorDialog.title}
                    </div>
                    <div className="text-sm mt-1">{errorDialog.error}</div>
                  </div>
                  <button
                    onClick={() =>
                      setErrorDialog((prev) => ({ ...prev, open: false }))
                    }
                    className="flex-shrink-0 text-red-600 hover:text-red-800"
                  >
                    ✕
                  </button>
                </div>
              </div>
            )}

            <PullRequestForm
              title={title}
              description={description}
              isGenerating={isGenerating}
              onTitleChange={setTitle}
              onDescriptionChange={setDescription}
            />
            <DialogFooter className="gap-2">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                onClick={
                  isGenerating
                    ? handleCancelGeneration
                    : () => void handleSubmit()
                }
                disabled={loading}
              >
                {loading ? (
                  <>
                    <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                    {isUpdate ? "Updating PR..." : "Creating PR..."}
                  </>
                ) : isGenerating ? (
                  "Cancel Generation"
                ) : isUpdate ? (
                  "Update PR"
                ) : (
                  "Create PR"
                )}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </>
    );
  }

  return (
    <>
      <Drawer open={open && !showCreateRepoDialog} onOpenChange={onOpenChange}>
        <DrawerContent>
          <DrawerHeader className="text-left">
            <DrawerTitle>
              {isUpdate ? "Update Pull Request" : "Create Pull Request"}
            </DrawerTitle>
            <DrawerDescription>
              {isUpdate
                ? `Update the pull request for worktree ${worktree.branch}`
                : `Create a pull request for worktree ${worktree.branch}`}
            </DrawerDescription>
          </DrawerHeader>
          <PullRequestForm
            className="px-4"
            title={title}
            description={description}
            isGenerating={isGenerating}
            onTitleChange={setTitle}
            onDescriptionChange={setDescription}
          />
          <DrawerFooter className="pt-2">
            <Button
              onClick={
                isGenerating
                  ? handleCancelGeneration
                  : () => void handleSubmit()
              }
              disabled={loading}
            >
              {loading ? (
                <>
                  <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                  {isUpdate ? "Updating PR..." : "Creating PR..."}
                </>
              ) : isGenerating ? (
                "Cancel Generation"
              ) : isUpdate ? (
                "Update PR"
              ) : (
                "Create PR"
              )}
            </Button>
            <DrawerClose asChild>
              <Button variant="outline">Cancel</Button>
            </DrawerClose>
          </DrawerFooter>
        </DrawerContent>
      </Drawer>

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

      {/* Create GitHub Repository Dialog - Mobile */}
      {!isDesktop && showCreateRepoDialog && (
        <Drawer
          open={showCreateRepoDialog}
          onOpenChange={setShowCreateRepoDialog}
        >
          <DrawerContent>
            <DrawerHeader className="text-left">
              <DrawerTitle>Create GitHub Repository</DrawerTitle>
              <DrawerDescription>
                This local repository doesn't have a GitHub remote. Create a
                GitHub repository to enable pull requests.
              </DrawerDescription>
            </DrawerHeader>
            <div className="grid gap-4 px-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor="repo-name-mobile">Repository Name</Label>
                <Input
                  id="repo-name-mobile"
                  value={repoCreationForm.name}
                  onChange={(e) =>
                    setRepoCreationForm((prev) => ({
                      ...prev,
                      name: e.target.value,
                    }))
                  }
                  placeholder="my-awesome-project"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="repo-description-mobile">Description</Label>
                <Input
                  id="repo-description-mobile"
                  value={repoCreationForm.description}
                  onChange={(e) =>
                    setRepoCreationForm((prev) => ({
                      ...prev,
                      description: e.target.value,
                    }))
                  }
                  placeholder="A brief description of your project"
                />
              </div>
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="repo-private-mobile"
                  checked={repoCreationForm.isPrivate}
                  onCheckedChange={(checked: boolean) =>
                    setRepoCreationForm((prev) => ({
                      ...prev,
                      isPrivate: checked,
                    }))
                  }
                />
                <Label
                  htmlFor="repo-private-mobile"
                  className="text-sm font-normal"
                >
                  Make repository private
                </Label>
              </div>
            </div>
            <DrawerFooter className="pt-2">
              <Button
                onClick={handleCreateGitHubRepository}
                disabled={loading || !repoCreationForm.name}
              >
                {loading ? (
                  <>
                    <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                    Creating...
                  </>
                ) : (
                  "Create Repository"
                )}
              </Button>
              <DrawerClose asChild>
                <Button variant="outline">Cancel</Button>
              </DrawerClose>
            </DrawerFooter>
          </DrawerContent>
        </Drawer>
      )}
    </>
  );
}
