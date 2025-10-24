import { toast } from "sonner";
import { fetchWithTimeout, TimeoutError } from "./fetch-with-timeout";

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

export interface Todo {
  id: string;
  content: string;
  status: "pending" | "in_progress" | "completed";
  priority?: "high" | "medium" | "low";
}

export interface DirtyFile {
  path: string;
  status: string; // M, A, D, R, etc.
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
  dirty_files?: DirtyFile[];
  created_at: string;
  last_accessed: string;
  session_title?: TitleEntry;
  session_title_history?: TitleEntry[];
  cache_status?: CacheStatus;
  has_active_claude_session?: boolean;
  claude_activity_state: "inactive" | "running" | "active";
  pull_request_url?: string;
  pull_request_title?: string;
  pull_request_body?: string;
  pull_request_state?: string;
  pull_request_last_synced?: string;
  todos?: Todo[];
  latest_claude_message?: string;
  latest_claude_message_type?: string;
  latest_claude_message_timestamp?: number;
  latest_user_prompt?: string;
  latest_session_title?: string;
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
  available: boolean;
  remote_origin?: string;
  has_github_remote?: boolean;
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

export interface AppSettings {
  catnipProxy?: string;
  authRequired: boolean;
  codespaceName?: string;
  isCodespace: boolean;
}

export const gitApi = {
  // STATE FETCHING METHODS
  // Note: These methods are used internally by the appStore for initial data loading.
  // Components should use the zustand store (useAppStore) directly for state access.

  async fetchGitStatus(): Promise<GitStatus> {
    try {
      const response = await fetchWithTimeout("/v1/git/status", {
        timeout: 30000,
      });
      if (response.ok) {
        return await response.json();
      }
      throw new Error("Failed to fetch git status");
    } catch (error) {
      if (error instanceof TimeoutError) {
        console.error("Git status request timed out");
        throw new Error(
          "Request timed out. The backend server may be unavailable.",
        );
      }
      throw error;
    }
  },

  async fetchWorktrees(): Promise<Worktree[]> {
    try {
      const response = await fetchWithTimeout("/v1/git/worktrees", {
        timeout: 30000,
      });
      if (response.ok) {
        return await response.json();
      }
      throw new Error("Failed to fetch worktrees");
    } catch (error) {
      if (error instanceof TimeoutError) {
        console.error("Worktrees request timed out");
        throw new Error(
          "Request timed out. The backend server may be unavailable.",
        );
      }
      throw error;
    }
  },

  async fetchRepositories(): Promise<Repository[]> {
    try {
      const response = await fetchWithTimeout("/v1/git/github/repos", {
        timeout: 30000,
      });
      if (response.ok) {
        return await response.json();
      }
      throw new Error("Failed to fetch repositories");
    } catch (error) {
      if (error instanceof TimeoutError) {
        console.error("Repositories request timed out");
        throw new Error(
          "Request timed out. The backend server may be unavailable.",
        );
      }
      throw error;
    }
  },

  async fetchSettings(): Promise<AppSettings> {
    try {
      const response = await fetchWithTimeout("/v1/settings", {
        timeout: 10000,
      });
      if (response.ok) {
        return await response.json();
      }
      throw new Error("Failed to fetch settings");
    } catch (error) {
      if (error instanceof TimeoutError) {
        console.error("Settings request timed out");
        throw new Error(
          "Request timed out. The backend server may be unavailable.",
        );
      }
      throw error;
    }
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

  async fetchWorktreeLatestAssistantMessage(
    worktreePath: string,
  ): Promise<string> {
    try {
      const response = await fetch(
        `/v1/claude/latest-message?worktree_path=${encodeURIComponent(worktreePath)}`,
      );
      if (response.ok) {
        const result = await response.json();
        return result.message || "";
      }
      return "";
    } catch (error) {
      console.error("Failed to fetch latest assistant message:", error);
      return "";
    }
  },

  async fetchWorktreeLatestMessageOrError(
    worktreePath: string,
  ): Promise<{ content: string; isError: boolean }> {
    try {
      const response = await fetch(
        `/v1/claude/latest-message?worktree_path=${encodeURIComponent(worktreePath)}`,
      );
      if (response.ok) {
        const result = await response.json();
        return {
          content: result.content || "",
          isError: result.isError || false,
        };
      }
      return { content: "", isError: false };
    } catch (error) {
      console.error("Failed to fetch latest message or error:", error);
      return { content: "", isError: false };
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

  async startPTY(
    workspacePath: string,
    agent: string = "claude",
  ): Promise<void> {
    try {
      const response = await fetch(
        `/v1/pty/start?session=${encodeURIComponent(workspacePath)}&agent=${encodeURIComponent(agent)}`,
        {
          method: "POST",
        },
      );
      if (!response.ok) {
        throw new Error("Failed to start PTY");
      }
    } catch (error) {
      console.error("Failed to start PTY:", error);
      throw error;
    }
  },

  async sendPromptToPTY(
    workspacePath: string,
    prompt: string,
    agent: string = "claude",
  ): Promise<void> {
    try {
      const response = await fetch(
        `/v1/pty/prompt?session=${encodeURIComponent(workspacePath)}&agent=${encodeURIComponent(agent)}`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ prompt }),
        },
      );

      if (response.status === 408) {
        throw new Error("PTY_TIMEOUT");
      }

      if (!response.ok) {
        throw new Error("Failed to send prompt to PTY");
      }
    } catch (error) {
      if (error instanceof Error && error.message === "PTY_TIMEOUT") {
        throw error;
      }
      console.error("Failed to send prompt to PTY:", error);
      throw error;
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

  // MUTATION OPERATIONS
  // These methods perform server-side operations and are used by the useGitApi hook.

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

  // NOTE: Conflict checking and batch diff stats removed
  // Conflicts are now tracked via SSE events in worktree.has_conflicts
  // Individual diff stats still available via fetchWorktreeDiffStats if needed

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

  async createFromTemplate(
    templateId: string,
    projectName: string,
    errorHandler: (error: Error) => void,
  ): Promise<{ success: boolean; worktreeName?: string }> {
    try {
      const response = await fetch("/v1/git/template", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          template_id: templateId,
          project_name: projectName,
        }),
      });

      if (response.ok) {
        const data = await response.json();
        return {
          success: true,
          worktreeName: data.worktree_name,
        };
      }

      if (response.status === 400) {
        const error = await response.json();
        errorHandler(new Error(error.error || "Invalid template request"));
        return { success: false };
      }

      errorHandler(
        new Error(`Failed to create from template: ${response.statusText}`),
      );
      return { success: false };
    } catch (error) {
      console.error("Error creating from template:", error);
      errorHandler(error instanceof Error ? error : new Error("Network error"));
      return { success: false };
    }
  },

  async createGitHubRepository(
    repoId: string,
    name: string,
    description: string,
    isPrivate: boolean,
  ): Promise<{ url: string; message: string }> {
    const response = await fetch(
      `/v1/git/repositories/${encodeURIComponent(repoId)}/github`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          name,
          description,
          is_private: isPrivate,
        }),
      },
    );

    if (!response.ok) {
      const errorData = await response.json();
      throw new Error(errorData.error || "Failed to create GitHub repository");
    }

    return await response.json();
  },
};
