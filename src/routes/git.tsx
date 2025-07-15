import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { RepoSelector } from "@/components/RepoSelector";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorAlert } from "@/components/ErrorAlert";
import { WorktreeRow } from "@/components/WorktreeRow";
import {
  GitBranch,
  Copy,
  RefreshCw,
  Eye,
} from "lucide-react";
import { toast } from "sonner";
import { copyRemoteCommand, showPreviewToast, parseGitUrl } from "@/lib/git-utils";
import { gitApi } from "@/lib/git-api";
import { useGitState } from "@/hooks/useGitState";


function GitPage() {
  const {
    gitStatus,
    worktrees,
    repositories,
    repoBranches,
    claudeSessions,
    syncConflicts,
    mergeConflicts,
    worktreeSummaries,
    diffStats,
    prStatuses,
    loading,
    reposLoading,
    fetchGitStatus,
    fetchWorktrees,
    fetchRepositories,
    fetchActiveSessions,
    fetchPRStatuses,
    generateWorktreeSummaryForId,
    refreshAll,
    setLoading,
  } = useGitState();

  const [githubUrl, setGithubUrl] = useState("");
  const [openDiffWorktreeId, setOpenDiffWorktreeId] = useState<string | null>(null);
  const [confirmDialog, setConfirmDialog] = useState<{
    open: boolean;
    title: string;
    description: string;
    onConfirm: () => void;
    variant?: "default" | "destructive";
  }>({
    open: false,
    title: "",
    description: "",
    onConfirm: () => {},
  });
  const [errorAlert, setErrorAlert] = useState<{
    open: boolean;
    title: string;
    description: string;
  }>({
    open: false,
    title: "",
    description: "",
  });

  const [prDialog, setPrDialog] = useState<{
    open: boolean;
    worktreeId: string;
    branchName: string;
    title: string;
    description: string;
    isUpdate: boolean;
  }>({
    open: false,
    worktreeId: "",
    branchName: "",
    title: "",
    description: "",
    isUpdate: false,
  });

  const [prLoading, setPrLoading] = useState(false);
  const handleCheckout = async (url: string) => {
    setLoading(true);
    try {
      const parsedUrl = parseGitUrl(url);
      if (!parsedUrl) {
        setErrorAlert({
          open: true,
          title: "Invalid URL",
          description: `Unknown repository URL format: ${url}`
        });
        return;
      }

      const { org, repo } = parsedUrl;
      const response = await fetch(`/v1/git/checkout/${org}/${repo}`, {
        method: "POST",
      });
      
      if (response.ok) {
        refreshAll();
        const message = parsedUrl.type === "local" 
          ? "Local repository checked out successfully"
          : "Repository checked out successfully";
        toast.success(message);
      } else {
        const errorData = await response.json();
        console.error("Failed to checkout repository:", errorData);
        setErrorAlert({
          open: true,
          title: "Checkout Failed",
          description: `Failed to checkout repository: ${errorData.error || 'Unknown error'}`
        });
      }
    } catch (error) {
      console.error("Failed to checkout repository:", error);
      setErrorAlert({
        open: true,
        title: "Checkout Failed",
        description: `Failed to checkout repository: ${error}`
      });
    } finally {
      setLoading(false);
    }
  };

  const deleteWorktree = async (id: string) => {
    try {
      await gitApi.deleteWorktree(id);
      fetchWorktrees();
      fetchActiveSessions();
    } catch (error) {
      console.error("Failed to delete worktree:", error);
    }
  };



  const syncWorktree = async (id: string) => {
    const success = await gitApi.syncWorktree(id, { setErrorAlert });
    if (success) {
      fetchWorktrees();
    }
  };

  const mergeWorktreeToMain = async (id: string, worktreeName: string, squash: boolean = true) => {
    const success = await gitApi.mergeWorktree(id, worktreeName, squash, { setErrorAlert });
    if (success) {
      fetchWorktrees();
      fetchGitStatus();
    }
  };

  const createWorktreePreview = async (id: string, branchName: string) => {
    const success = await gitApi.createWorktreePreview(id, { setErrorAlert });
    if (success) {
      showPreviewToast(branchName);
    }
  };

  const createPullRequest = async () => {
    setPrLoading(true);
    try {
      const isUpdate = prDialog.isUpdate;
      const method = isUpdate ? "PUT" : "POST";
      const response = await fetch(`/v1/git/worktrees/${prDialog.worktreeId}/pr`, {
        method,
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          title: prDialog.title,
          body: prDialog.description,
        }),
      });
      if (response.ok) {
        const prData = await response.json();
        
        // Success toast with PR link
        toast.success(
          <div className="flex items-center gap-2 w-full">
            <div className="flex-1">
              <div className="font-medium">Pull request {isUpdate ? 'updated' : 'created'}!</div>
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
          }
        );
        
        // Refresh PR statuses after successful operation
        void fetchPRStatuses();
        
        // Close the dialog after successful creation/update
        setPrDialog({
          open: false,
          worktreeId: "",
          branchName: "",
          title: "",
          description: "",
          isUpdate: false,
        });
      } else {
        let errorMessage = 'Unknown error';
        try {
          const errorData = await response.json();
          errorMessage = errorData.error ?? 'Unknown error';
        } catch {
          // If JSON parsing fails, use status text or response text
          errorMessage = response.statusText || `HTTP ${response.status}`;
        }
        setErrorAlert({
          open: true,
          title: "Pull Request Failed",
          description: `Failed to ${isUpdate ? 'update' : 'create'} pull request: ${errorMessage}`
        });
      }
    } catch (error) {
      console.error(`Failed to ${prDialog.isUpdate ? 'update' : 'create'} pull request:`, error);
      setErrorAlert({
        open: true,
        title: "Pull Request Failed",
        description: `Failed to ${prDialog.isUpdate ? 'update' : 'create'} pull request: ${error}`
      });
    } finally {
      setPrLoading(false);
    }
  };

  const toggleDiff = (worktreeId: string) => {
    setOpenDiffWorktreeId(prev => prev === worktreeId ? null : worktreeId);
  };


  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      <div className="flex items-center gap-2 mb-6">
        <GitBranch size={24} />
        <h1 className="text-3xl font-bold">Source Code Management</h1>
      </div>

      {/* Worktrees Table */}
      <Card>
        <CardHeader>
          <CardTitle>Worktrees</CardTitle>
          <CardDescription>
            Active worktrees and their branch relationships
          </CardDescription>
        </CardHeader>
        <CardContent>
          {worktrees.length > 0 ? (
            <div className="space-y-2">
              {worktrees.map((worktree) => (
                <WorktreeRow
                  key={worktree.id}
                  worktree={worktree}
                  claudeSessions={claudeSessions}
                  syncConflicts={syncConflicts}
                  mergeConflicts={mergeConflicts}
                  worktreeSummaries={worktreeSummaries}
                  diffStats={diffStats}
                  prStatuses={prStatuses}
                  openDiffWorktreeId={openDiffWorktreeId}
                  setPrDialog={setPrDialog}
                  onToggleDiff={toggleDiff}
                  onSync={syncWorktree}
                  onMerge={(id, name) => {
                    setConfirmDialog({
                      open: true,
                      title: "Merge to Main",
                      description: mergeConflicts[id]?.has_conflicts
                        ? `⚠️ Warning: This merge will cause conflicts in ${mergeConflicts[id]?.conflict_files?.join(", ") || "multiple files"}. Merge ${worktree.commit_count} commits from "${name}" back to the ${worktree.source_branch} branch anyway?`
                        : `Merge ${worktree.commit_count} commits from "${name}" back to the ${worktree.source_branch} branch? This will make your changes available outside the container.`,
                      onConfirm: () => mergeWorktreeToMain(id, name),
                      variant: mergeConflicts[id]?.has_conflicts ? "destructive" : "default",
                    });
                  }}
                  onCreatePreview={createWorktreePreview}
                  onConfirmDelete={(id, name, isDirty, commitCount) => {
                    const changesList = [];
                    if (isDirty) changesList.push("uncommitted changes");
                    if (commitCount > 0) changesList.push(`${commitCount} commits`);
                    
                    setConfirmDialog({
                      open: true,
                      title: "Delete Worktree",
                      description: `Delete worktree "${name}"? This worktree has ${changesList.join(" and ")}. This action cannot be undone.`,
                      onConfirm: () => deleteWorktree(id),
                      variant: "destructive",
                    });
                  }}
                  onRegenerateSummary={generateWorktreeSummaryForId}
                />
              ))}
            </div>
          ) : (
            <p className="text-muted-foreground">No worktrees found</p>
          )}
        </CardContent>
      </Card>

      {/* GitHub URL Input */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Checkout Repository</CardTitle>
              <CardDescription>
                Select from your repositories or enter a GitHub URL
              </CardDescription>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={fetchRepositories}
              disabled={reposLoading}
              title="Refresh GitHub repositories"
            >
              <RefreshCw
                className={`h-4 w-4 ${reposLoading ? "animate-spin" : ""}`}
              />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-2">
            <div className="flex-1 space-y-2">
              <Label htmlFor="github-url">GitHub Repository URL</Label>
              <RepoSelector
                value={githubUrl}
                onValueChange={setGithubUrl}
                repositories={repositories}
                currentRepositories={gitStatus.repositories || {}}
                loading={reposLoading}
                placeholder="Select repository or enter URL..."
              />
            </div>
            <Button
              onClick={() => handleCheckout(githubUrl)}
              disabled={!githubUrl || loading}
              className="mt-6"
            >
              {loading ? (
                <RefreshCw className="animate-spin" size={16} />
              ) : (
                "Checkout"
              )}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Current Status */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Current Repository Status</CardTitle>
              <CardDescription>
                Active repository and worktree information
              </CardDescription>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={refreshAll}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {gitStatus.repositories &&
          Object.keys(gitStatus.repositories).length > 0 ? (
            <div className="space-y-4">
              {Object.values(gitStatus.repositories).map((repo: any) => (
                <div key={repo.id} className="space-y-2">
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold text-base">{repo.id}</h3>
                    {repoBranches[repo.id] &&
                      repoBranches[repo.id].length > 0 && (
                        <>
                          {(() => {
                            // For local repos, only show branches that have worktrees
                            let branchesToShow = repoBranches[repo.id];
                            if (repo.id.startsWith("local/")) {
                              const worktreeBranches = worktrees
                                .filter(wt => wt.repo_id === repo.id)
                                .map(wt => wt.source_branch);
                              branchesToShow = repoBranches[repo.id].filter(branch => 
                                worktreeBranches.includes(branch)
                              );
                            }
                            
                            return branchesToShow.map((branch) => (
                              <Badge
                                key={branch}
                                variant="secondary"
                                className="text-xs cursor-pointer hover:bg-secondary/80"
                                onClick={() => {
                                  if (!repo.id.startsWith("local/")) {
                                    window.open(
                                      `${repo.url}/tree/${branch}`,
                                      "_blank"
                                    )
                                  }
                                }}
                              >
                                {branch}
                              </Badge>
                            ));
                          })()}
                        </>
                      )}
                  </div>
                  {!repo.id.startsWith("local/") && (
                    <div className="mt-2">
                      <div className="inline-flex items-center gap-2 p-2 bg-muted rounded text-sm font-mono">
                        <code className="text-muted-foreground">
                          git remote add catnip {window.location.origin}/
                          {repo.id.split("/")[1]}.git
                        </code>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            const url = `${window.location.origin}/${
                              repo.id.split("/")[1]
                            }.git`;
                            copyRemoteCommand(url);
                          }}
                          className="h-6 w-6 p-0"
                        >
                          <Copy size={12} />
                        </Button>
                      </div>
                    </div>
                  )}
                </div>
              ))}
              <div className="border-t pt-2">
                <p className="text-xs text-muted-foreground">
                  Total repositories:{" "}
                  {Object.keys(gitStatus.repositories).length} | Total
                  worktrees: {gitStatus.worktree_count || 0}
                </p>
              </div>
            </div>
          ) : (
            <p className="text-muted-foreground">No repositories checked out</p>
          )}
        </CardContent>
      </Card>

      {/* Confirmation Dialog */}
      <ConfirmDialog
        open={confirmDialog.open}
        onOpenChange={(open) => setConfirmDialog(prev => ({ ...prev, open }))}
        title={confirmDialog.title}
        description={confirmDialog.description}
        onConfirm={confirmDialog.onConfirm}
        variant={confirmDialog.variant}
        confirmText={confirmDialog.variant === "destructive" ? "Delete" : "Continue"}
      />

      {/* Error Alert */}
      <ErrorAlert
        open={errorAlert.open}
        onOpenChange={(open) => setErrorAlert(prev => ({ ...prev, open }))}
        title={errorAlert.title}
        description={errorAlert.description}
      />

      {/* Pull Request Dialog */}
      <Dialog open={prDialog.open} onOpenChange={(open) => setPrDialog(prev => ({ ...prev, open }))}>
        <DialogContent className="max-w-4xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{prDialog.isUpdate ? 'Update Pull Request' : 'Create Pull Request'}</DialogTitle>
            <DialogDescription>
              {prDialog.isUpdate ? 'Update the pull request' : 'Create a pull request'} for the worktree {prDialog.branchName}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-6 py-4">
            <div className="grid gap-2">
              <Label htmlFor="pr-title" className="text-sm font-medium">
                Title
              </Label>
              <Input
                id="pr-title"
                value={prDialog.title}
                onChange={(e) =>
                  setPrDialog((prev) => ({ ...prev, title: e.target.value }))
                }
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
                value={prDialog.description}
                onChange={(e) =>
                  setPrDialog((prev) => ({ ...prev, description: e.target.value }))
                }
                className="w-full min-h-[300px] rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 md:text-sm resize-vertical"
                placeholder="Enter pull request description..."
              />
            </div>
          </div>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setPrDialog({ ...prDialog, open: false })}>
              Cancel
            </Button>
            <Button onClick={createPullRequest} disabled={prLoading}>
              {prLoading ? (
                <>
                  <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                  {prDialog.isUpdate ? 'Updating PR...' : 'Creating PR...'}
                </>
              ) : (
                prDialog.isUpdate ? 'Update PR' : 'Create PR'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export const Route = createFileRoute("/git")({
  component: GitPage,
});
