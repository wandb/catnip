import { useCallback } from "react";
import { toast } from "sonner";
import { useGitOperations } from "./useGitApi";
import { handleMergeConflict, createPreviewToast, createPullRequestToast } from "@/lib/worktreeActions";
import { Worktree } from "@/types/git";

export const useConflictHandling = (
  showErrorAlert: (title: string, description: string) => void,
  refreshData: () => void
) => {
  const gitOps = useGitOperations();

  const checkConflicts = useCallback(async (worktrees: Worktree[]) => {
    if (worktrees.length === 0) return { syncConflicts: {}, mergeConflicts: {} };

    const syncConflictPromises = worktrees.map(async (worktree) => {
      try {
        const data = await gitOps.checkSyncConflicts(worktree.id);
        return { worktreeId: worktree.id, data };
      } catch (error) {
        console.error(`Failed to check sync conflicts for ${worktree.id}:`, error);
        return { worktreeId: worktree.id, data: null };
      }
    });

    const mergeConflictPromises = worktrees.map(async (worktree) => {
      if (!worktree.repo_id.startsWith("local/")) {
        return { worktreeId: worktree.id, data: null };
      }
      
      try {
        const data = await gitOps.checkMergeConflicts(worktree.id);
        return { worktreeId: worktree.id, data };
      } catch (error) {
        console.error(`Failed to check merge conflicts for ${worktree.id}:`, error);
        return { worktreeId: worktree.id, data: null };
      }
    });

    const [syncResults, mergeResults] = await Promise.all([
      Promise.all(syncConflictPromises),
      Promise.all(mergeConflictPromises)
    ]);

    const newSyncConflicts: Record<string, any> = {};
    syncResults.forEach(({ worktreeId, data }) => {
      if (data) {
        newSyncConflicts[worktreeId] = data;
      }
    });

    const newMergeConflicts: Record<string, any> = {};
    mergeResults.forEach(({ worktreeId, data }) => {
      if (data) {
        newMergeConflicts[worktreeId] = data;
      }
    });

    return { syncConflicts: newSyncConflicts, mergeConflicts: newMergeConflicts };
  }, [gitOps]);

  const syncWorktree = useCallback(async (id: string) => {
    try {
      await gitOps.syncWorktree(id);
      refreshData();
      toast.success("Successfully synced worktree");
    } catch (error: any) {
      const conflictInfo = handleMergeConflict(error, "sync");
      if (conflictInfo) {
        showErrorAlert(conflictInfo.title, conflictInfo.description);
      } else {
        showErrorAlert("Sync Failed", `Failed to sync worktree: ${error.error || error}`);
      }
    }
  }, [gitOps, refreshData, showErrorAlert]);

  const mergeWorktree = useCallback(async (id: string, worktreeName: string, squash: boolean = true) => {
    try {
      await gitOps.mergeWorktree(id, squash);
      refreshData();
      const mergeType = squash ? "squash merged" : "merged";
      toast.success(`Successfully ${mergeType} ${worktreeName} to main branch`);
    } catch (error: any) {
      const conflictInfo = handleMergeConflict(error, "merge");
      if (conflictInfo) {
        showErrorAlert(conflictInfo.title, conflictInfo.description);
      } else {
        showErrorAlert("Merge Failed", `Failed to merge worktree: ${error.error || error}`);
      }
    }
  }, [gitOps, refreshData, showErrorAlert]);

  const createPreview = useCallback(async (id: string, branchName: string) => {
    try {
      await gitOps.createPreview(id);
      toast.success(createPreviewToast(branchName), { duration: 8000 });
    } catch (error: any) {
      showErrorAlert("Preview Failed", `Failed to create preview: ${error.error || error}`);
    }
  }, [gitOps, showErrorAlert]);

  const createPullRequest = useCallback(async (id: string, title: string, body: string) => {
    try {
      const prData = await gitOps.createPullRequest(id, title, body);
      toast.success(createPullRequestToast(prData), { duration: 10000 });
      return true;
    } catch (error: any) {
      showErrorAlert("Pull Request Failed", `Failed to create pull request: ${error.error || error}`);
      return false;
    }
  }, [gitOps, showErrorAlert]);

  const deleteWorktree = useCallback(async (id: string) => {
    try {
      await gitOps.deleteWorktree(id);
      refreshData();
      toast.success("Worktree deleted successfully");
    } catch (error: any) {
      showErrorAlert("Delete Failed", `Failed to delete worktree: ${error.error || error}`);
    }
  }, [gitOps, refreshData, showErrorAlert]);

  return {
    checkConflicts,
    syncWorktree,
    mergeWorktree,
    createPreview,
    createPullRequest,
    deleteWorktree,
  };
};