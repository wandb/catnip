import { useState, useEffect } from "react";
import { gitApi, type GitStatus, type Worktree, type Repository } from "@/lib/git-api";

export interface GitState {
  gitStatus: GitStatus;
  worktrees: Worktree[];
  repositories: Repository[];
  repoBranches: Record<string, string[]>;
  claudeSessions: Record<string, any>;
  activeSessions: Record<string, any>;
  syncConflicts: Record<string, any>;
  mergeConflicts: Record<string, any>;
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
    fetchGitStatus();
    fetchWorktrees();
    fetchRepositories();
    fetchClaudeSessions();
    fetchActiveSessions();
  }, []);

  // Check for conflicts when worktrees change
  useEffect(() => {
    if (state.worktrees.length > 0) {
      checkConflicts();
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
    refreshAll,
    setLoading,
  };
}