import { createFileRoute, Link } from "@tanstack/react-router";
import { useState, useEffect } from "react";
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
import {
  GitBranch,
  Copy,
  RefreshCw,
  Trash2,
  GitMerge,
  Eye,
  AlertTriangle,
} from "lucide-react";
import { toast } from "sonner";

// Utility function for relative time display
const getRelativeTime = (date: string | Date) => {
  const now = new Date();
  const then = new Date(date);
  const diffMs = now.getTime() - then.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "just now";
  if (diffMins < 60)
    return `${diffMins} minute${diffMins !== 1 ? "s" : ""} ago`;
  if (diffHours < 24)
    return `${diffHours} hour${diffHours !== 1 ? "s" : ""} ago`;
  return `${diffDays} day${diffDays !== 1 ? "s" : ""} ago`;
};

const getDuration = (startDate: string | Date, endDate: string | Date) => {
  const start = new Date(startDate);
  const end = new Date(endDate);
  const diffMs = end.getTime() - start.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);

  if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? "s" : ""}`;
  if (diffHours < 24)
    return `${diffHours} hour${diffHours !== 1 ? "s" : ""} ${
      diffMins % 60
    } minute${diffMins % 60 !== 1 ? "s" : ""}`;
  return `${Math.floor(diffHours / 24)} day${
    Math.floor(diffHours / 24) !== 1 ? "s" : ""
  }`;
};

interface GitStatus {
  repository?: {
    id: string;
    url: string;
    path: string;
    default_branch: string;
  };
  repositories?: Record<string, any>;
  active_worktree?: {
    id: string;
    repo_id: string;
    name: string;
    path: string;
    branch: string;
    commit_hash: string;
    is_dirty: boolean;
  };
  worktree_count?: number;
}

interface Worktree {
  id: string;
  repo_id: string;
  name: string;
  branch: string;
  source_branch: string;
  path: string;
  commit_hash: string;
  commit_count: number;
  commits_behind: number;
  is_dirty: boolean;
}

interface Repository {
  name: string;
  url: string;
  private: boolean;
  description?: string;
  fullName?: string;
}

function GitPage() {
  const [githubUrl, setGithubUrl] = useState("");
  const [gitStatus, setGitStatus] = useState<GitStatus>({});
  const [worktrees, setWorktrees] = useState<Worktree[]>([]);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repoBranches, setRepoBranches] = useState<Record<string, string[]>>(
    {}
  );
  const [claudeSessions, setClaudeSessions] = useState<Record<string, any>>({});
  const [activeSessions, setActiveSessions] = useState<Record<string, any>>({});
  const [syncConflicts, setSyncConflicts] = useState<Record<string, any>>({});
  const [mergeConflicts, setMergeConflicts] = useState<Record<string, any>>({});
  const [loading, setLoading] = useState(false);
  const [reposLoading, setReposLoading] = useState(false);
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

  const fetchGitStatus = async () => {
    try {
      const response = await fetch("/v1/git/status");
      if (response.ok) {
        const data = await response.json();
        setGitStatus(data);

        // Fetch branches for each repository
        if (data.repositories) {
          const branchPromises = Object.keys(data.repositories).map(
            async (repoId) => {
              try {
                const branchResponse = await fetch(
                  `/v1/git/branches/${encodeURIComponent(repoId)}`
                );
                if (branchResponse.ok) {
                  const branches = await branchResponse.json();
                  return { repoId, branches };
                }
              } catch (error) {
                console.error(`Failed to fetch branches for ${repoId}:`, error);
              }
              return { repoId, branches: [] };
            }
          );

          const branchResults = await Promise.all(branchPromises);
          const branchMap: Record<string, string[]> = {};
          branchResults.forEach(({ repoId, branches }) => {
            branchMap[repoId] = branches;
          });
          setRepoBranches(branchMap);
        }
      }
    } catch (error) {
      console.error("Failed to fetch git status:", error);
    }
  };

  const fetchWorktrees = async () => {
    try {
      const response = await fetch("/v1/git/worktrees");
      if (response.ok) {
        const data = await response.json();
        setWorktrees(data);
      }
    } catch (error) {
      console.error("Failed to fetch worktrees:", error);
    }
  };

  const fetchClaudeSessions = async () => {
    try {
      const response = await fetch("/v1/claude/sessions");
      if (response.ok) {
        const data = await response.json();
        setClaudeSessions(data || {});
      } else {
        // Don't error on missing Claude data, just set empty object
        setClaudeSessions({});
      }
    } catch (error) {
      console.error("Failed to fetch Claude sessions:", error);
      // Set empty object on error so UI doesn't break
      setClaudeSessions({});
    }
  };

  const fetchRepositories = async () => {
    setReposLoading(true);
    try {
      const response = await fetch("/v1/git/github/repos");
      if (response.ok) {
        const data = await response.json();
        setRepositories(data);
      }
    } catch (error) {
      console.error("Failed to fetch repositories:", error);
    } finally {
      setReposLoading(false);
    }
  };

  const fetchActiveSessions = async () => {
    try {
      const response = await fetch("/v1/sessions/active");
      if (response.ok) {
        const data = await response.json();
        setActiveSessions(data || {});
      }
    } catch (error) {
      console.error("Failed to fetch active sessions:", error);
      setActiveSessions({});
    }
  };

  const checkConflicts = async () => {
    if (worktrees.length === 0) return;
    
    const syncConflictPromises = worktrees.map(async (worktree) => {
      try {
        const response = await fetch(`/v1/git/worktrees/${worktree.id}/sync/check`);
        if (response.ok) {
          const data = await response.json();
          return { worktreeId: worktree.id, data };
        }
      } catch (error) {
        console.error(`Failed to check sync conflicts for ${worktree.id}:`, error);
      }
      return { worktreeId: worktree.id, data: null };
    });

    const mergeConflictPromises = worktrees.map(async (worktree) => {
      // Only check merge conflicts for local repos
      if (!worktree.repo_id.startsWith("local/")) {
        return { worktreeId: worktree.id, data: null };
      }
      
      try {
        const response = await fetch(`/v1/git/worktrees/${worktree.id}/merge/check`);
        if (response.ok) {
          const data = await response.json();
          return { worktreeId: worktree.id, data };
        }
      } catch (error) {
        console.error(`Failed to check merge conflicts for ${worktree.id}:`, error);
      }
      return { worktreeId: worktree.id, data: null };
    });

    const [syncResults, mergeResults] = await Promise.all([
      Promise.all(syncConflictPromises),
      Promise.all(mergeConflictPromises)
    ]);

    // Update sync conflicts state
    const newSyncConflicts: Record<string, any> = {};
    syncResults.forEach(({ worktreeId, data }) => {
      if (data) {
        newSyncConflicts[worktreeId] = data;
      }
    });
    setSyncConflicts(newSyncConflicts);

    // Update merge conflicts state
    const newMergeConflicts: Record<string, any> = {};
    mergeResults.forEach(({ worktreeId, data }) => {
      if (data) {
        newMergeConflicts[worktreeId] = data;
      }
    });
    setMergeConflicts(newMergeConflicts);
  };

  const handleCheckout = async (url: string) => {
    setLoading(true);
    try {
      // Handle local repositories (format: local/dirname)
      if (url.startsWith("local/")) {
        const parts = url.split("/");
        if (parts.length >= 2) {
          // Format: local/dirname -> send as org=local, repo=dirname
          const response = await fetch(`/v1/git/checkout/${parts[0]}/${parts[1]}`, {
            method: "POST",
          });
          if (response.ok) {
            fetchGitStatus();
            fetchWorktrees();
            fetchActiveSessions();
            toast.success("Local repository checked out successfully");
          } else {
            const errorData = await response.json();
            console.error("Failed to checkout local repository:", errorData);
            toast.error(`Failed to checkout local repository: ${errorData.error || 'Unknown error'}`);
          }
        }
      } else if (url.startsWith("https://github.com/")) {
        // Handle regular GitHub repos
        const urlParts = url.replace("https://github.com/", "").split("/");
        if (urlParts.length >= 2) {
          const [org, repoWithGit] = urlParts;
          // Remove .git extension if present
          const repo = repoWithGit.replace(/\.git$/, "");
          const response = await fetch(`/v1/git/checkout/${org}/${repo}`, {
            method: "POST",
          });
          if (response.ok) {
            fetchGitStatus();
            fetchWorktrees();
            fetchActiveSessions();
            toast.success("Repository checked out successfully");
          } else {
            const errorData = await response.json();
            console.error("Failed to checkout repository:", errorData);
            toast.error(`Failed to checkout repository: ${errorData.error || 'Unknown error'}`);
          }
        }
      } else {
        console.error("Unknown URL format:", url);
        toast.error(`Unknown repository URL format: ${url}`);
      }
    } catch (error) {
      console.error("Failed to checkout repository:", error);
      toast.error(`Failed to checkout repository: ${error}`);
    } finally {
      setLoading(false);
    }
  };

  const deleteWorktree = async (id: string) => {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}`, {
        method: "DELETE",
      });
      if (response.ok) {
        fetchWorktrees();
        fetchActiveSessions();
      }
    } catch (error) {
      console.error("Failed to delete worktree:", error);
    }
  };

  const copyRemoteCommand = (url: string) => {
    const command = `git remote add catnip ${url} && git fetch catnip`;
    navigator.clipboard.writeText(command);
    toast.success("Command copied to clipboard");
  };

  const handleMergeConflict = (errorData: any, operation: string) => {
    if (errorData.error === "merge_conflict") {
      const worktreeName = errorData.worktree_name;
      const conflictFiles = errorData.conflict_files || [];
      const sessionId = encodeURIComponent(worktreeName);
      const terminalUrl = `/terminal/${sessionId}`;
      
      const conflictText = conflictFiles.length > 0 
        ? `Conflicts in: ${conflictFiles.join(", ")}`
        : "Multiple files have conflicts";

      const claudePrompt = `I have a merge conflict during a ${operation} operation. ${conflictText}. Please help me resolve these conflicts by examining the files, understanding the conflicting changes, and providing a resolution strategy.`;

      toast.error(
        <div className="space-y-2">
          <div className="font-semibold">Merge Conflict in {worktreeName}</div>
          <div className="text-sm">{conflictText}</div>
          <div className="space-y-1">
            <a 
              href={terminalUrl} 
              className="inline-block text-blue-600 hover:text-blue-800 underline text-sm"
              target="_blank"
              rel="noopener noreferrer"
            >
              Open Terminal to Resolve →
            </a>
            <div className="text-xs text-gray-600">
              Suggested Claude prompt: "{claudePrompt}"
            </div>
          </div>
        </div>,
        { duration: 15000 }
      );
      return true;
    }
    return false;
  };

  const syncWorktree = async (id: string) => {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/sync`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ strategy: "rebase" }),
      });
      if (response.ok) {
        fetchWorktrees();
        // Show success feedback
        toast.success(`Successfully synced worktree`);
      } else {
        const errorData = await response.json();
        if (!handleMergeConflict(errorData, "sync")) {
          toast.error(`Failed to sync worktree: ${errorData.error}`);
        }
      }
    } catch (error) {
      console.error("Failed to sync worktree:", error);
      toast.error(`Failed to sync worktree: ${error}`);
    }
  };

  const mergeWorktreeToMain = async (id: string, worktreeName: string) => {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/merge`, {
        method: "POST",
      });
      if (response.ok) {
        fetchWorktrees();
        fetchGitStatus();
        toast.success(`Successfully merged ${worktreeName} to main branch`);
      } else {
        const errorData = await response.json();
        if (!handleMergeConflict(errorData, "merge")) {
          toast.error(`Failed to merge worktree: ${errorData.error}`);
        }
      }
    } catch (error) {
      console.error("Failed to merge worktree:", error);
      toast.error(`Failed to merge worktree: ${error}`);
    }
  };

  const createWorktreePreview = async (id: string, branchName: string) => {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/preview`, {
        method: "POST",
      });
      if (response.ok) {
        const previewBranch = `preview/${branchName}`;
        toast.success(`Preview branch created! Run: git checkout ${previewBranch}`, {
          duration: 8000,
        });
      } else {
        const errorData = await response.json();
        toast.error(`Failed to create preview: ${errorData.error}`);
      }
    } catch (error) {
      console.error("Failed to create preview:", error);
      toast.error(`Failed to create preview: ${error}`);
    }
  };

  useEffect(() => {
    fetchGitStatus();
    fetchWorktrees();
    fetchRepositories();
    fetchClaudeSessions();
    fetchActiveSessions();
  }, []);

  useEffect(() => {
    // Check for conflicts when worktrees change
    if (worktrees.length > 0) {
      checkConflicts();
    }
  }, [worktrees]);

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
                <div
                  key={worktree.id}
                  className="flex items-center justify-between p-3 border rounded-lg"
                >
                  <div className="flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="font-medium">{worktree.name}</span>
                      {activeSessions[worktree.path] && (
                        <div
                          className="w-2 h-2 bg-green-500 rounded-full animate-pulse"
                          title="Active session running"
                        />
                      )}
                      <Badge variant="outline">
                        {worktree.repo_id}@{worktree.source_branch || "unknown"}
                      </Badge>
                      {worktree.is_dirty ? (
                        <Badge variant="destructive">Dirty</Badge>
                      ) : (
                        <Badge
                          variant="secondary"
                          className="text-xs bg-green-100 text-green-800 border-green-200"
                        >
                          Clean
                        </Badge>
                      )}
                      {worktree.commit_count > 0 && (
                        <Badge variant="secondary">
                          +{worktree.commit_count} commits
                        </Badge>
                      )}
                      {worktree.commits_behind > 0 && (
                        <Badge variant="outline" className="border-orange-200 text-orange-800 bg-orange-50">
                          {worktree.commits_behind} behind
                          {syncConflicts[worktree.id]?.has_conflicts && " ⚠️"}
                        </Badge>
                      )}
                      {(syncConflicts[worktree.id]?.has_conflicts || mergeConflicts[worktree.id]?.has_conflicts) && (
                        <Badge variant="outline" className="border-red-200 text-red-800 bg-red-50">
                          Conflicts detected
                        </Badge>
                      )}
                      {claudeSessions[worktree.path] && (
                        <>
                          <Badge variant="secondary" className="text-xs">
                            {claudeSessions[worktree.path].turnCount} turns
                          </Badge>
                          {claudeSessions[worktree.path].lastCost > 0 && (
                            <Badge variant="secondary" className="text-xs">
                              $
                              {claudeSessions[worktree.path].lastCost.toFixed(
                                4
                              )}
                            </Badge>
                          )}
                        </>
                      )}
                    </div>
                    <div className="text-xs text-muted-foreground space-y-1">
                      <Link
                        to="/terminal/$sessionId"
                        params={{ sessionId: worktree.name }}
                        search={{ agent: undefined }}
                        className="cursor-pointer hover:text-primary underline-offset-4 hover:underline"
                      >
                        {worktree.path}
                      </Link>
                      {claudeSessions[worktree.path] ? (
                        <div className="space-y-1">
                          {claudeSessions[worktree.path].sessionStartTime &&
                          !claudeSessions[worktree.path].isActive ? (
                            // Finished session (has start time and is not active)
                            <p>
                              Finished:{" "}
                              {getRelativeTime(
                                claudeSessions[worktree.path].sessionEndTime ||
                                  claudeSessions[worktree.path].sessionStartTime
                              )}{" "}
                              • Lasted:{" "}
                              {getDuration(
                                claudeSessions[worktree.path].sessionStartTime,
                                claudeSessions[worktree.path].sessionEndTime ||
                                  claudeSessions[worktree.path].sessionStartTime
                              )}
                            </p>
                          ) : claudeSessions[worktree.path].sessionStartTime &&
                            claudeSessions[worktree.path].isActive ? (
                            // Active session with timing data
                            <p>
                              Running:{" "}
                              {getDuration(
                                claudeSessions[worktree.path].sessionStartTime,
                                new Date()
                              )}
                            </p>
                          ) : claudeSessions[worktree.path].isActive ? (
                            // Active session without timestamp data
                            <p>Running: recently started</p>
                          ) : (
                            // Completed session without timestamp data
                            <p>Session completed (timing data unavailable)</p>
                          )}
                        </div>
                      ) : (
                        <div className="space-y-1">
                          <p className="text-xs text-muted-foreground">
                            No Claude sessions
                          </p>
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <Link
                      to="/terminal/$sessionId"
                      params={{ sessionId: worktree.name }}
                      search={{ agent: "claude" }}
                    >
                      <Button variant="outline" size="sm" asChild>
                        <span>Vibe</span>
                      </Button>
                    </Link>
                    {worktree.commits_behind > 0 && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => syncWorktree(worktree.id)}
                        title={
                          syncConflicts[worktree.id]?.has_conflicts
                            ? `⚠️ Sync will cause conflicts: ${syncConflicts[worktree.id]?.conflict_files?.join(", ") || "multiple files"}`
                            : `Sync ${worktree.commits_behind} commits from ${worktree.source_branch}`
                        }
                        className={
                          syncConflicts[worktree.id]?.has_conflicts
                            ? "text-red-600 border-red-200 hover:bg-red-50"
                            : "text-orange-600 border-orange-200 hover:bg-orange-50"
                        }
                      >
                        {syncConflicts[worktree.id]?.has_conflicts ? (
                          <AlertTriangle size={16} />
                        ) : (
                          <RefreshCw size={16} />
                        )}
                      </Button>
                    )}
                    {worktree.repo_id.startsWith("local/") && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => createWorktreePreview(worktree.id, worktree.branch)}
                        title={`Create preview branch to view changes outside container`}
                        className="text-purple-600 border-purple-200 hover:bg-purple-50"
                      >
                        <Eye size={16} />
                      </Button>
                    )}
                    {worktree.repo_id.startsWith("local/") && worktree.commit_count > 0 && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setConfirmDialog({
                            open: true,
                            title: "Merge to Main",
                            description: mergeConflicts[worktree.id]?.has_conflicts
                              ? `⚠️ Warning: This merge will cause conflicts in ${mergeConflicts[worktree.id]?.conflict_files?.join(", ") || "multiple files"}. Merge ${worktree.commit_count} commits from "${worktree.name}" back to the ${worktree.source_branch} branch anyway?`
                              : `Merge ${worktree.commit_count} commits from "${worktree.name}" back to the ${worktree.source_branch} branch? This will make your changes available outside the container.`,
                            onConfirm: () => mergeWorktreeToMain(worktree.id, worktree.name),
                            variant: mergeConflicts[worktree.id]?.has_conflicts ? "destructive" : "default",
                          });
                        }}
                        title={
                          mergeConflicts[worktree.id]?.has_conflicts
                            ? `⚠️ Merge will cause conflicts: ${mergeConflicts[worktree.id]?.conflict_files?.join(", ") || "multiple files"}`
                            : `Merge ${worktree.commit_count} commits to ${worktree.source_branch}`
                        }
                        className={
                          mergeConflicts[worktree.id]?.has_conflicts
                            ? "text-red-600 border-red-200 hover:bg-red-50"
                            : "text-blue-600 border-blue-200 hover:bg-blue-50"
                        }
                      >
                        {mergeConflicts[worktree.id]?.has_conflicts ? (
                          <AlertTriangle size={16} />
                        ) : (
                          <GitMerge size={16} />
                        )}
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        if (worktree.is_dirty || worktree.commit_count > 0) {
                          const changesList = [];
                          if (worktree.is_dirty) changesList.push("uncommitted changes");
                          if (worktree.commit_count > 0) changesList.push(`${worktree.commit_count} commits`);
                          
                          setConfirmDialog({
                            open: true,
                            title: "Delete Worktree",
                            description: `Delete worktree "${worktree.name}"? This worktree has ${changesList.join(" and ")}. This action cannot be undone.`,
                            onConfirm: () => deleteWorktree(worktree.id),
                            variant: "destructive",
                          });
                        } else {
                          deleteWorktree(worktree.id);
                        }
                      }}
                      className="text-destructive hover:text-destructive"
                    >
                      <Trash2 size={16} />
                    </Button>
                  </div>
                </div>
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
              onClick={() => {
                fetchGitStatus();
                fetchWorktrees();
                fetchClaudeSessions();
                fetchActiveSessions();
              }}
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
    </div>
  );
}

export const Route = createFileRoute("/git")({
  component: GitPage,
});
