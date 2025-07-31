import { createFileRoute, useLocation, Link } from "@tanstack/react-router";
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
import { WorkspaceActions } from "@/components/WorkspaceActions";
import { DiffViewer } from "@/components/DiffViewer";
import {
  GitBranch,
  Copy,
  Eye,
  RotateCcw,
  Loader2,
  GitPullRequest,
  ExternalLink,
  ChevronDown,
  Check,
} from "lucide-react";
// Utility imports removed - not used in current implementation
import { type LocalRepository, gitApi } from "@/lib/git-api";
import { useHighlight } from "@/hooks/useHighlight";
import { useAppStore } from "@/stores/appStore";
import { useGitApi } from "@/hooks/useGitApi";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

// Session title components
interface CommitHashDisplayProps {
  commitHash: string;
  pullRequestUrl?: string;
}

function CommitHashDisplay({
  commitHash,
  pullRequestUrl,
}: CommitHashDisplayProps) {
  const [copiedHash, setCopiedHash] = useState<string | null>(null);

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedHash(text);
      setTimeout(() => setCopiedHash(null), 2000);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  };

  if (pullRequestUrl) {
    const prCommitUrl = `${pullRequestUrl}/commits/${commitHash}`;
    return (
      <a
        href={prCommitUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="font-mono text-xs text-muted-foreground hover:text-foreground hover:underline transition-colors inline-flex items-center gap-1 group"
      >
        {commitHash.slice(0, 7)}
        <svg
          className="w-3 h-3 opacity-0 group-hover:opacity-100 transition-opacity"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
          />
        </svg>
      </a>
    );
  }

  const isCopied = copiedHash === commitHash;
  return (
    <button
      onClick={() => copyToClipboard(commitHash)}
      className="font-mono text-xs text-muted-foreground hover:text-foreground hover:bg-muted/50 rounded px-1 py-0.5 transition-colors inline-flex items-center gap-1 group cursor-pointer"
      title={commitHash}
    >
      {commitHash.slice(0, 7)}
      {isCopied ? (
        <Check className="w-3 h-3 text-green-500 opacity-100 transition-opacity" />
      ) : (
        <Copy className="w-3 h-3 opacity-0 group-hover:opacity-100 transition-opacity" />
      )}
    </button>
  );
}

interface SessionHistoryItemProps {
  historyEntry: any;
  index: number;
  pullRequestUrl?: string;
}

function SessionHistoryItem({
  historyEntry,
  index,
  pullRequestUrl,
}: SessionHistoryItemProps) {
  return (
    <div className="px-2 py-1.5 text-sm">
      <div className="flex items-center justify-between w-full">
        <div className="flex flex-col min-w-0">
          <div className="flex items-center justify-between w-full">
            <span className="truncate font-medium">{historyEntry.title}</span>
            {historyEntry.commit_hash && (
              <span className="ml-2 shrink-0">
                <CommitHashDisplay
                  commitHash={historyEntry.commit_hash}
                  pullRequestUrl={pullRequestUrl}
                />
              </span>
            )}
          </div>
          <span className="text-xs text-muted-foreground">
            {new Date(historyEntry.timestamp).toLocaleString()}
          </span>
        </div>
        {index === 0 && (
          <Badge variant="secondary" className="ml-2 text-xs shrink-0">
            Current
          </Badge>
        )}
      </div>
    </div>
  );
}

interface SessionTitleProps {
  worktree: any;
  isActive: boolean;
  pullRequestUrl?: string;
}

function SessionTitle({
  worktree,
  isActive,
  pullRequestUrl,
}: SessionTitleProps) {
  const { session_title, session_title_history = [] } = worktree;

  if (
    !session_title &&
    (!session_title_history || session_title_history.length === 0)
  ) {
    return null;
  }

  const displayTitle =
    session_title?.title ||
    session_title_history[session_title_history.length - 1]?.title;
  if (!displayTitle) {
    return null;
  }

  return (
    <div className="mt-2 flex items-center gap-2">
      {isActive ? (
        <div
          className="w-2 h-2 bg-green-500 rounded-full animate-pulse"
          title="Active"
        />
      ) : (
        <div className="w-2 h-2 bg-gray-500 rounded-full" title="Inactive" />
      )}
      {session_title_history && session_title_history.length >= 1 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className="h-auto p-1 justify-start hover:bg-muted"
            >
              <div className="flex items-center gap-2">
                <span
                  className="text-sm font-medium text-foreground"
                  title={displayTitle}
                >
                  {displayTitle}
                </span>
                <ChevronDown size={12} className="text-muted-foreground" />
              </div>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            side="right"
            align="start"
            className="w-96 max-h-80 overflow-y-auto"
          >
            <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
              Session history
            </div>
            <DropdownMenuSeparator />
            {session_title_history
              .slice()
              .reverse()
              .map((historyEntry: any, index: number) => (
                <SessionHistoryItem
                  key={index}
                  historyEntry={historyEntry}
                  index={index}
                  pullRequestUrl={pullRequestUrl}
                />
              ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  );
}

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
    createWorktreePreview,
    checkoutRepository,
    // createPullRequest,
    // updatePullRequest,
    // getPullRequestInfo,
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

  // Track open diff states
  const [openDiffWorktreeId, setOpenDiffWorktreeId] = useState<string | null>(
    null,
  );

  // Diff loading state (DiffViewer handles its own loading)
  const [diffLoading, setDiffLoading] = useState(false);

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

  // Handle checkout functionality
  const handleCheckout = async (url: string, branch?: string) => {
    if (!url || !branch) return false;

    // Check if this is a local repository (starts with "local/")
    if (url.startsWith("local/")) {
      // For local repos, extract the repo name
      const repoName = url.split("/")[1];
      return await checkoutRepository("local", repoName, branch);
    } else {
      // For GitHub URLs, parse the org and repo name
      // Expected format: https://github.com/org/repo or git@github.com:org/repo.git
      let match = url.match(/github\.com[/:]([\w-]+)\/([\w-]+?)(\.git)?$/);
      if (!match) {
        // Try without protocol
        match = url.match(/^([\w-]+)\/([\w-]+)$/);
      }

      if (match) {
        const org = match[1];
        const repo = match[2];
        return await checkoutRepository(org, repo, branch);
      } else {
        console.error("Invalid GitHub URL format:", url);
        return false;
      }
    }
  };

  // Handle repo selection change - fetch branches for the selected repo
  const handleRepoChange = async (url: string) => {
    setGithubUrl(url);
    setSelectedBranch("");
    setSelectedRepoBranches([]);

    if (!url) return;

    setBranchesLoading(true);
    try {
      // Check if this is a current repository (already checked out)
      const repositories = gitStatus.repositories as
        | Record<string, LocalRepository>
        | undefined;
      const currentRepo = Object.values(repositories ?? {}).find(
        (repo: LocalRepository) =>
          (repo.id.startsWith("local/") ? repo.id : repo.url) === url,
      );

      let branches: string[] = [];
      let repoId: string;

      if (currentRepo) {
        // For checked out repos, use the repository ID
        repoId = currentRepo.id;
        branches = await gitApi.fetchBranches(repoId);

        // Set default branch as selected for current repos
        if (
          currentRepo.default_branch &&
          branches.includes(currentRepo.default_branch)
        ) {
          setSelectedBranch(currentRepo.default_branch);
        } else if (branches.length > 0) {
          setSelectedBranch(branches[0]);
        }
      } else {
        // For remote GitHub repos that haven't been checked out yet
        // Parse GitHub URL to get org/repo format
        let repoPath = "";
        if (url.startsWith("https://github.com/")) {
          // Extract org/repo from full GitHub URL
          const match = url.match(/github\.com\/([^/]+\/[^/]+)/);
          if (match) {
            repoPath = match[1].replace(/\.git$/, ""); // Remove .git suffix if present
          }
        } else if (url.includes("/") && !url.startsWith("local/")) {
          // Already in org/repo format
          repoPath = url;
        }

        if (repoPath) {
          repoId = repoPath;
          branches = await gitApi.fetchBranches(repoId);

          // For remote repos, set the first branch as default (usually main/master)
          if (branches.length > 0) {
            setSelectedBranch(branches[0]);
          }
        }
      }

      setSelectedRepoBranches(branches);
    } catch (error) {
      console.error("Failed to fetch branches:", error);
      setSelectedRepoBranches([]);
    } finally {
      setBranchesLoading(false);
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

  // Handle preview creation
  const handleCreatePreview = async (worktreeId: string, _branch: string) => {
    await createWorktreePreview(worktreeId, errorHandler);
  };

  // Toggle diff view
  const toggleDiff = (worktreeId: string) => {
    setOpenDiffWorktreeId((prev) => (prev === worktreeId ? null : worktreeId));
  };

  // Handle merge with confirmation
  const handleMerge = async (worktreeId: string, name: string) => {
    if (
      window.confirm(
        `Are you sure you want to merge worktree "${name}"? This will merge changes back to the source branch.`,
      )
    ) {
      await mergeWorktree(worktreeId, name, false, errorHandler, autoCleanup);
    }
  };

  // Handle delete with confirmation
  const handleConfirmDelete = async (
    worktreeId: string,
    name: string,
    isDirty: boolean,
    commitCount: number,
  ) => {
    const changesList = [];
    if (isDirty) changesList.push("uncommitted changes");
    if (commitCount > 0) changesList.push(`${commitCount} commits`);

    const confirmMessage =
      changesList.length > 0
        ? `Delete worktree "${name}"? This worktree has ${changesList.join(" and ")}. This action cannot be undone.`
        : `Delete worktree "${name}"? This action cannot be undone.`;

    if (window.confirm(confirmMessage)) {
      await deleteWorktree(worktreeId);
    }
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
          <RotateCcw className="w-4 h-4 mr-2" />
          Refresh
        </Button>
      </div>

      {/* Main Content */}
      {loading ? (
        <div className="flex justify-center">
          <Loader2 className="animate-spin" />
        </div>
      ) : (
        <div className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Left Column: Create New Workspace */}
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

            {/* Right Column: Local Repositories */}
            {gitStatus.repositories &&
            Object.keys(gitStatus.repositories).length > 0 ? (
              <Card>
                <CardHeader>
                  <CardTitle>Local Repositories</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="max-h-96 overflow-y-auto space-y-4">
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
                                  .filter(
                                    (b: string) => !b.startsWith("origin/HEAD"),
                                  )
                                  .sort((a: string, b: string) => {
                                    if (repo.id.startsWith("local/")) {
                                      const repoWorktrees = worktrees.filter(
                                        (wt: any) => wt.repo_id === repo.id,
                                      );
                                      const currentBranch = repoWorktrees.find(
                                        (wt: any) => wt.commits_behind === 0,
                                      )?.source_branch
                                        ? repoWorktrees.find(
                                            (wt: any) =>
                                              wt.commits_behind === 0,
                                          )?.source_branch
                                        : repo.default_branch ||
                                          allBranches?.[0];

                                      if (a === currentBranch) return -1;
                                      if (b === currentBranch) return 1;
                                    }
                                    return (
                                      (a ===
                                      (repo.default_branch || allBranches?.[0])
                                        ? -1
                                        : 1) -
                                      (b ===
                                      (repo.default_branch || allBranches?.[0])
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

                        {repo.id.startsWith("local/") ? (
                          <div className="space-y-1">
                            <p className="text-sm text-muted-foreground">
                              Local repository: {repo.id.split("/")[1]}.git
                            </p>
                          </div>
                        ) : (
                          <div className="space-y-1">
                            <p className="text-sm text-muted-foreground">
                              Remote repository
                            </p>
                            <Button
                              size="sm"
                              onClick={() => {
                                const repoName = repo.id.includes("/")
                                  ? repo.id.split("/").pop()
                                  : repo.id;
                                const cloneCommand = `git clone localhost:8080/${repoName}.git`;
                                void navigator.clipboard.writeText(
                                  cloneCommand,
                                );
                              }}
                            >
                              <Copy className="h-4 w-4 mr-1" />
                              Clone externally
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
                  </div>
                  <div className="text-sm text-muted-foreground text-center mt-4">
                    <span>
                      {
                        Object.keys(
                          gitStatus.repositories as Record<
                            string,
                            LocalRepository
                          >,
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

          {/* Active Workspaces Section - Full Width */}
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
                      <div key={worktree.id} className="border rounded-lg p-4">
                        <div className="flex items-center justify-between">
                          <div className="flex-1">
                            <div className="flex items-center gap-4">
                              <div>
                                <h3 className="font-semibold">
                                  {worktree.name}
                                </h3>
                                <p className="text-sm text-muted-foreground flex items-center gap-2">
                                  <span>
                                    {worktree.branch} •{" "}
                                    {worktree.commit_hash.slice(0, 8)}
                                    {worktree.commit_count > 0 && (
                                      <span className="ml-2 text-blue-600">
                                        • {worktree.commit_count} commit
                                        {worktree.commit_count !== 1 ? "s" : ""}
                                      </span>
                                    )}
                                  </span>
                                  {worktree.pull_request_url && (
                                    <a
                                      href={worktree.pull_request_url}
                                      target="_blank"
                                      rel="noopener noreferrer"
                                      onClick={(e) => e.stopPropagation()}
                                      className="inline-flex items-center gap-1 text-blue-600 hover:text-blue-800 transition-colors"
                                      title="View Pull Request"
                                    >
                                      <GitPullRequest className="w-3 h-3" />
                                      PR
                                      <ExternalLink className="w-2.5 h-2.5" />
                                    </a>
                                  )}
                                </p>
                                <div className="flex gap-2 mt-1">
                                  {worktree.is_dirty && (
                                    <Badge variant="secondary">Dirty</Badge>
                                  )}
                                  {worktree.has_conflicts && (
                                    <Badge variant="destructive">
                                      Conflicts
                                    </Badge>
                                  )}
                                </div>
                                {/* Session Title */}
                                <SessionTitle
                                  worktree={worktree}
                                  isActive={
                                    worktree.has_active_claude_session ?? false
                                  }
                                  pullRequestUrl={worktree.pull_request_url}
                                />
                              </div>
                            </div>
                          </div>
                          <div className="flex gap-2">
                            {/* View Diff Button */}
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                void toggleDiff(worktree.id);
                              }}
                              disabled={
                                diffLoading &&
                                openDiffWorktreeId === worktree.id
                              }
                              className={
                                openDiffWorktreeId === worktree.id
                                  ? "bg-muted"
                                  : ""
                              }
                              title={
                                diffLoading &&
                                openDiffWorktreeId === worktree.id
                                  ? "Loading diff..."
                                  : undefined
                              }
                            >
                              {diffLoading &&
                              openDiffWorktreeId === worktree.id ? (
                                <Loader2 className="w-4 h-4 mr-1 animate-spin" />
                              ) : (
                                <Eye className="w-4 h-4 mr-1" />
                              )}
                              {openDiffWorktreeId === worktree.id
                                ? "Hide"
                                : "View"}{" "}
                              Diff
                            </Button>

                            {/* Vibe Button */}
                            <Button variant="outline" size="sm" asChild>
                              <Link
                                to="/terminal/$sessionId"
                                params={{ sessionId: worktree.name }}
                                search={{
                                  agent: "claude",
                                }}
                              >
                                <img
                                  src="/anthropic.png"
                                  alt="Claude"
                                  className="w-4 h-4 mr-1"
                                />
                                Vibe
                              </Link>
                            </Button>

                            {/* WorkspaceActions Dropdown */}
                            <WorkspaceActions
                              mode="worktree"
                              worktree={worktree}
                              prStatus={
                                worktree.pull_request_url
                                  ? {
                                      exists: true,
                                      has_commits_ahead:
                                        worktree.commit_count > 0,
                                      url: worktree.pull_request_url,
                                      // We don't have title/body here, so PullRequestDialog will use fallbacks
                                    }
                                  : undefined
                              }
                              isSyncing={syncingWorktrees.has(worktree.id)}
                              isMerging={mergingWorktrees.has(worktree.id)}
                              onSync={(id) => syncWorktree(id, errorHandler)}
                              onMerge={handleMerge}
                              onCreatePreview={handleCreatePreview}
                              onConfirmDelete={handleConfirmDelete}
                            />
                          </div>
                        </div>

                        {/* Diff Section */}
                        <DiffViewer
                          worktreeId={worktree.id}
                          isOpen={openDiffWorktreeId === worktree.id}
                          onClose={() => toggleDiff(worktree.id)}
                          onLoadingChange={setDiffLoading}
                        />
                      </div>
                    ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}

export const Route = createFileRoute("/git")({
  component: GitPage,
});
