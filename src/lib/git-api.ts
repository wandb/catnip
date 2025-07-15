import { toast } from "sonner";
import { createMergeConflictPrompt } from "./git-utils";

export interface GitStatus {
  repositories?: Record<string, any>;
  worktree_count?: number;
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
}

export interface Repository {
  name: string;
  url: string;
  private: boolean;
  description?: string;
  fullName?: string;
}

export interface ErrorHandler {
  setErrorAlert: (alert: { open: boolean; title: string; description: string }) => void;
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
    const response = await fetch(`/v1/git/branches/${encodeURIComponent(repoId)}`);
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
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/sync/check`);
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
      const response = await fetch(`/v1/git/worktrees/${worktreeId}/merge/check`);
      if (response.ok) {
        return await response.json();
      }
      return null;
    } catch (error) {
      console.error(`Failed to check merge conflicts for ${worktreeId}:`, error);
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
          
          const conflictText = conflictFiles.length > 0 
            ? `Conflicts in: ${conflictFiles.join(", ")}`
            : "Multiple files have conflicts";

          const claudePrompt = createMergeConflictPrompt("sync", conflictFiles);

          errorHandler.setErrorAlert({
            open: true,
            title: `Merge Conflict in ${worktreeName}`,
            description: `${conflictText}\n\nOpen terminal to resolve: ${terminalUrl}\n\nSuggested Claude prompt: "${claudePrompt}"`
          });
          return false;
        }
        
        errorHandler.setErrorAlert({
          open: true,
          title: "Sync Failed",
          description: `Failed to sync worktree: ${errorData.error}`
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to sync worktree:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Sync Failed",
        description: `Failed to sync worktree: ${error}`
      });
      return false;
    }
  },

  async mergeWorktree(id: string, worktreeName: string, squash: boolean, errorHandler: ErrorHandler): Promise<boolean> {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/merge`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ squash }),
      });
      
      if (response.ok) {
        const mergeType = squash ? "squash merged" : "merged";
        toast.success(`Successfully ${mergeType} ${worktreeName} to main branch`);
        return true;
      } else {
        const errorData = await response.json();
        if (errorData.error === "merge_conflict") {
          const conflictFiles = errorData.conflict_files || [];
          const sessionId = encodeURIComponent(worktreeName);
          const terminalUrl = `/terminal/${sessionId}`;
          
          const conflictText = conflictFiles.length > 0 
            ? `Conflicts in: ${conflictFiles.join(", ")}`
            : "Multiple files have conflicts";

          const claudePrompt = createMergeConflictPrompt("merge", conflictFiles);

          errorHandler.setErrorAlert({
            open: true,
            title: `Merge Conflict in ${worktreeName}`,
            description: `${conflictText}\n\nOpen terminal to resolve: ${terminalUrl}\n\nSuggested Claude prompt: "${claudePrompt}"`
          });
          return false;
        }
        
        errorHandler.setErrorAlert({
          open: true,
          title: "Merge Failed",
          description: `Failed to merge worktree: ${errorData.error}`
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to merge worktree:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Merge Failed",
        description: `Failed to merge worktree: ${error}`
      });
      return false;
    }
  },

  async createWorktreePreview(id: string, errorHandler: ErrorHandler): Promise<boolean> {
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
          description: `Failed to create preview: ${errorData.error}`
        });
        return false;
      }
    } catch (error) {
      console.error("Failed to create preview:", error);
      errorHandler.setErrorAlert({
        open: true,
        title: "Preview Failed",
        description: `Failed to create preview: ${error}`
      });
      return false;
    }
  },

  async fetchBranchesForRepositories(repositories: Record<string, any>): Promise<Record<string, string[]>> {
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
      Promise.all(mergePromises)
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
  }
};