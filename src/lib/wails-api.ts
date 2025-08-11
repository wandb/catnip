// Wails API wrapper for replacing HTTP calls with direct Wails service calls
// This provides a clean interface to the generated Wails bindings

import * as ClaudeDesktopService from "../bindings/github.com/vanpelt/catnip/cmd/desktop/claudedesktopservice.js";
import * as GitDesktopService from "../bindings/github.com/vanpelt/catnip/cmd/desktop/gitdesktopservice.js";
import * as SessionDesktopService from "../bindings/github.com/vanpelt/catnip/cmd/desktop/sessiondesktopservice.js";
import * as SettingsDesktopService from "../bindings/github.com/vanpelt/catnip/cmd/desktop/settingsdesktopservice.js";

// Re-export models for type definitions
export * from "../bindings/github.com/vanpelt/catnip/internal/models/models.js";

// Use any for types since generated bindings don't have TypeScript definitions
type GitStatus = any;
type Repository = any;
type Worktree = any;
type ClaudeSettings = any;
type ClaudeSettingsUpdateRequest = any;
type CreateCompletionRequest = any;
type CreateCompletionResponse = any;
type FullSessionData = any;
type ClaudeSessionSummary = any;
type Todo = any;
type ClaudeActivityState = any;

/**
 * Wails API wrapper providing a clean interface to backend services
 * This replaces HTTP-based API calls with direct Wails method calls
 */
export const wailsApi = {
  // =============================================================================
  // GIT OPERATIONS
  // =============================================================================

  git: {
    /**
     * Get overall Git status including all repositories
     */
    async getStatus(): Promise<GitStatus | null> {
      return await GitDesktopService.GetGitStatus();
    },

    /**
     * Get all Git worktrees
     */
    async getWorktrees(): Promise<(Worktree | null)[]> {
      return await GitDesktopService.GetAllWorktrees();
    },

    /**
     * Get a specific worktree by ID
     */
    async getWorktree(worktreeId: string): Promise<Worktree | null> {
      return await GitDesktopService.GetWorktree(worktreeId);
    },

    /**
     * Get all repositories (GitHub repositories)
     */
    async getRepositories(): Promise<(Repository | null)[]> {
      return await GitDesktopService.GetRepositories();
    },

    /**
     * Create a new worktree
     */
    async createWorktree(
      repoId: string,
      branch: string,
      directory: string,
    ): Promise<Worktree | null> {
      return await GitDesktopService.CreateWorktree(repoId, branch, directory);
    },

    /**
     * Delete a worktree
     */
    async deleteWorktree(worktreeId: string): Promise<void> {
      return await GitDesktopService.DeleteWorktree(worktreeId);
    },
  },

  // =============================================================================
  // CLAUDE OPERATIONS
  // =============================================================================

  claude: {
    /**
     * Get current Claude settings
     */
    async getSettings(): Promise<ClaudeSettings | null> {
      return await ClaudeDesktopService.GetClaudeSettings();
    },

    /**
     * Update Claude settings
     */
    async updateSettings(
      request: ClaudeSettingsUpdateRequest,
    ): Promise<ClaudeSettings | null> {
      return await ClaudeDesktopService.UpdateClaudeSettings(request);
    },

    /**
     * Create a completion request to Claude
     */
    async createCompletion(
      request: CreateCompletionRequest,
    ): Promise<CreateCompletionResponse | null> {
      return await ClaudeDesktopService.CreateCompletion(request);
    },

    /**
     * Get all session summaries for all worktrees
     */
    async getAllSessionSummaries(): Promise<{
      [worktreePath: string]: ClaudeSessionSummary | null;
    }> {
      return await ClaudeDesktopService.GetAllWorktreeSessionSummaries();
    },

    /**
     * Get session summary for a specific worktree
     */
    async getWorktreeSessionSummary(
      worktreePath: string,
    ): Promise<ClaudeSessionSummary | null> {
      return await ClaudeDesktopService.GetWorktreeSessionSummary(worktreePath);
    },

    /**
     * Get complete session data with all messages
     */
    async getFullSessionData(
      worktreePath: string,
      includeFullData = false,
    ): Promise<FullSessionData | null> {
      return await ClaudeDesktopService.GetFullSessionData(
        worktreePath,
        includeFullData,
      );
    },

    /**
     * Get the latest todos from a session
     */
    async getLatestTodos(worktreePath: string): Promise<Todo[]> {
      return await ClaudeDesktopService.GetLatestTodos(worktreePath);
    },
  },

  // =============================================================================
  // SESSION OPERATIONS
  // =============================================================================

  session: {
    /**
     * Get active session for a workspace directory
     */
    async getActiveSession(workspaceDir: string): Promise<[any, boolean]> {
      return await SessionDesktopService.GetActiveSession(workspaceDir);
    },

    /**
     * Get Claude activity state for a directory
     */
    async getClaudeActivityState(
      workDir: string,
    ): Promise<ClaudeActivityState> {
      return await SessionDesktopService.GetClaudeActivityState(workDir);
    },

    /**
     * Start an active session
     */
    async startActiveSession(
      workspaceDir: string,
      claudeSessionUUID: string,
    ): Promise<void> {
      return await SessionDesktopService.StartActiveSession(
        workspaceDir,
        claudeSessionUUID,
      );
    },

    /**
     * Update session title
     */
    async updateSessionTitle(
      workspaceDir: string,
      title: string,
      commitHash: string,
    ): Promise<void> {
      return await SessionDesktopService.UpdateSessionTitle(
        workspaceDir,
        title,
        commitHash,
      );
    },
  },

  // =============================================================================
  // SETTINGS OPERATIONS
  // =============================================================================

  settings: {
    /**
     * Get basic app information
     */
    async getAppInfo(): Promise<{ [key: string]: any }> {
      return await SettingsDesktopService.GetAppInfo();
    },

    /**
     * Get current desktop app settings
     */
    async getAppSettings(): Promise<any> {
      return await SettingsDesktopService.GetAppSettings();
    },

    /**
     * Update desktop app settings
     */
    async updateAppSettings(settings: any): Promise<void> {
      return await SettingsDesktopService.UpdateAppSettings(settings);
    },
  },
};

// =============================================================================
// UTILITY FUNCTIONS
// =============================================================================

/**
 * Convert Wails worktrees array to the format expected by the existing code
 */
export function convertWailsWorktreesToMap(
  worktrees: (Worktree | null)[],
): Map<string, Worktree> {
  const worktreeMap = new Map<string, Worktree>();

  worktrees.forEach((worktree) => {
    if (worktree) {
      // Ensure cache status is present for compatibility
      const enhancedWorktree = {
        ...worktree,
        cache_status: {
          is_cached: true,
          is_loading: false,
          last_updated: Date.now(),
        },
      };
      worktreeMap.set(worktree.id, enhancedWorktree);
    }
  });

  return worktreeMap;
}

/**
 * Convert Wails repositories array to the format expected by the existing code
 */
export function convertWailsRepositoriesToMap(
  repositories: (Repository | null)[],
): Map<string, Repository> {
  const repositoryMap = new Map<string, Repository>();

  repositories.forEach((repo) => {
    if (repo) {
      repositoryMap.set(repo.id, repo);
    }
  });

  return repositoryMap;
}

/**
 * Convert Wails GitStatus to the format expected by the existing code
 */
export function convertWailsGitStatus(gitStatus: GitStatus | null): any {
  if (!gitStatus) {
    return {};
  }

  return {
    repositories: gitStatus.repositories || {},
    worktree_count: gitStatus.worktree_count || 0,
  };
}

/**
 * Helper function to handle API errors consistently
 */
export function handleWailsError(error: any): Error {
  if (error instanceof Error) {
    return error;
  }

  if (typeof error === "string") {
    return new Error(error);
  }

  if (error && typeof error === "object" && error.message) {
    return new Error(error.message);
  }

  return new Error("An unknown error occurred");
}

/**
 * Wrapper for async operations with error handling
 */
export async function wailsCall<T>(
  operation: () => Promise<T>,
  fallback?: T,
): Promise<T> {
  try {
    return await operation();
  } catch (error) {
    console.error("Wails API call failed:", error);

    if (fallback !== undefined) {
      return fallback;
    }

    throw handleWailsError(error);
  }
}

// =============================================================================
// MIGRATION HELPERS
// =============================================================================

/**
 * Check if we're running in Wails environment
 */
export function isWailsEnvironment(): boolean {
  return typeof window !== "undefined" && "go" in window;
}

/**
 * Fallback HTTP fetch for development/testing when not in Wails environment
 * This allows the same code to work in both environments
 */
export async function fetchWithWailsFallback(
  url: string,
  options?: RequestInit,
): Promise<Response> {
  if (isWailsEnvironment()) {
    throw new Error("Use Wails API instead of HTTP fetch in Wails environment");
  }

  return fetch(url, options);
}

export default wailsApi;
