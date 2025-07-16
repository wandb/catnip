import { useState, useEffect } from "react";
import { gitApi, type GitStatus, type Worktree, type Repository, type WorktreeDiffStats, type PullRequestInfo } from "@/lib/git-api";
import { generateWorktreeSummary, shouldGenerateSummary, type WorktreeSummary } from "@/lib/worktree-summary";

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
  });

  const fetchGitStatus = async () => {
    try {
      const data = await gitApi.fetchGitStatus();
      setState(prev => ({ ...prev, gitStatus: data }));

      // Fetch branches for each repository
      if (data.repositories) {
        const branchMap = await gitApi.fetchBranchesForRepositories(data.repositories);
        setState(prev => ({ ...prev, repoBranches: branchMap }));
      }
    } catch (error) {
      console.error("Failed to fetch git status:", error);
    }
  };

  const fetchWorktrees = async () => {
    try {
      const data = await gitApi.fetchWorktrees();
      setState(prev => ({ ...prev, worktrees: data }));
    } catch (error) {
      console.error("Failed to fetch worktrees:", error);
    }
  };

  const fetchRepositories = async () => {
    setState(prev => ({ ...prev, reposLoading: true }));
    try {
      const data = await gitApi.fetchRepositories();
      setState(prev => ({ ...prev, repositories: data }));
    } catch (error) {
      console.error("Failed to fetch repositories:", error);
    } finally {
      setState(prev => ({ ...prev, reposLoading: false }));
    }
  };

  const fetchClaudeSessions = async () => {
    const data = await gitApi.fetchClaudeSessions();
    setState(prev => ({ ...prev, claudeSessions: data }));
  };

  const fetchActiveSessions = async () => {
    const data = await gitApi.fetchActiveSessions();
    setState(prev => ({ ...prev, activeSessions: data }));
  };

  const checkConflicts = async () => {
    const { syncConflicts, mergeConflicts } = await gitApi.checkAllConflicts(state.worktrees);
    setState(prev => ({ ...prev, syncConflicts, mergeConflicts }));
  };

  const fetchDiffStats = async () => {
    try {
      const diffStats = await gitApi.fetchAllDiffStats(state.worktrees);
      setState(prev => ({ ...prev, diffStats }));
    } catch (error) {
      console.error("Failed to fetch diff stats:", error);
    }
  };

  // Fetch PR statuses for all worktrees
  const fetchPrStatuses = async () => {
    if (state.worktrees.length === 0) {
      setState(prev => ({ ...prev, prStatuses: {} }));
      return;
    }

    try {
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

      setState(prev => ({ ...prev, prStatuses: newPrStatuses }));
    } catch (error) {
      console.error("Failed to fetch PR statuses:", error);
    }
  };

  // Generate summary for a specific worktree
  const generateWorktreeSummaryForId = async (worktreeId: string) => {
    // Set status to generating
    setState(prev => ({
      ...prev,
      worktreeSummaries: {
        ...prev.worktreeSummaries,
        [worktreeId]: {
          worktreeId,
          title: '',
          summary: '',
          status: 'generating'
        }
      }
    }));

    try {
      const summary = await generateWorktreeSummary(worktreeId);
      setState(prev => ({
        ...prev,
        worktreeSummaries: {
          ...prev.worktreeSummaries,
          [worktreeId]: summary
        }
      }));
    } catch (error) {
      console.error(`Failed to generate summary for worktree ${worktreeId}:`, error);
      setState(prev => ({
        ...prev,
        worktreeSummaries: {
          ...prev.worktreeSummaries,
          [worktreeId]: {
            worktreeId,
            title: 'Failed to generate summary',
            summary: 'An error occurred while generating the summary',
            status: 'error',
            error: error instanceof Error ? error.message : 'Unknown error'
          }
        }
      }));
    }
  };

  // Generate summaries for all qualifying worktrees
  const generateAllWorktreeSummaries = async () => {
    const qualifyingWorktrees = state.worktrees.filter(shouldGenerateSummary);
    
    // Initialize pending summaries
    const pendingSummaries: Record<string, WorktreeSummary> = {};
    qualifyingWorktrees.forEach(worktree => {
      if (!state.worktreeSummaries[worktree.id] || state.worktreeSummaries[worktree.id].status === 'error') {
        pendingSummaries[worktree.id] = {
          worktreeId: worktree.id,
          title: '',
          summary: '',
          status: 'pending'
        };
      }
    });

    if (Object.keys(pendingSummaries).length > 0) {
      setState(prev => ({
        ...prev,
        worktreeSummaries: {
          ...prev.worktreeSummaries,
          ...pendingSummaries
        }
      }));

      // Generate summaries in parallel
      const summaryPromises = Object.keys(pendingSummaries).map(async (worktreeId) => {
        // Set to generating
        setState(prev => ({
          ...prev,
          worktreeSummaries: {
            ...prev.worktreeSummaries,
            [worktreeId]: {
              ...prev.worktreeSummaries[worktreeId],
              status: 'generating'
            }
          }
        }));

        try {
          const summary = await generateWorktreeSummary(worktreeId);
          setState(prev => ({
            ...prev,
            worktreeSummaries: {
              ...prev.worktreeSummaries,
              [worktreeId]: summary
            }
          }));
        } catch (error) {
          console.error(`Failed to generate summary for worktree ${worktreeId}:`, error);
          setState(prev => ({
            ...prev,
            worktreeSummaries: {
              ...prev.worktreeSummaries,
              [worktreeId]: {
                worktreeId,
                title: 'Failed to generate summary',
                summary: 'An error occurred while generating the summary',
                status: 'error',
                error: error instanceof Error ? error.message : 'Unknown error'
              }
            }
          }));
        }
      });

      await Promise.all(summaryPromises);
    }
  };

  // Clear summary for a specific worktree
  const clearWorktreeSummary = (worktreeId: string) => {
    setState(prev => {
      const newSummaries = { ...prev.worktreeSummaries };
      delete newSummaries[worktreeId];
      return { ...prev, worktreeSummaries: newSummaries };
    });
  };

  // Clear all summaries
  const clearAllWorktreeSummaries = () => {
    setState(prev => ({ ...prev, worktreeSummaries: {} }));
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
    setState(prev => ({ ...prev, loading }));
  };

  // Initial fetch
  useEffect(() => {
    void fetchGitStatus();
    void fetchWorktrees();
    void fetchRepositories();
    void fetchClaudeSessions();
    void fetchActiveSessions();
  }, []);

  // Check for conflicts, fetch diff stats, and fetch PR statuses when worktrees change
  useEffect(() => {
    if (state.worktrees.length > 0) {
      void checkConflicts();
      void fetchDiffStats();
      void fetchPrStatuses();
    }
  }, [state.worktrees]);

  // Generate summaries for qualifying worktrees when they change
  useEffect(() => {
    if (state.worktrees.length > 0) {
      void generateAllWorktreeSummaries();
    }
  }, [state.worktrees]);

  return {
    ...state,
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
  };
}
