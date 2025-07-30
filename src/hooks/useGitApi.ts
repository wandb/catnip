import { useState } from "react";
import { gitApi, type ErrorHandler } from "@/lib/git-api";
import { useAppStore } from "@/stores/appStore";
import { toast } from "sonner";

/**
 * Hook that provides git operations (mutations) while using the central zustand store for state.
 * This replaces direct git-api calls and works with the centralized state management.
 */
export function useGitApi() {
  const { updateWorktree, getWorktreeById } = useAppStore();

  // Operation-specific loading states
  const [syncingWorktrees, setSyncingWorktrees] = useState<Set<string>>(
    new Set(),
  );
  const [mergingWorktrees, setMergingWorktrees] = useState<Set<string>>(
    new Set(),
  );
  const [checkoutLoading, setCheckoutLoading] = useState(false);

  // Helper to update worktree loading state
  const setSyncingWorktree = (worktreeId: string, syncing: boolean) => {
    setSyncingWorktrees((prev) => {
      const newSet = new Set(prev);
      if (syncing) {
        newSet.add(worktreeId);
      } else {
        newSet.delete(worktreeId);
      }
      return newSet;
    });
  };

  const setMergingWorktree = (worktreeId: string, merging: boolean) => {
    setMergingWorktrees((prev) => {
      const newSet = new Set(prev);
      if (merging) {
        newSet.add(worktreeId);
      } else {
        newSet.delete(worktreeId);
      }
      return newSet;
    });
  };

  // Sync worktree operation
  const syncWorktree = async (
    worktreeId: string,
    errorHandler: ErrorHandler,
  ): Promise<boolean> => {
    setSyncingWorktree(worktreeId, true);
    try {
      const success = await gitApi.syncWorktree(worktreeId, errorHandler);

      if (success) {
        // Update worktree in store to reflect sync
        const worktree = getWorktreeById(worktreeId);
        if (worktree) {
          updateWorktree(worktreeId, {
            ...worktree,
            commits_behind: 0, // Synced, so no longer behind
            has_conflicts: false, // Sync resolved conflicts
          });
        }
      }

      return success;
    } finally {
      setSyncingWorktree(worktreeId, false);
    }
  };

  // Merge worktree operation
  const mergeWorktree = async (
    worktreeId: string,
    worktreeName: string,
    squash: boolean,
    errorHandler: ErrorHandler,
    autoCleanup = true,
  ): Promise<boolean> => {
    setMergingWorktree(worktreeId, true);
    try {
      const success = await gitApi.mergeWorktree(
        worktreeId,
        worktreeName,
        squash,
        errorHandler,
        autoCleanup,
      );

      // Note: If autoCleanup is true, the worktree will be deleted via SSE events
      // If not, we might need to update its state

      return success;
    } finally {
      setMergingWorktree(worktreeId, false);
    }
  };

  // Delete worktree operation
  const deleteWorktree = async (worktreeId: string): Promise<boolean> => {
    try {
      await gitApi.deleteWorktree(worktreeId);
      // Note: Worktree removal from store will be handled by SSE event
      toast.success("Worktree deleted successfully");
      return true;
    } catch (error) {
      console.error("Failed to delete worktree:", error);
      toast.error("Failed to delete worktree");
      return false;
    }
  };

  // Create worktree preview
  const createWorktreePreview = async (
    worktreeId: string,
    errorHandler: ErrorHandler,
  ): Promise<boolean> => {
    return gitApi.createWorktreePreview(worktreeId, errorHandler);
  };

  // Pull request operations
  const createPullRequest = async (
    worktreeId: string,
    title: string,
    body: string,
    errorHandler: ErrorHandler,
  ): Promise<boolean> => {
    const success = await gitApi.createPullRequest(
      worktreeId,
      title,
      body,
      errorHandler,
    );

    if (success) {
      // Refresh PR info for this worktree (will be updated via events)
      // The actual PR URL will come through SSE events
    }

    return success;
  };

  const updatePullRequest = async (
    worktreeId: string,
    title: string,
    body: string,
    errorHandler: ErrorHandler,
  ): Promise<boolean> => {
    return gitApi.updatePullRequest(worktreeId, title, body, errorHandler);
  };

  // Get PR info (still needed for components that need immediate data)
  const getPullRequestInfo = async (worktreeId: string) => {
    return gitApi.getPullRequestInfo(worktreeId);
  };

  // Checkout repository (creates new worktree)
  const checkoutRepository = async (
    org: string,
    repo: string,
    branch: string = "main",
  ): Promise<boolean> => {
    setCheckoutLoading(true);
    try {
      // Build URL with optional branch query parameter
      const url = new URL(
        `/v1/git/checkout/${encodeURIComponent(org)}/${encodeURIComponent(repo)}`,
        window.location.origin,
      );
      if (branch && branch !== "main") {
        url.searchParams.set("branch", branch);
      }

      const response = await fetch(url.toString(), {
        method: "POST",
      });

      if (response.ok) {
        // New worktree will be added to store via SSE events
        toast.success(`Successfully checked out ${org}/${repo}:${branch}`);
        return true;
      } else {
        const errorData = await response.json();
        toast.error(`Failed to checkout: ${errorData.error}`);
        return false;
      }
    } catch (error) {
      console.error("Checkout failed:", error);
      toast.error("Checkout failed");
      return false;
    } finally {
      setCheckoutLoading(false);
    }
  };

  return {
    // Operations
    syncWorktree,
    mergeWorktree,
    deleteWorktree,
    createWorktreePreview,
    createPullRequest,
    updatePullRequest,
    getPullRequestInfo,
    checkoutRepository,

    // Loading states
    syncingWorktrees,
    mergingWorktrees,
    checkoutLoading,

    // Helpers to check individual worktree loading states
    isSyncing: (worktreeId: string) => syncingWorktrees.has(worktreeId),
    isMerging: (worktreeId: string) => mergingWorktrees.has(worktreeId),
  };
}
