import {
  createFileRoute,
  useLocation,
  useNavigate,
} from "@tanstack/react-router";
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
import { RepoSelector } from "@/components/RepoSelector";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorAlert } from "@/components/ErrorAlert";
import { WorktreeRow } from "@/components/WorktreeRow";
import { PullRequestDialog } from "@/components/PullRequestDialog";
import { GitBranch, Copy, RefreshCw, Loader2 } from "lucide-react";
import { copyRemoteCommand } from "@/lib/git-utils";
import { type LocalRepository } from "@/lib/git-api";
import { useGitState } from "@/hooks/useGitState";
import { useGitActions } from "@/hooks/useGitActions";
import { useHighlight } from "@/hooks/useHighlight";

function GitPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const fromWorkspace = (location.state as any)?.fromWorkspace === true;
  const { highlightClassName } = useHighlight(fromWorkspace);

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
    diffStatsLoading,
    prStatuses,
    loading,
    reposLoading,
    checkoutLoading,
    syncingWorktrees,
    mergingWorktrees,
    loadingNewWorktrees,
    fetchRepositories,
    fetchPrStatuses,
    refreshAll,
    // Functions needed by useGitActions
    addNewWorktrees,
    backgroundRefreshGitStatus,
    refreshWorktree,
    removeWorktree,
    fetchActiveSessions,
    setCheckoutLoading,
    setSyncingWorktree,
    setMergingWorktree,
  } = useGitState();

  const [githubUrl, setGithubUrl] = useState("");
  const [openDiffWorktreeId, setOpenDiffWorktreeId] = useState<string | null>(
    null,
  );
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
    onConfirm: () => undefined,
  });
  const [errorAlert, setErrorAlert] = useState<{
    open: boolean;
    title: string;
    description: string;
    secondaryAction?: {
      label: string;
      onClick: () => void;
      variant?: "default" | "destructive";
    };
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

  // Enhanced error alert handler that can include Claude assistance for conflicts
  const setErrorAlertWithClaudeAction = (alert: {
    open: boolean;
    title: string;
    description: string;
    worktreeName?: string;
    conflictFiles?: string[];
    operation?: string;
  }) => {
    if (alert.worktreeName && alert.conflictFiles && alert.operation) {
      // This is a conflict - add Claude assistance
      const sessionId = encodeURIComponent(alert.worktreeName);
      const conflictText =
        alert.conflictFiles.length > 0
          ? `Conflicts detected in: ${alert.conflictFiles.join(", ")}`
          : "Multiple files have conflicts";

      setErrorAlert({
        open: alert.open,
        title: alert.title,
        description: `${conflictText}\n\nDon't worry! Claude can help resolve these conflicts automatically.`,
        secondaryAction: {
          label: "Let Claude Help",
          onClick: () => {
            void navigate({
              to: "/terminal/$sessionId",
              params: { sessionId },
              search: {
                agent: "claude",
                prompt: `Please help me resolve these conflicts and complete the ${alert.operation}. Ask me for confirmation before completing the ${alert.operation}.`,
              },
            });
          },
        },
      });
    } else {
      // Regular error
      setErrorAlert({
        open: alert.open,
        title: alert.title,
        description: alert.description,
      });
    }
  };

  const {
    checkoutRepository,
    deleteWorktree,
    syncWorktree,
    mergeWorktreeToMain,
    createWorktreePreview,
  } = useGitActions({
    addNewWorktrees,
    backgroundRefreshGitStatus,
    refreshWorktree,
    removeWorktree,
    fetchActiveSessions,
    setCheckoutLoading,
    setSyncingWorktree,
    setMergingWorktree,
  });

  const handleCheckout = async (url: string) => {
    const success = await checkoutRepository(
      url,
      setErrorAlertWithClaudeAction,
    );
    if (success) {
      setGithubUrl("");
    }
  };

  const syncWorktreeWrapper = async (id: string) => {
    void syncWorktree(id, setErrorAlertWithClaudeAction);
  };

  const mergeWorktreeWrapper = async (
    id: string,
    name: string,
    squash = true,
  ) => {
    void mergeWorktreeToMain(id, name, setErrorAlertWithClaudeAction, squash);
  };

  const createPreviewWrapper = async (id: string, branchName: string) => {
    void createWorktreePreview(id, branchName, setErrorAlertWithClaudeAction);
  };

  const toggleDiff = (worktreeId: string) => {
    setOpenDiffWorktreeId((prev) => (prev === worktreeId ? null : worktreeId));
  };

  const onMerge = (id: string, name: string) => {
    const hasConflicts = mergeConflicts[id]?.has_conflicts ?? false;
    const conflictFilesString =
      mergeConflicts[id]?.conflict_files?.join(", ") ??
      `${mergeConflicts[id]?.conflict_files?.length} files`;
    const worktree = worktrees.find((wt) => wt.id === id);
    const commitCount = worktree?.commit_count ?? 0;
    const sourceBranch = worktree?.source_branch ?? "";
    const description = `
      Merge ${commitCount} commits from "${name}" back to the ${sourceBranch} branch? This will make your changes available outside the container.
      ${hasConflicts ? `⚠️ Warning: This merge will cause conflicts in ${conflictFilesString}. Merge ${commitCount} commits from "${name}" back to the ${sourceBranch} branch anyway?` : ""}
    `;
    setConfirmDialog({
      open: true,
      title: "Merge to Main",
      description: description,
      onConfirm: () => void mergeWorktreeWrapper(id, name),
      variant: hasConflicts ? "destructive" : "default",
    });
  };

  const onConfirmDelete = (
    id: string,
    name: string,
    isDirty: boolean,
    commitCount: number,
  ) => {
    const changesList = [];
    if (isDirty) changesList.push("uncommitted changes");
    if (commitCount > 0) changesList.push(`${commitCount} commits`);

    setConfirmDialog({
      open: true,
      title: "Delete Worktree",
      description: `Delete worktree "${name}"? This worktree has ${changesList.join(" and ")}. This action cannot be undone.`,
      onConfirm: () => void deleteWorktree(id),
      variant: "destructive",
    });
  };

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      <div className="flex items-center gap-2 mb-6">
        <GitBranch size={24} />
        <h1 className="text-3xl font-bold">Source Code Management</h1>
      </div>

      {/* GitHub URL Input */}
      <Card className={highlightClassName}>
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
              onClick={void fetchRepositories}
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
                currentRepositories={gitStatus.repositories ?? {}}
                loading={reposLoading}
                placeholder="Select repository or enter URL..."
                autoExpand={fromWorkspace}
              />
            </div>
            <Button
              onClick={() => void handleCheckout(githubUrl)}
              disabled={!githubUrl || checkoutLoading}
              className="mt-5.5"
            >
              {checkoutLoading ? (
                <RefreshCw className="animate-spin" size={16} />
              ) : (
                "Checkout"
              )}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Worktrees Table */}
      <Card>
        <CardHeader>
          <CardTitle>Worktrees</CardTitle>
          <CardDescription>
            Active worktrees and their branch relationships
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex justify-center items-center h-full">
              <Loader2 className="animate-spin" />
            </div>
          ) : (
            <>
              {worktrees.length > 0 ? (
                <div className="space-y-2">
                  {worktrees
                    .sort(
                      (a, b) =>
                        new Date(a.created_at).getTime() -
                        new Date(b.created_at).getTime(),
                    )
                    .map((worktree) => (
                      <WorktreeRow
                        key={worktree.id}
                        worktree={worktree}
                        claudeSessions={claudeSessions}
                        _syncConflicts={syncConflicts}
                        _mergeConflicts={mergeConflicts}
                        worktreeSummaries={worktreeSummaries}
                        diffStats={diffStats}
                        diffStatsLoading={diffStatsLoading}
                        openDiffWorktreeId={openDiffWorktreeId}
                        setPrDialog={setPrDialog}
                        onToggleDiff={toggleDiff}
                        onSync={() => void syncWorktreeWrapper(worktree.id)}
                        onMerge={onMerge}
                        onCreatePreview={() =>
                          void createPreviewWrapper(
                            worktree.id,
                            worktree.branch,
                          )
                        }
                        prStatuses={prStatuses}
                        onConfirmDelete={onConfirmDelete}
                        repositories={gitStatus.repositories}
                        // Pass individual loading states
                        isSyncing={syncingWorktrees.has(worktree.id)}
                        isMerging={mergingWorktrees.has(worktree.id)}
                      />
                    ))}
                  {loadingNewWorktrees && (
                    <div className="border rounded-lg p-4 mb-4 bg-card animate-pulse">
                      <div className="flex items-center justify-between">
                        <div className="flex-1">
                          <div className="flex items-center gap-3">
                            <div className="flex items-center gap-2">
                              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current"></div>
                              <span className="text-sm text-muted-foreground">
                                Loading new worktree...
                              </span>
                            </div>
                          </div>
                          <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
                            <div className="h-3 w-16 bg-muted rounded"></div>
                            <div className="h-3 w-20 bg-muted rounded"></div>
                          </div>
                          <div className="flex items-center gap-4 mt-2">
                            <div className="h-3 w-32 bg-muted rounded"></div>
                          </div>
                        </div>
                        <div className="flex items-center gap-2 pb-10">
                          <div className="h-8 w-24 bg-muted rounded"></div>
                          <div className="h-8 w-16 bg-muted rounded"></div>
                          <div className="h-8 w-8 bg-muted rounded"></div>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              ) : (
                <p className="text-muted-foreground">No worktrees found</p>
              )}
            </>
          )}
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
              onClick={() => void refreshAll()}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {gitStatus.repositories &&
          Object.keys(gitStatus.repositories).length > 0 ? (
            <div className="space-y-4">
              {Object.values(gitStatus.repositories).map(
                (repo: LocalRepository) => (
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
                                  .filter((wt) => wt.repo_id === repo.id)
                                  .map((wt) => wt.source_branch);
                                branchesToShow = repoBranches[repo.id].filter(
                                  (branch) => worktreeBranches.includes(branch),
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
                                        "_blank",
                                      );
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
                ),
              )}
              <div className="border-t pt-2">
                <p className="text-xs text-muted-foreground">
                  Total repositories:{" "}
                  {Object.keys(gitStatus.repositories).length} | Total
                  worktrees: {gitStatus.worktree_count ?? 0}
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
        onOpenChange={(open) => setConfirmDialog((prev) => ({ ...prev, open }))}
        title={confirmDialog.title}
        description={confirmDialog.description}
        onConfirm={confirmDialog.onConfirm}
        variant={confirmDialog.variant}
        confirmText={
          confirmDialog.variant === "destructive" ? "Delete" : "Continue"
        }
      />

      {/* Error Alert */}
      <ErrorAlert
        open={errorAlert.open}
        onOpenChange={(open) => setErrorAlert((prev) => ({ ...prev, open }))}
        title={errorAlert.title}
        description={errorAlert.description}
        secondaryAction={errorAlert.secondaryAction}
      />

      {/* Pull Request Dialog */}
      <PullRequestDialog
        open={prDialog.open}
        onOpenChange={(open) => setPrDialog((prev) => ({ ...prev, open }))}
        worktreeId={prDialog.worktreeId}
        branchName={prDialog.branchName}
        title={prDialog.title}
        description={prDialog.description}
        isUpdate={prDialog.isUpdate}
        onTitleChange={(title) => setPrDialog((prev) => ({ ...prev, title }))}
        onDescriptionChange={(description) =>
          setPrDialog((prev) => ({ ...prev, description }))
        }
        onRefreshPrStatuses={fetchPrStatuses}
      />
    </div>
  );
}

export const Route = createFileRoute("/git")({
  component: GitPage,
});
