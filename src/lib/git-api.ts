import { toast } from "sonner";

export interface GitStatus {
  repositories?: Record<string, LocalRepository>;
  worktree_count?: number;
}

export interface TitleEntry {
  title: string;
  timestamp: string;
  commit_hash?: string;
}

export interface CacheStatus {
  is_cached: boolean;
  is_loading: boolean;
  last_updated: number;
}

export interface Worktree {
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
  has_conflicts: boolean;
  dirty_files?: string[];
  created_at: string;
  last_accessed: string;
  session_title?: TitleEntry;
  session_title_history?: TitleEntry[];
  cache_status?: CacheStatus;
  has_active_claude_session?: boolean;
  pull_request_url?: string;
}

interface Owner {
  id: string;
  name: string;
}

export interface Repository {
  url: string;
  description?: string;
  fullName?: string;
  name?: string;
  private?: boolean;
  owner: Owner;
}

export interface LocalRepository {
  created_at: string;
  default_branch: string;
  description: string;
  id: string;
  last_accessed: string;
  name: string;
  path: string;
  url: string;
}

interface FileDiff {
  file_path: string;
  change_type: string;
  old_content?: string;
  new_content?: string;
  diff_text?: string;
  is_expanded: boolean;
}

export interface WorktreeDiffStats {
  summary: string;
  file_diffs: FileDiff[];
  total_files: number;
  worktree_id: string;
  worktree_name: string;
  source_branch: string;
  fork_commit: string;
}

export interface PullRequestInfo {
  has_commits_ahead: boolean;
  exists: boolean;
  title?: string;
  body?: string;
  number?: number;
  url?: string;
}

export interface ErrorHandler {
  setErrorAlert: (alert: {
    open: boolean;
    title: string;
    description: string;
    worktreeName?: string;
    conflictFiles?: string[];
    operation?: string;
  }) => void;
}

export const gitApi = {
  async fetchGitStatus(): Promise<GitStatus> {
    const response = await fetch("/v1/git/status");
    if (response.ok) {
      return await response.json();
    }
    throw new Error("Failed to fetch git status");
  },

  async fetchWorktrees(): Promise<Worktree[]> {
    const response = await fetch("/v1/git/worktrees");
    if (response.ok) {
      return await response.json();
    }
    throw new Error("Failed to fetch worktrees");
  },

  async fetchRepositories(): Promise<Repository[]> {
    const response = await fetch("/v1/git/github/repos");
    if (response.ok) {
      return await response.json();
    }
    throw new Error("Failed to fetch repositories");
  },

  async fetchBranches(repoId: string): Promise<string[]> {
    const response = await fetch(
      `/v1/git/branches/${encodeURIComponent(repoId)}`,
    );
    if (response.ok) {
      return await response.json();
    }
    return [];
  },

  async fetchClaudeSessions(): Promise<Record<string, any>> {
    try {
      const response = await fetch("/v1/claude/sessions");
      if (response.ok) {
        return (await response.json()) || {};
      }
      return {};
    } catch (error) {
      console.error("Failed to fetch Claude sessions:", error);
      return {};
    }
  },

  async fetchActiveSessions(): Promise<Record<string, any>> {
    try {
      const response = await fetch("/v1/sessions/active");
      if (response.ok) {
        return (await response.json()) || {};
      }
      return {};
    } catch (error) {
      console.error("Failed to fetch active sessions:", error);
      return {};
    }
  },

  async checkSyncConflicts(worktreeId: string): Promise<any> {
    try {
      const response = await fetch(
        `/v1/git/worktrees/${worktreeId}/sync/check`,
      );
      if (response.ok) {
        return await response.json();
      }
      return null;
    } catch (error) {
      console.error(`Failed to check sync conflicts for ${worktreeId}:`, error);
      return null;
    }
  },

  async checkMergeConflicts(worktreeId: string): Promise<any> {
    try {
      const response = await fetch(
        `/v1/git/worktrees/${worktreeId}/merge/check`,
      );
      if (response.ok) {
        return await response.json();
      }
      return null;
    } catch (error) {
      console.error(
        `Failed to check merge conflicts for ${worktreeId}:`,
        error,
      );
      return null;
    }
  },

  async deleteWorktree(id: string): Promise<void> {
    const response = await fetch(`/v1/git/worktrees/${id}`, {
      method: "DELETE",
    });
    if (!response.ok) {
      throw new Error("Failed to delete worktree");
    }
  },

  async syncWorktree(id: string, errorHandler: ErrorHandler): Promise<boolean> {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/sync`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ strategy: "rebase" }),
      });

      if (response.ok) {
        toast.success("Successfully synced worktree");
        return true;
      } else {
        const errorData = await response.json();
        if (errorData.error === "merge_conflict") {
          const worktreeName = errorData.worktree_name;
          const conflictFiles = errorData.conflict_files || [];

          errorHandler.setErrorAlert({
            open: true,
            title: `Sync Conflict in ${worktreeName}`,
            description: "", // Will be set by the enhanced handler
            worktreeName,
            conflictFiles,
            operation: "rebase",
          });
          return false;
        }

        errorHandler.setErrorAlert({
          open: true,
          title: "Sync Failed",
          description: `Failed to sync worktree: ${errorData.error}`,
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to sync worktree:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Sync Failed",
        description: `Failed to sync worktree: ${error}`,
      });
      return false;
    }
  },

  async mergeWorktree(
    id: string,
    worktreeName: string,
    squash: boolean,
    errorHandler: ErrorHandler,
    autoCleanup = true,
  ): Promise<boolean> {
    try {
      const url = `/v1/git/worktrees/${id}/merge?auto_cleanup=${autoCleanup}`;
      const response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ squash }),
      });

      if (response.ok) {
        const mergeType = squash ? "squash merged" : "merged";
        toast.success(
          `Successfully ${mergeType} ${worktreeName} to main branch`,
        );
        return true;
      } else {
        const errorData = await response.json();
        if (errorData.error === "merge_conflict") {
          const conflictFiles = errorData.conflict_files || [];

          errorHandler.setErrorAlert({
            open: true,
            title: `Merge Conflict in ${worktreeName}`,
            description: "", // Will be set by the enhanced handler
            worktreeName,
            conflictFiles,
            operation: "merge",
          });
          return false;
        }

        errorHandler.setErrorAlert({
          open: true,
          title: "Merge Failed",
          description: `Failed to merge worktree: ${errorData.error}`,
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to merge worktree:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Merge Failed",
        description: `Failed to merge worktree: ${error}`,
      });
      return false;
    }
  },

  async createWorktreePreview(
    id: string,
    errorHandler: ErrorHandler,
  ): Promise<boolean> {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/preview`, {
        method: "POST",
      });

      if (response.ok) {
        return true;
      } else {
        const errorData = await response.json();
        errorHandler.setErrorAlert({
          open: true,
          title: "Preview Failed",
          description: `Failed to create preview: ${errorData.error}`,
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to create preview:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Preview Failed",
        description: `Failed to create preview: ${error}`,
      });
      return false;
    }
  },

  async fetchBranchesForRepositories(
    repositories: Record<string, any>,
  ): Promise<Record<string, string[]>> {
    const branchPromises = Object.keys(repositories).map(async (repoId) => {
      const branches = await this.fetchBranches(repoId);
      return { repoId, branches };
    });

    const branchResults = await Promise.all(branchPromises);
    const branchMap: Record<string, string[]> = {};
    branchResults.forEach(({ repoId, branches }) => {
      branchMap[repoId] = branches;
    });
    return branchMap;
  },

  async fetchWorktreeDiffStats(
    worktreeId: string,
  ): Promise<WorktreeDiffStats | null> {
    try {
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/diff`);
      if (response.ok) {
        const data = await response.json();
        return {
          summary: data?.summary || "",
          file_diffs: data?.file_diffs || [],
          total_files: data?.total_files || 0,
          worktree_id: data?.worktree_id || "",
          worktree_name: data?.worktree_name || "",
          source_branch: data?.source_branch || "",
          fork_commit: data?.fork_commit || "",
        };
      }
      return null;
    } catch (error) {
      console.error(`Failed to fetch diff stats for ${worktreeId}:`, error);
      return null;
    }
  },

  async checkAllConflicts(worktrees: Worktree[]): Promise<{
    syncConflicts: Record<string, any>;
    mergeConflicts: Record<string, any>;
  }> {
    if (worktrees.length === 0) {
      return { syncConflicts: {}, mergeConflicts: {} };
    }

    const syncPromises = worktrees.map(async (worktree) => {
      const data = await this.checkSyncConflicts(worktree.id);
      return { worktreeId: worktree.id, data };
    });

    const mergePromises = worktrees.map(async (worktree) => {
      if (!worktree.repo_id.startsWith("local/")) {
        return { worktreeId: worktree.id, data: null };
      }
      const data = await this.checkMergeConflicts(worktree.id);
      return { worktreeId: worktree.id, data };
    });

    const [syncResults, mergeResults] = await Promise.all([
      Promise.all(syncPromises),
      Promise.all(mergePromises),
    ]);

    const syncConflicts: Record<string, any> = {};
    syncResults.forEach(({ worktreeId, data }) => {
      if (data) {
        syncConflicts[worktreeId] = data;
      }
    });

    const mergeConflicts: Record<string, any> = {};
    mergeResults.forEach(({ worktreeId, data }) => {
      if (data) {
        mergeConflicts[worktreeId] = data;
      }
    });

    return { syncConflicts, mergeConflicts };
  },

  async fetchAllDiffStats(
    worktrees: Worktree[],
  ): Promise<Record<string, WorktreeDiffStats | undefined>> {
    if (worktrees.length === 0) {
      return {};
    }

    const diffPromises = worktrees.map(async (worktree) => {
      const data = await this.fetchWorktreeDiffStats(worktree.id);
      return { worktreeId: worktree.id, data };
    });

    const diffResults = await Promise.all(diffPromises);
    const diffStats: Record<string, WorktreeDiffStats> = {};

    diffResults.forEach(({ worktreeId, data }) => {
      if (data) {
        diffStats[worktreeId] = data;
      }
    });

    return diffStats;
  },

  // Enhanced PR management functions
  async createPullRequest(
    worktreeId: string,
    title: string,
    body: string,
    errorHandler: ErrorHandler,
  ): Promise<boolean> {
    try {
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/pr`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title, body }),
      });

      if (response.ok) {
        const prData = await response.json();
        toast.success(
          `Pull request created! PR #${prData.number}: ${prData.title}`,
        );
        return true;
      } else {
        const errorData = await response.json();
        errorHandler.setErrorAlert({
          open: true,
          title: "Pull Request Failed",
          description: `Failed to create pull request: ${errorData.error || "Unknown error"}`,
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to create pull request:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Pull Request Failed",
        description: `Failed to create pull request: ${error}`,
      });
      return false;
    }
  },

  async updatePullRequest(
    worktreeId: string,
    title: string,
    body: string,
    errorHandler: ErrorHandler,
  ): Promise<boolean> {
    try {
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/pr`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title, body }),
      });

      if (response.ok) {
        const prData = await response.json();
        toast.success(
          `Pull request updated! PR #${prData.number}: ${prData.title}`,
        );
        return true;
      } else {
        const errorData = await response.json();
        errorHandler.setErrorAlert({
          open: true,
          title: "Pull Request Update Failed",
          description: `Failed to update pull request: ${errorData.error || "Unknown error"}`,
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to update pull request:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Pull Request Update Failed",
        description: `Failed to update pull request: ${error}`,
      });
      return false;
    }
  },

  async getPullRequestInfo(
    worktreeId: string,
  ): Promise<PullRequestInfo | null> {
    try {
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/pr`);
      if (response.ok) {
        return await response.json();
      }
      return null;
    } catch (error) {
      console.error("Failed to get pull request info:", error);
      return null;
    }
  },
};
