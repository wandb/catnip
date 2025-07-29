import { useCallback } from "react";
import { useAppStore } from "@/stores/appStore";

/**
 * Hook that provides worktree-specific computed state and helpers.
 * Uses the central zustand store for state - no more REST API polling!
 * All worktree data is kept up-to-date via SSE events.
 */
export function useWorktreeActions() {
  const { getWorktreesList, getWorktreeById } = useAppStore();

  // Computed getters that work directly with store state
  const getDirtyWorktrees = useCallback(() => {
    return getWorktreesList().filter((wt) => wt.is_dirty);
  }, [getWorktreesList]);

  const getWorktreesWithConflicts = useCallback(() => {
    return getWorktreesList().filter((wt) => wt.has_conflicts);
  }, [getWorktreesList]);

  const getWorktreesByRepo = useCallback(
    (repoId: string) => {
      return getWorktreesList().filter((wt) => wt.repo_id === repoId);
    },
    [getWorktreesList],
  );

  const getWorktreesWithPullRequests = useCallback(() => {
    return getWorktreesList().filter((wt) => wt.pull_request_url);
  }, [getWorktreesList]);

  const getWorktreesBehindMain = useCallback(() => {
    return getWorktreesList().filter((wt) => wt.commits_behind > 0);
  }, [getWorktreesList]);

  const getWorktreesWithCommitsAhead = useCallback(() => {
    return getWorktreesList().filter((wt) => wt.commit_count > 0);
  }, [getWorktreesList]);

  // Individual worktree state checkers
  const isWorktreeDirty = useCallback(
    (worktreeId: string) => {
      const worktree = getWorktreeById(worktreeId);
      return worktree?.is_dirty || false;
    },
    [getWorktreeById],
  );

  const hasWorktreeConflicts = useCallback(
    (worktreeId: string) => {
      const worktree = getWorktreeById(worktreeId);
      return worktree?.has_conflicts || false;
    },
    [getWorktreeById],
  );

  const getWorktreeDirtyFiles = useCallback(
    (worktreeId: string) => {
      const worktree = getWorktreeById(worktreeId);
      return worktree?.dirty_files || [];
    },
    [getWorktreeById],
  );

  const getWorktreePullRequestUrl = useCallback(
    (worktreeId: string) => {
      const worktree = getWorktreeById(worktreeId);
      return worktree?.pull_request_url;
    },
    [getWorktreeById],
  );

  const getWorktreeCommitInfo = useCallback(
    (worktreeId: string) => {
      const worktree = getWorktreeById(worktreeId);
      if (!worktree) return null;

      return {
        commit_hash: worktree.commit_hash,
        commit_count: worktree.commit_count,
        commits_behind: worktree.commits_behind,
        branch: worktree.branch,
        source_branch: worktree.source_branch,
      };
    },
    [getWorktreeById],
  );

  // Summary/statistics helpers
  const getWorktreeStats = useCallback(() => {
    const worktrees = getWorktreesList();
    return {
      total: worktrees.length,
      dirty: worktrees.filter((wt) => wt.is_dirty).length,
      withConflicts: worktrees.filter((wt) => wt.has_conflicts).length,
      withPullRequests: worktrees.filter((wt) => wt.pull_request_url).length,
      behindMain: worktrees.filter((wt) => wt.commits_behind > 0).length,
      withCommitsAhead: worktrees.filter((wt) => wt.commit_count > 0).length,
    };
  }, [getWorktreesList]);

  return {
    // Filtered lists of worktrees
    getDirtyWorktrees,
    getWorktreesWithConflicts,
    getWorktreesByRepo,
    getWorktreesWithPullRequests,
    getWorktreesBehindMain,
    getWorktreesWithCommitsAhead,

    // Individual worktree state checkers
    isWorktreeDirty,
    hasWorktreeConflicts,
    getWorktreeDirtyFiles,
    getWorktreePullRequestUrl,
    getWorktreeCommitInfo,

    // Summary helpers
    getWorktreeStats,
  };
}
