import { createFileRoute, useLocation } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { BranchSelector } from "@/components/BranchSelector";
import { RepoSelector } from "@/components/RepoSelector";
import { ErrorAlert } from "@/components/ErrorAlert";
import { GitBranch, Copy, Loader2 } from "lucide-react";
import { copyRemoteCommand } from "@/lib/git-utils";
import { type LocalRepository, gitApi } from "@/lib/git-api";
import { useHighlight } from "@/hooks/useHighlight";
import { useAppStore } from "@/stores/appStore";
import { useGitApi } from "@/hooks/useGitApi";

function GitPage() {
  const location = useLocation();
  const fromWorkspace = (location.state as any)?.fromWorkspace === true;
  const { highlightClassName } = useHighlight(fromWorkspace);

  // Use the new centralized hooks
  const { getWorktreesList, getRepositoriesList, gitStatus, initialLoading } =
    useAppStore();

  const {
    checkoutLoading,
    syncingWorktrees,
    mergingWorktrees,
    deleteWorktree,
    syncWorktree,
    mergeWorktree,
  } = useGitApi();

  // Convert to the expected format for compatibility
  const worktrees = getWorktreesList();
  const repositories = getRepositoriesList();
  const loading = initialLoading;

  const [githubUrl, setGithubUrl] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("");
  const [selectedRepoBranches, setSelectedRepoBranches] = useState<string[]>(
    [],
  );
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [showDirtyOnly, setShowDirtyOnly] = useState(false);
  const [autoCleanup] = useState(true);
  const [githubRepos, setGithubRepos] = useState<any[]>([]);
  const [currentGithubRepos, setCurrentGithubRepos] = useState<
    Record<string, LocalRepository>
  >({});

  // Error state
  const [errorAlert, setErrorAlert] = useState({
    open: false,
    title: "",
    description: "",
  });

  const errorHandler = {
    setErrorAlert: (alert: {
      open: boolean;
      title: string;
      description: string;
      worktreeName?: string;
      conflictFiles?: string[];
      operation?: string;
    }) => {
      setErrorAlert({
        open: alert.open,
        title: alert.title,
        description: alert.description,
      });
    },
  };

  // TODO: Implement proper checkout functionality
  const handleCheckout = async (url: string, branch?: string) => {
    // This needs to be properly implemented
    console.log("Checkout functionality needs to be implemented", url, branch);
    return false;
  };

  // Handle repo selection change - fetch branches for the selected repo
  const handleRepoChange = async (url: string) => {
    setGithubUrl(url);
    setSelectedBranch("");
    setSelectedRepoBranches([]);

    if (!url) return;

    // Check if this is a current repository (already checked out)
    const repositories = gitStatus.repositories as
      | Record<string, LocalRepository>
      | undefined;
    const currentRepo = Object.values(repositories ?? {}).find(
      (repo: LocalRepository) =>
        (repo.id.startsWith("local/") ? repo.id : repo.url) === url,
    );

    if (currentRepo) {
      // For current repos, get the current branch and default branch
      setBranchesLoading(true);
      try {
        const branches = await gitApi.fetchBranches(currentRepo.id);
        setSelectedRepoBranches(branches);

        // Set default branch as selected for current repos
        if (
          currentRepo.default_branch &&
          branches.includes(currentRepo.default_branch)
        ) {
          setSelectedBranch(currentRepo.default_branch);
        } else if (branches.length > 0) {
          setSelectedBranch(branches[0]);
        }
      } catch (error) {
        console.error("Failed to fetch branches:", error);
      } finally {
        setBranchesLoading(false);
      }
    }
  };

  // Initial load
  useEffect(() => {
    const loadData = async () => {
      // Initial data loading is handled by the store
    };
    void loadData();
  }, []);

  // Fetch GitHub repositories on mount
  useEffect(() => {
    const fetchGithubRepos = async () => {
      try {
        // Fetch GitHub repos
        const repos = await gitApi.fetchRepositories();
        setGithubRepos(repos);

        // repositories available from getRepositoriesList();
        setCurrentGithubRepos(
          repositories.reduce(
            (acc, repo) => {
              acc[repo.id] = repo;
              return acc;
            },
            {} as Record<string, LocalRepository>,
          ),
        );

        // fetchRepositories replaced by automatic store loading
      } catch (error) {
        console.error("Failed to load repositories:", error);
      }
    };

    void fetchGithubRepos();
  }, [repositories]);

  // TODO: Fetch branches for repositories
  const getBranchesForRepo = (_repoId: string): string[] => {
    // This functionality needs to be re-implemented
    return [];
  };

  // Filter worktrees
  const filteredWorktrees = showDirtyOnly
    ? worktrees.filter((wt: any) => wt.is_dirty)
    : worktrees;

  // Force refresh button
  const handleRefresh = async () => {
    // refreshAll replaced by automatic store loading
  };

  return (
    <div className="container mx-auto p-4 space-y-6">
      {/* Error Alert */}
      <ErrorAlert
        open={errorAlert.open}
        onOpenChange={(open) => setErrorAlert({ ...errorAlert, open })}
        title={errorAlert.title}
        description={errorAlert.description}
      />

      {/* Header with Refresh */}
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Git Workspaces</h1>
        <Button onClick={handleRefresh} variant="outline" size="sm">
          <Loader2 className="w-4 h-4 mr-2" />
          Refresh
        </Button>
      </div>

      {/* Main Content */}
      {loading ? (
        <div className="flex justify-center">
          <Loader2 className="animate-spin" />
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Left Column: Repository Selection */}
          <div className="space-y-4">
            <Card className={highlightClassName}>
              <CardHeader>
                <CardTitle>Create New Workspace</CardTitle>
                <CardDescription>
                  Select a repository and branch to create a new workspace
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <RepoSelector
                  value={githubUrl}
                  onValueChange={handleRepoChange}
                  repositories={githubRepos}
                  currentRepositories={currentGithubRepos}
                  loading={false}
                />

                <BranchSelector
                  value={selectedBranch}
                  onValueChange={setSelectedBranch}
                  branches={selectedRepoBranches}
                  currentBranch={(() => {
                    const repositories = gitStatus.repositories as
                      | Record<string, LocalRepository>
                      | undefined;
                    const currentRepo = Object.values(repositories ?? {}).find(
                      (repo: LocalRepository) =>
                        (repo.id.startsWith("local/") ? repo.id : repo.url) ===
                        githubUrl,
                    );
                    if (currentRepo?.id.startsWith("local/")) {
                      // For local repos, get the current branch from worktrees
                      const repoWorktrees = worktrees.filter(
                        (wt: any) => wt.repo_id === currentRepo.id,
                      );
                      return repoWorktrees.length > 0
                        ? repoWorktrees[0].source_branch
                        : undefined;
                    }
                    return currentRepo?.default_branch;
                  })()}
                  defaultBranch={(() => {
                    const repositories = gitStatus.repositories as
                      | Record<string, LocalRepository>
                      | undefined;
                    const currentRepo = Object.values(repositories ?? {}).find(
                      (repo: LocalRepository) =>
                        (repo.id.startsWith("local/") ? repo.id : repo.url) ===
                        githubUrl,
                    );
                    return currentRepo?.default_branch;
                  })()}
                  disabled={false}
                  loading={branchesLoading}
                />

                <Button
                  onClick={() => handleCheckout(githubUrl, selectedBranch)}
                  disabled={!githubUrl || !selectedBranch || checkoutLoading}
                  className="w-full"
                >
                  {checkoutLoading ? (
                    <Loader2 className="animate-spin mr-2 h-4 w-4" />
                  ) : null}
                  Create Workspace
                </Button>
              </CardContent>
            </Card>
          </div>

          {/* Right Column: Workspace List */}
          <div className="space-y-4">
            <Card>
              <CardHeader>
                <div className="flex justify-between items-center">
                  <div>
                    <CardTitle>Active Workspaces</CardTitle>
                    <CardDescription>
                      {filteredWorktrees.length} workspace
                      {filteredWorktrees.length !== 1 ? "s" : ""}
                    </CardDescription>
                  </div>
                  <div className="flex items-center space-x-2">
                    <input
                      type="checkbox"
                      checked={showDirtyOnly}
                      onChange={(e) => setShowDirtyOnly(e.target.checked)}
                      className="rounded"
                    />
                    <label className="text-sm">Dirty only</label>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                {filteredWorktrees.length === 0 ? (
                  <p className="text-muted-foreground">No workspaces found</p>
                ) : (
                  <div className="space-y-2">
                    {filteredWorktrees
                      .sort((a: any, b: any) => {
                        // Sort by last accessed descending
                        return (
                          new Date(b.last_accessed).getTime() -
                          new Date(a.last_accessed).getTime()
                        );
                      })
                      .map((worktree: any) => (
                        <div
                          key={worktree.id}
                          className="border rounded-lg p-4"
                        >
                          <div className="flex items-center justify-between">
                            <div>
                              <h3 className="font-semibold">{worktree.name}</h3>
                              <p className="text-sm text-muted-foreground">
                                {worktree.branch} •{" "}
                                {worktree.commit_hash.slice(0, 8)}
                              </p>
                              {worktree.is_dirty && (
                                <Badge variant="secondary" className="mt-1">
                                  Dirty
                                </Badge>
                              )}
                            </div>
                            <div className="flex gap-2">
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() =>
                                  syncWorktree(worktree.id, errorHandler)
                                }
                                disabled={syncingWorktrees.has(worktree.id)}
                              >
                                {syncingWorktrees.has(worktree.id) && (
                                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                                )}
                                Sync
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() =>
                                  mergeWorktree(
                                    worktree.id,
                                    worktree.name,
                                    false,
                                    errorHandler,
                                    autoCleanup,
                                  )
                                }
                                disabled={mergingWorktrees.has(worktree.id)}
                              >
                                {mergingWorktrees.has(worktree.id) && (
                                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                                )}
                                Merge
                              </Button>
                              <Button
                                variant="destructive"
                                size="sm"
                                onClick={() => deleteWorktree(worktree.id)}
                              >
                                Delete
                              </Button>
                            </div>
                          </div>
                        </div>
                      ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </div>
      )}

      {/* Local Repositories Section */}
      {gitStatus.repositories &&
      Object.keys(gitStatus.repositories).length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>Local Repositories</CardTitle>
          </CardHeader>
          <CardContent>
            {Object.values(
              gitStatus.repositories as Record<string, LocalRepository>,
            ).map((repo: LocalRepository) => (
              <div key={repo.id} className="space-y-2">
                <div className="flex justify-between items-center">
                  <h3 className="font-semibold text-base">{repo.id}</h3>
                  {getBranchesForRepo(repo.id).length > 0 && (
                    <div className="text-sm text-muted-foreground mb-2">
                      <span>Branches: </span>
                      {(() => {
                        const allBranches = getBranchesForRepo(repo.id);
                        const displayBranches = allBranches
                          .filter((b: string) => !b.startsWith("origin/HEAD"))
                          .sort((a: string, b: string) => {
                            if (repo.id.startsWith("local/")) {
                              const repoWorktrees = worktrees.filter(
                                (wt: any) => wt.repo_id === repo.id,
                              );
                              const currentBranch = repoWorktrees.find(
                                (wt: any) => wt.commits_behind === 0,
                              )?.source_branch
                                ? repoWorktrees.find(
                                    (wt: any) => wt.commits_behind === 0,
                                  )?.source_branch
                                : repo.default_branch || allBranches?.[0];

                              if (a === currentBranch) return -1;
                              if (b === currentBranch) return 1;
                            }
                            return (
                              (a === (repo.default_branch || allBranches?.[0])
                                ? -1
                                : 1) -
                              (b === (repo.default_branch || allBranches?.[0])
                                ? -1
                                : 1)
                            );
                          })
                          .slice(0, 5);

                        return displayBranches.join(", ");
                      })()}
                    </div>
                  )}
                </div>

                {repo.id.startsWith("local/") && (
                  <div className="space-y-1">
                    <p className="text-sm text-muted-foreground">
                      Local repository: {repo.id.split("/")[1]}.git
                    </p>
                    <Button
                      size="sm"
                      onClick={() => copyRemoteCommand(repo.path || "")}
                    >
                      <Copy className="h-4 w-4 mr-1" />
                      Copy Remote Command
                    </Button>
                  </div>
                )}

                <div className="pl-4 space-y-1">
                  {(() => {
                    const repoWorktrees = worktrees.filter(
                      (wt: any) => wt.repo_id === repo.id,
                    );
                    const sortedWorktrees = repoWorktrees.sort(
                      (a: any, b: any) => {
                        // Sort by last accessed descending
                        return (
                          new Date(b.last_accessed).getTime() -
                          new Date(a.last_accessed).getTime()
                        );
                      },
                    );

                    return sortedWorktrees.map((worktree: any) => (
                      <div
                        key={worktree.id}
                        className="flex items-center space-x-2 text-sm"
                      >
                        <GitBranch className="h-4 w-4" />
                        <span>{worktree.branch}</span>
                        {worktree.is_dirty && (
                          <Badge variant="secondary">dirty</Badge>
                        )}
                        {worktree.has_conflicts && (
                          <Badge variant="destructive">conflicts</Badge>
                        )}
                      </div>
                    ));
                  })()}
                </div>
              </div>
            ))}
            <div className="text-sm text-muted-foreground text-center">
              <span>
                {
                  Object.keys(
                    gitStatus.repositories as Record<string, LocalRepository>,
                  ).length
                }{" "}
                | Total Repositories
              </span>
            </div>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardContent className="py-8">
            <p className="text-center text-muted-foreground">
              No local repositories found.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export const Route = createFileRoute("/git")({
  component: GitPage,
});
