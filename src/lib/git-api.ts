import { toast } from "sonner";
import { createMergeConflictPrompt } from "./git-utils";

export interface GitStatus {
  repositories?: Record<string, LocalRepository>;
  worktree_count?: number;
}

export interface TitleEntry {
  title: string;
  timestamp: string;
  commit_hash?: string;
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
  created_at: string;
  last_accessed: string;
  session_title?: TitleEntry;
  session_title_history?: TitleEntry[];
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
          const sessionId = encodeURIComponent(worktreeName);
          const terminalUrl = `/terminal/${sessionId}`;

          const conflictText =
            conflictFiles.length > 0
              ? `Conflicts in: ${conflictFiles.join(", ")}`
              : "Multiple files have conflicts";

          const claudePrompt = createMergeConflictPrompt("sync", conflictFiles);

          errorHandler.setErrorAlert({
            open: true,
            title: `Merge Conflict in ${worktreeName}`,
            description: `${conflictText}\n\nOpen terminal to resolve: ${terminalUrl}\n\nSuggested Claude prompt: "${claudePrompt}"`,
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
  ): Promise<boolean> {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/merge`, {
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
          const sessionId = encodeURIComponent(worktreeName);
          const terminalUrl = `/terminal/${sessionId}`;

          const conflictText =
            conflictFiles.length > 0
              ? `Conflicts in: ${conflictFiles.join(", ")}`
              : "Multiple files have conflicts";

          const claudePrompt = createMergeConflictPrompt(
            "merge",
            conflictFiles,
          );

          errorHandler.setErrorAlert({
            open: true,
            title: `Merge Conflict in ${worktreeName}`,
            description: `${conflictText}\n\nOpen terminal to resolve: ${terminalUrl}\n\nSuggested Claude prompt: "${claudePrompt}"`,
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

    const worktreeIds = worktrees.map((wt) => wt.id);

    try {
      // Use batch endpoints for both sync and merge conflicts
      const [syncConflicts, mergeConflicts] = await Promise.all([
        this.checkBatchSyncConflicts(worktreeIds),
        this.checkBatchMergeConflicts(
          worktrees
            .filter((wt) => wt.repo_id.startsWith("local/"))
            .map((wt) => wt.id),
        ),
      ]);

      return { syncConflicts, mergeConflicts };
    } catch (error) {
      console.error(
        "Batch conflict check failed, falling back to individual requests:",
        error,
      );
      return await this.checkAllConflictsFallback(worktrees);
    }
  },

  async checkBatchSyncConflicts(
    worktreeIds: string[],
  ): Promise<Record<string, any>> {
    if (worktreeIds.length === 0) {
      return {};
    }

    const response = await fetch("/v1/git/worktrees/batch/sync/check", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ worktree_ids: worktreeIds }),
    });

    if (!response.ok) {
      throw new Error(
        `Batch sync conflict check failed: ${response.statusText}`,
      );
    }

    const results = await response.json();

    // Only return entries that have conflicts
    const conflicts: Record<string, any> = {};
    for (const [worktreeId, data] of Object.entries(results)) {
      if (
        data &&
        typeof data === "object" &&
        "has_conflicts" in data &&
        data.has_conflicts
      ) {
        conflicts[worktreeId] = data;
      }
    }

    return conflicts;
  },

  async checkBatchMergeConflicts(
    worktreeIds: string[],
  ): Promise<Record<string, any>> {
    if (worktreeIds.length === 0) {
      return {};
    }

    const response = await fetch("/v1/git/worktrees/batch/merge/check", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ worktree_ids: worktreeIds }),
    });

    if (!response.ok) {
      throw new Error(
        `Batch merge conflict check failed: ${response.statusText}`,
      );
    }

    const results = await response.json();

    // Only return entries that have conflicts
    const conflicts: Record<string, any> = {};
    for (const [worktreeId, data] of Object.entries(results)) {
      if (
        data &&
        typeof data === "object" &&
        "has_conflicts" in data &&
        data.has_conflicts
      ) {
        conflicts[worktreeId] = data;
      }
    }

    return conflicts;
  },

  async checkAllConflictsFallback(worktrees: Worktree[]): Promise<{
    syncConflicts: Record<string, any>;
    mergeConflicts: Record<string, any>;
  }> {
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

    try {
      // Use batch endpoint to get all diff stats in one request
      const worktreeIds = worktrees.map((wt) => wt.id);
      const response = await fetch("/v1/git/worktrees/batch/diff", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ worktree_ids: worktreeIds }),
      });

      if (response.ok) {
        const results = await response.json();
        const diffStats: Record<string, WorktreeDiffStats> = {};

        for (const [worktreeId, data] of Object.entries(results)) {
          if (data && typeof data === "object" && !("error" in data)) {
            diffStats[worktreeId] = {
              summary: (data as any)?.summary || "",
              file_diffs: (data as any)?.file_diffs || [],
              total_files: (data as any)?.total_files || 0,
              worktree_id: (data as any)?.worktree_id || "",
              worktree_name: (data as any)?.worktree_name || "",
              source_branch: (data as any)?.source_branch || "",
              fork_commit: (data as any)?.fork_commit || "",
            };
          }
        }

        return diffStats;
      }

      // Fallback to individual requests if batch endpoint fails
      console.warn(
        "Batch diff endpoint failed, falling back to individual requests",
      );
      return await this.fetchAllDiffStatsFallback(worktrees);
    } catch (error) {
      console.error("Batch diff request failed:", error);
      return await this.fetchAllDiffStatsFallback(worktrees);
    }
  },

  async fetchAllDiffStatsFallback(
    worktrees: Worktree[],
  ): Promise<Record<string, WorktreeDiffStats | undefined>> {
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

  async fetchAllPullRequestInfo(
    worktrees: Worktree[],
  ): Promise<Record<string, PullRequestInfo | undefined>> {
    if (worktrees.length === 0) {
      return {};
    }

    // Use batch endpoint to get all PR info in one request
    try {
      const worktreeIds = worktrees.map((wt) => wt.id);
      const response = await fetch("/v1/git/worktrees/batch/pr", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ worktree_ids: worktreeIds }),
      });

      if (response.ok) {
        return await response.json();
      }

      // Fallback to individual requests if batch endpoint fails
      console.warn(
        "Batch PR endpoint failed, falling back to individual requests",
      );
      return await this.fetchAllPullRequestInfoFallback(worktrees);
    } catch (error) {
      console.error("Batch PR request failed:", error);
      return await this.fetchAllPullRequestInfoFallback(worktrees);
    }
  },

  async fetchAllPullRequestInfoFallback(
    worktrees: Worktree[],
  ): Promise<Record<string, PullRequestInfo | undefined>> {
    // Process in parallel but limit concurrency to avoid overwhelming the connection pool
    const batchSize = 3;
    const results: Record<string, PullRequestInfo | undefined> = {};

    for (let i = 0; i < worktrees.length; i += batchSize) {
      const batch = worktrees.slice(i, i + batchSize);
      const promises = batch.map(async (worktree) => {
        const prInfo = await this.getPullRequestInfo(worktree.id);
        return { worktreeId: worktree.id, prInfo };
      });

      const batchResults = await Promise.all(promises);
      batchResults.forEach(({ worktreeId, prInfo }) => {
        if (prInfo) {
          results[worktreeId] = prInfo;
        }
      });
    }

    return results;
  },
};
