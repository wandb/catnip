import { useState, useEffect, useRef } from "react";
import {
  gitApi,
  type GitStatus,
  type Worktree,
  type Repository,
  type WorktreeDiffStats,
  type PullRequestInfo,
} from "@/lib/git-api";
import {
  generateWorktreeSummary,
  shouldGenerateSummary,
  type WorktreeSummary,
} from "@/lib/worktree-summary";

interface ClaudeSession {
  sessionStartTime?: string | Date;
  sessionEndTime?: string | Date;
  isActive: boolean;
  turnCount: number;
  lastCost: number;
  header?: string;
}

export interface ConflictStatus {
  has_conflicts?: boolean;
  operation?: string;
  worktree_name?: string;
  worktree_path?: string;
  conflict_files?: string[];
  message?: string;
}

export interface GitState {
  gitStatus: GitStatus;
  worktrees: Worktree[];
  repositories: Repository[];
  repoBranches: Record<string, string[]>;
  claudeSessions: Record<string, ClaudeSession>;
  activeSessions: Record<string, ConflictStatus>;
  syncConflicts: Record<string, ConflictStatus>;
  mergeConflicts: Record<string, ConflictStatus>;
  worktreeSummaries: Record<string, WorktreeSummary>;
  diffStats: Record<string, WorktreeDiffStats | undefined>;
  prStatuses: Record<string, PullRequestInfo | undefined>;
  loading: boolean;
  reposLoading: boolean;
  worktreesLoading: boolean;
  diffStatsLoading: boolean;
  // Individual operation loading states
  checkoutLoading: boolean;
  syncingWorktrees: Set<string>;
  mergingWorktrees: Set<string>;
  // Background loading state for new worktrees
  loadingNewWorktrees: boolean;
}

export function useGitState() {
  const [state, setState] = useState<GitState>({
    gitStatus: {},
    worktrees: [],
    repositories: [],
    repoBranches: {},
    claudeSessions: {},
    activeSessions: {},
    syncConflicts: {},
    mergeConflicts: {},
    worktreeSummaries: {},
    diffStats: {},
    prStatuses: {},
    loading: false,
    reposLoading: false,
    worktreesLoading: false,
    diffStatsLoading: false,
    checkoutLoading: false,
    syncingWorktrees: new Set(),
    mergingWorktrees: new Set(),
    loadingNewWorktrees: false,
  });

  // Track previous worktree IDs to detect new ones
  const previousWorktreeIds = useRef<Set<string>>(new Set());

  const fetchGitStatus = async () => {
    try {
      const data = await gitApi.fetchGitStatus();
      setState((prev) => ({ ...prev, gitStatus: data }));

      // Fetch branches for each repository
      if (data.repositories) {
        const branchMap = await gitApi.fetchBranchesForRepositories(
          data.repositories,
        );
        setState((prev) => ({ ...prev, repoBranches: branchMap }));
      }
    } catch (error) {
      console.error("Failed to fetch git status:", error);
    }
  };

  const fetchWorktrees = async () => {
    setState((prev) => ({ ...prev, worktreesLoading: true }));
    try {
      const data = await gitApi.fetchWorktrees();
      setState((prev) => ({ ...prev, worktrees: data }));
    } catch (error) {
      console.error("Failed to fetch worktrees:", error);
    } finally {
      setState((prev) => ({ ...prev, worktreesLoading: false }));
    }
  };

  const fetchRepositories = async () => {
    setState((prev) => ({ ...prev, reposLoading: true }));
    try {
      const data = await gitApi.fetchRepositories();
      setState((prev) => ({ ...prev, repositories: data }));
    } catch (error) {
      console.error("Failed to fetch repositories:", error);
    } finally {
      setState((prev) => ({ ...prev, reposLoading: false }));
    }
  };

  const fetchClaudeSessions = async () => {
    try {
      const data = await gitApi.fetchClaudeSessions();
      setState((prev) => ({ ...prev, claudeSessions: data }));
    } catch (error) {
      console.error("Failed to fetch claude sessions:", error);
    }
  };

  const fetchActiveSessions = async () => {
    try {
      const data = await gitApi.fetchActiveSessions();
      setState((prev) => ({ ...prev, activeSessions: data }));
    } catch (error) {
      console.error("Failed to fetch active sessions:", error);
    }
  };

  const checkConflicts = async () => {
    try {
      const { syncConflicts, mergeConflicts } = await gitApi.checkAllConflicts(
        state.worktrees,
      );
      setState((prev) => ({ ...prev, syncConflicts, mergeConflicts }));
    } catch (error) {
      console.error("Failed to check conflicts:", error);
    }
  };

  const fetchDiffStats = async () => {
    setState((prev) => ({ ...prev, diffStatsLoading: true }));
    try {
      const diffStats = await gitApi.fetchAllDiffStats(state.worktrees);
      setState((prev) => ({ ...prev, diffStats }));
    } catch (error) {
      console.error("Failed to fetch diff stats:", error);
    } finally {
      setState((prev) => ({ ...prev, diffStatsLoading: false }));
    }
  };

  // Fetch PR statuses for all worktrees
  const fetchPrStatuses = async () => {
    try {
      if (state.worktrees.length === 0) {
        setState((prev) => ({ ...prev, prStatuses: {} }));
        return;
      }

      const prPromises = state.worktrees.map(async (worktree) => {
        const prInfo = await gitApi.getPullRequestInfo(worktree.id);
        return { worktreeId: worktree.id, prInfo };
      });

      const prResults = await Promise.all(prPromises);
      const newPrStatuses: Record<string, PullRequestInfo | undefined> = {};

      prResults.forEach(({ worktreeId, prInfo }) => {
        if (prInfo) {
          newPrStatuses[worktreeId] = prInfo;
        }
      });

      setState((prev) => ({ ...prev, prStatuses: newPrStatuses }));
    } catch (error) {
      console.error("Failed to fetch PR statuses:", error);
    }
  };

  // Generate summary for a specific worktree
  const generateWorktreeSummaryForId = async (worktreeId: string) => {
    // Set status to generating
    setState((prev) => ({
      ...prev,
      worktreeSummaries: {
        ...prev.worktreeSummaries,
        [worktreeId]: {
          worktreeId,
          title: "",
          summary: "",
          status: "generating",
        },
      },
    }));

    try {
      const summary = await generateWorktreeSummary(worktreeId);
      setState((prev) => ({
        ...prev,
        worktreeSummaries: {
          ...prev.worktreeSummaries,
          [worktreeId]: summary,
        },
      }));
    } catch (error) {
      console.error(
        `Failed to generate summary for worktree ${worktreeId}:`,
        error,
      );
      setState((prev) => ({
        ...prev,
        worktreeSummaries: {
          ...prev.worktreeSummaries,
          [worktreeId]: {
            worktreeId,
            title: "Failed to generate summary",
            summary: "An error occurred while generating the summary",
            status: "error",
            error: error instanceof Error ? error.message : "Unknown error",
          },
        },
      }));
    }
  };

  // Generate summaries for all qualifying worktrees
  const generateAllWorktreeSummaries = async () => {
    const qualifyingWorktrees = state.worktrees.filter(shouldGenerateSummary);

    // Initialize pending summaries
    const pendingSummaries: Record<string, WorktreeSummary> = {};
    qualifyingWorktrees.forEach((worktree) => {
      if (
        !state.worktreeSummaries[worktree.id] ||
        state.worktreeSummaries[worktree.id].status === "error"
      ) {
        pendingSummaries[worktree.id] = {
          worktreeId: worktree.id,
          title: "",
          summary: "",
          status: "pending",
        };
      }
    });

    if (Object.keys(pendingSummaries).length > 0) {
      setState((prev) => ({
        ...prev,
        worktreeSummaries: {
          ...prev.worktreeSummaries,
          ...pendingSummaries,
        },
      }));

      // Generate summaries in parallel
      const summaryPromises = Object.keys(pendingSummaries).map(
        async (worktreeId) => {
          // Set to generating
          setState((prev) => ({
            ...prev,
            worktreeSummaries: {
              ...prev.worktreeSummaries,
              [worktreeId]: {
                ...prev.worktreeSummaries[worktreeId],
                status: "generating",
              },
            },
          }));

          try {
            const summary = await generateWorktreeSummary(worktreeId);
            setState((prev) => ({
              ...prev,
              worktreeSummaries: {
                ...prev.worktreeSummaries,
                [worktreeId]: summary,
              },
            }));
          } catch (error) {
            console.error(
              `Failed to generate summary for worktree ${worktreeId}:`,
              error,
            );
            setState((prev) => ({
              ...prev,
              worktreeSummaries: {
                ...prev.worktreeSummaries,
                [worktreeId]: {
                  worktreeId,
                  title: "Failed to generate summary",
                  summary: "An error occurred while generating the summary",
                  status: "error",
                  error:
                    error instanceof Error ? error.message : "Unknown error",
                },
              },
            }));
          }
        },
      );

      await Promise.all(summaryPromises);
    }
  };

  // Clear summary for a specific worktree
  const clearWorktreeSummary = (worktreeId: string) => {
    setState((prev) => {
      const newSummaries = { ...prev.worktreeSummaries };
      delete newSummaries[worktreeId];
      return { ...prev, worktreeSummaries: newSummaries };
    });
  };

  // Clear all summaries
  const clearAllWorktreeSummaries = () => {
    setState((prev) => ({ ...prev, worktreeSummaries: {} }));
  };

  const refreshAll = async () => {
    await Promise.all([
      fetchGitStatus(),
      fetchWorktrees(),
      fetchClaudeSessions(),
      fetchActiveSessions(),
    ]);
  };

  const setLoading = (loading: boolean) => {
    setState((prev) => ({ ...prev, loading }));
  };

  // Compute overall loading state
  const computedLoading = state.loading || state.worktreesLoading;

  // Initial fetch
  useEffect(() => {
    void fetchGitStatus();
    void fetchWorktrees();
    void fetchRepositories();
    void fetchClaudeSessions();
    void fetchActiveSessions();
  }, []);

  // Smart effect: only fetch data for truly new worktrees, not on removals/updates
  useEffect(() => {
    if (state.worktrees.length > 0) {
      const currentWorktreeIds = new Set(state.worktrees.map((wt) => wt.id));
      const previousIds = previousWorktreeIds.current;

      // Find truly new worktrees (not in previous set)
      const newWorktreeIds = Array.from(currentWorktreeIds).filter(
        (id) => !previousIds.has(id),
      );

      if (newWorktreeIds.length > 0) {
        // Only fetch data for new worktrees
        Promise.all([
          checkConflicts(),
          // Only fetch diff stats if this is the initial load (no previous worktrees)
          // or if we have specific new worktrees to fetch for
          previousIds.size === 0
            ? fetchDiffStats()
            : Promise.all(
                newWorktreeIds.map((id) => updateWorktreeDiffStats(id)),
              ),
          fetchPrStatuses(),
        ]).catch(console.error);
      } else if (previousIds.size === 0) {
        // Initial load with existing worktrees
        Promise.all([
          checkConflicts(),
          fetchDiffStats(),
          fetchPrStatuses(),
        ]).catch(console.error);
      }

      // Update the previous IDs reference
      previousWorktreeIds.current = currentWorktreeIds;
    }
  }, [state.worktrees]);

  // Generate summaries for qualifying worktrees when they change
  useEffect(() => {
    if (state.worktrees.length > 0) {
      void generateAllWorktreeSummaries();
    }
  }, [state.worktrees]);

  // Background refresh functions that don't affect loading states
  const backgroundRefreshGitStatus = async () => {
    try {
      const data = await gitApi.fetchGitStatus();
      setState((prev) => ({ ...prev, gitStatus: data }));

      // Fetch branches for each repository
      if (data.repositories) {
        const branchMap = await gitApi.fetchBranchesForRepositories(
          data.repositories,
        );
        setState((prev) => ({ ...prev, repoBranches: branchMap }));
      }
    } catch (error) {
      console.error("Failed to fetch git status:", error);
    }
  };

  const backgroundRefreshWorktrees = async () => {
    try {
      const data = await gitApi.fetchWorktrees();
      setState((prev) => ({ ...prev, worktrees: data }));
    } catch (error) {
      console.error("Failed to fetch worktrees:", error);
    }
  };

  const backgroundRefreshClaudeSessions = async () => {
    try {
      const data = await gitApi.fetchClaudeSessions();
      setState((prev) => ({ ...prev, claudeSessions: data }));
    } catch (error) {
      console.error("Failed to fetch claude sessions:", error);
    }
  };

  const backgroundRefreshActiveSessions = async () => {
    try {
      const data = await gitApi.fetchActiveSessions();
      setState((prev) => ({ ...prev, activeSessions: data }));
    } catch (error) {
      console.error("Failed to fetch active sessions:", error);
    }
  };

  // Individual worktree update functions
  const updateWorktree = (updatedWorktree: Worktree) => {
    setState((prev) => ({
      ...prev,
      worktrees: prev.worktrees.map((wt) =>
        wt.id === updatedWorktree.id ? updatedWorktree : wt,
      ),
    }));
  };

  const addWorktree = (newWorktree: Worktree) => {
    setState((prev) => ({
      ...prev,
      worktrees: [...prev.worktrees, newWorktree],
    }));
  };

  const removeWorktree = (worktreeId: string) => {
    setState((prev) => ({
      ...prev,
      worktrees: prev.worktrees.filter((wt) => wt.id !== worktreeId),
      // Clean up related state
      syncConflicts: Object.fromEntries(
        Object.entries(prev.syncConflicts).filter(([id]) => id !== worktreeId),
      ),
      mergeConflicts: Object.fromEntries(
        Object.entries(prev.mergeConflicts).filter(([id]) => id !== worktreeId),
      ),
      diffStats: Object.fromEntries(
        Object.entries(prev.diffStats).filter(([id]) => id !== worktreeId),
      ),
      prStatuses: Object.fromEntries(
        Object.entries(prev.prStatuses).filter(([id]) => id !== worktreeId),
      ),
      worktreeSummaries: Object.fromEntries(
        Object.entries(prev.worktreeSummaries).filter(
          ([id]) => id !== worktreeId,
        ),
      ),
    }));
  };

  // Update conflicts for a specific worktree
  const updateWorktreeConflicts = async (worktreeId: string) => {
    try {
      const [syncConflict, mergeConflict] = await Promise.all([
        gitApi.checkSyncConflicts(worktreeId),
        gitApi.checkMergeConflicts(worktreeId),
      ]);

      setState((prev) => ({
        ...prev,
        syncConflicts: {
          ...prev.syncConflicts,
          ...(syncConflict && { [worktreeId]: syncConflict }),
        },
        mergeConflicts: {
          ...prev.mergeConflicts,
          ...(mergeConflict && { [worktreeId]: mergeConflict }),
        },
      }));
    } catch (error) {
      console.error(`Failed to update conflicts for ${worktreeId}:`, error);
    }
  };

  // Update diff stats for a specific worktree
  const updateWorktreeDiffStats = async (worktreeId: string) => {
    try {
      const diffStat = await gitApi.fetchWorktreeDiffStats(worktreeId);
      setState((prev) => ({
        ...prev,
        diffStats: {
          ...prev.diffStats,
          [worktreeId]: diffStat || undefined,
        },
      }));
    } catch (error) {
      console.error(`Failed to update diff stats for ${worktreeId}:`, error);
    }
  };

  // Update PR status for a specific worktree
  const updateWorktreePrStatus = async (worktreeId: string) => {
    try {
      const prInfo = await gitApi.getPullRequestInfo(worktreeId);
      setState((prev) => ({
        ...prev,
        prStatuses: {
          ...prev.prStatuses,
          [worktreeId]: prInfo || undefined,
        },
      }));
    } catch (error) {
      console.error(`Failed to update PR status for ${worktreeId}:`, error);
    }
  };

  // Comprehensive refresh for a specific worktree (after sync/merge operations)
  const refreshWorktree = async (
    worktreeId: string,
    options: { includeDiffs?: boolean } = {},
  ) => {
    try {
      // Find and update the specific worktree
      const updatedWorktrees = await gitApi.fetchWorktrees();
      const updatedWorktree = updatedWorktrees.find(
        (wt) => wt.id === worktreeId,
      );

      if (updatedWorktree) {
        updateWorktree(updatedWorktree);
      }

      // Update related data for this worktree
      const updatePromises = [
        updateWorktreeConflicts(worktreeId),
        updateWorktreePrStatus(worktreeId),
      ];

      // Only update diff stats if explicitly requested (after operations that change code)
      if (options.includeDiffs) {
        updatePromises.push(updateWorktreeDiffStats(worktreeId));
      }

      await Promise.all(updatePromises);
    } catch (error) {
      console.error(`Failed to refresh worktree ${worktreeId}:`, error);
    }
  };

  // Add new worktrees incrementally (for checkout operations)
  const addNewWorktrees = async () => {
    setLoadingNewWorktrees(true);
    try {
      const currentWorktreeIds = new Set(state.worktrees.map((wt) => wt.id));
      const allWorktrees = await gitApi.fetchWorktrees();

      // Find new worktrees
      const newWorktrees = allWorktrees.filter(
        (wt) => !currentWorktreeIds.has(wt.id),
      );

      // Add them incrementally and stop loading immediately
      newWorktrees.forEach((newWorktree) => {
        addWorktree(newWorktree);
      });

      // Stop loading as soon as we have the worktrees
      setLoadingNewWorktrees(false);

      // Fetch additional data in background without loading indicator
      if (newWorktrees.length > 0) {
        Promise.all(
          newWorktrees.map(async (worktree) => {
            await Promise.all([
              updateWorktreeConflicts(worktree.id),
              updateWorktreeDiffStats(worktree.id), // Fetch diffs for new worktrees
              updateWorktreePrStatus(worktree.id),
            ]);
          }),
        ).catch(console.error);
      }

      return newWorktrees;
    } catch (error) {
      console.error("Failed to add new worktrees:", error);
      return [];
    } finally {
      // Ensure loading is stopped even on error
      setLoadingNewWorktrees(false);
    }
  };

  // Operation-specific loading state management
  const setCheckoutLoading = (loading: boolean) => {
    setState((prev) => ({ ...prev, checkoutLoading: loading }));
  };

  const setLoadingNewWorktrees = (loading: boolean) => {
    setState((prev) => ({ ...prev, loadingNewWorktrees: loading }));
  };

  const setSyncingWorktree = (worktreeId: string, syncing: boolean) => {
    setState((prev) => {
      const newSyncingWorktrees = new Set(prev.syncingWorktrees);
      if (syncing) {
        newSyncingWorktrees.add(worktreeId);
      } else {
        newSyncingWorktrees.delete(worktreeId);
      }
      return { ...prev, syncingWorktrees: newSyncingWorktrees };
    });
  };

  const setMergingWorktree = (worktreeId: string, merging: boolean) => {
    setState((prev) => {
      const newMergingWorktrees = new Set(prev.mergingWorktrees);
      if (merging) {
        newMergingWorktrees.add(worktreeId);
      } else {
        newMergingWorktrees.delete(worktreeId);
      }
      return { ...prev, mergingWorktrees: newMergingWorktrees };
    });
  };

  // Force refresh diff stats for a specific worktree (after code changes)
  const refreshWorktreeDiffStats = async (worktreeId: string) => {
    try {
      await updateWorktreeDiffStats(worktreeId);
    } catch (error) {
      console.error(`Failed to refresh diff stats for ${worktreeId}:`, error);
    }
  };

  return {
    ...state,
    loading: computedLoading,
    fetchGitStatus,
    fetchWorktrees,
    fetchRepositories,
    fetchClaudeSessions,
    fetchActiveSessions,
    checkConflicts,
    fetchDiffStats,
    fetchPrStatuses,
    generateWorktreeSummaryForId,
    generateAllWorktreeSummaries,
    clearWorktreeSummary,
    clearAllWorktreeSummaries,
    refreshAll,
    setLoading,
    // New incremental update functions
    backgroundRefreshGitStatus,
    backgroundRefreshWorktrees,
    backgroundRefreshClaudeSessions,
    backgroundRefreshActiveSessions,
    updateWorktree,
    addWorktree,
    removeWorktree,
    updateWorktreeConflicts,
    updateWorktreeDiffStats,
    updateWorktreePrStatus,
    refreshWorktree,
    addNewWorktrees,
    setCheckoutLoading,
    setLoadingNewWorktrees,
    setSyncingWorktree,
    setMergingWorktree,
    refreshWorktreeDiffStats,
  };
}
