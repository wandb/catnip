import {
  useState,
  useEffect,
  useRef,
  useCallback,
  startTransition,
} from "react";
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
    // Prioritize critical data first to avoid blocking navigation
    try {
      // Phase 1: Load critical data first
      await Promise.all([fetchGitStatus(), fetchWorktrees()]);

      // Phase 2: Load secondary data after core data is available
      startTransition(() => {
        fetchClaudeSessions().catch(console.error);
        fetchActiveSessions().catch(console.error);
      });
    } catch (error) {
      console.error("Failed to refresh data:", error);
    }
  };

  const setLoading = (loading: boolean) => {
    setState((prev) => ({ ...prev, loading }));
  };

  // Compute overall loading state
  const computedLoading = state.loading || state.worktreesLoading;

  // Initial fetch with request prioritization
  useEffect(() => {
    // Phase 1: Load critical data first (git status and worktrees)
    const loadCriticalData = async () => {
      try {
        // Load git status and worktrees first - these are needed for navigation
        await Promise.all([fetchGitStatus(), fetchWorktrees()]);

        // Phase 2: Load secondary data after core data is available
        startTransition(() => {
          fetchRepositories().catch(console.error);
          fetchClaudeSessions().catch(console.error);
          fetchActiveSessions().catch(console.error);
        });
      } catch (error) {
        console.error("Failed to load critical data:", error);
      }
    };

    void loadCriticalData();
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

      if (newWorktreeIds.length > 0 || previousIds.size === 0) {
        // Load data for new worktrees or initial load - use batch operations to avoid duplicates
        startTransition(() => {
          // Single batch operation to load all data efficiently
          const targetWorktrees = state.worktrees.filter((wt) =>
            newWorktreeIds.length > 0 ? newWorktreeIds.includes(wt.id) : true,
          );

          if (targetWorktrees.length > 0) {
            Promise.all([
              gitApi.checkAllConflicts(targetWorktrees),
              gitApi.fetchAllDiffStats(targetWorktrees),
              gitApi.fetchAllPullRequestInfo(targetWorktrees),
            ])
              .then(([conflicts, diffStats, prStatuses]) => {
                setState((prev) => ({
                  ...prev,
                  syncConflicts: {
                    ...prev.syncConflicts,
                    ...conflicts.syncConflicts,
                  },
                  mergeConflicts: {
                    ...prev.mergeConflicts,
                    ...conflicts.mergeConflicts,
                  },
                  diffStats: { ...prev.diffStats, ...diffStats },
                  prStatuses: { ...prev.prStatuses, ...prStatuses },
                }));
              })
              .catch(console.error);
          }
        });
      }

      // Update the previous IDs reference
      previousWorktreeIds.current = currentWorktreeIds;
    }
  }, [state.worktrees]);

  // Generate summaries for qualifying worktrees when they change
  useEffect(() => {
    if (state.worktrees.length > 0) {
      startTransition(() => {
        generateAllWorktreeSummaries().catch(console.error);
      });
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

  // Memoized batch operations to prevent duplicate expensive calls
  const refreshBatchData = useCallback(
    async (worktreeIds: string[]) => {
      const worktreeList = state.worktrees.filter((wt) =>
        worktreeIds.includes(wt.id),
      );

      if (worktreeList.length === 0) {
        return;
      }

      try {
        // Use batch operations for everything
        const [conflicts, diffStats, prStatuses] = await Promise.all([
          gitApi.checkAllConflicts(worktreeList),
          gitApi.fetchAllDiffStats(worktreeList),
          gitApi.fetchAllPullRequestInfo(worktreeList),
        ]);

        setState((prev) => ({
          ...prev,
          syncConflicts: { ...prev.syncConflicts, ...conflicts.syncConflicts },
          mergeConflicts: {
            ...prev.mergeConflicts,
            ...conflicts.mergeConflicts,
          },
          diffStats: { ...prev.diffStats, ...diffStats },
          prStatuses: { ...prev.prStatuses, ...prStatuses },
        }));
      } catch (error) {
        console.error(`Failed to refresh batch data for worktrees:`, error);
      }
    },
    [state.worktrees],
  );

  // Individual update functions now just trigger batch refresh for efficiency
  const updateWorktreeConflicts = useCallback(
    async (worktreeId: string) => {
      await refreshBatchData([worktreeId]);
    },
    [refreshBatchData],
  );

  const updateWorktreeDiffStats = useCallback(
    async (worktreeId: string) => {
      await refreshBatchData([worktreeId]);
    },
    [refreshBatchData],
  );

  const updateWorktreePrStatus = useCallback(
    async (worktreeId: string) => {
      await refreshBatchData([worktreeId]);
    },
    [refreshBatchData],
  );

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

      // Fetch additional data in background without loading indicator - use single batch operation
      if (newWorktrees.length > 0) {
        startTransition(() => {
          // Single batch operation for all new worktrees
          Promise.all([
            gitApi.checkAllConflicts(newWorktrees),
            gitApi.fetchAllDiffStats(newWorktrees),
            gitApi.fetchAllPullRequestInfo(newWorktrees),
          ])
            .then(([conflicts, diffStats, prStatuses]) => {
              setState((prev) => ({
                ...prev,
                syncConflicts: {
                  ...prev.syncConflicts,
                  ...conflicts.syncConflicts,
                },
                mergeConflicts: {
                  ...prev.mergeConflicts,
                  ...conflicts.mergeConflicts,
                },
                diffStats: { ...prev.diffStats, ...diffStats },
                prStatuses: { ...prev.prStatuses, ...prStatuses },
              }));
            })
            .catch(console.error);
        });
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
