import { useState, useCallback } from "react";
import { toast } from "sonner";

interface ApiError {
  error: string;
  conflict_files?: string[];
  worktree_name?: string;
}

export const useGitApi = () => {
  const [loading, setLoading] = useState(false);

  const apiCall = useCallback(async <T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T | null> => {
    try {
      setLoading(true);
      const response = await fetch(endpoint, options);
      if (response.ok) {
        return await response.json();
      }
      const errorData = await response.json();
      throw errorData;
    } catch (error) {
      console.error(`API call failed for ${endpoint}:`, error);
      throw error;
    } finally {
      setLoading(false);
    }
  }, []);

  const get = useCallback((endpoint: string) => {
    return apiCall(endpoint);
  }, [apiCall]);

  const post = useCallback((endpoint: string, data?: any) => {
    return apiCall(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: data ? JSON.stringify(data) : undefined,
    });
  }, [apiCall]);

  const del = useCallback((endpoint: string) => {
    return apiCall(endpoint, { method: "DELETE" });
  }, [apiCall]);

  return { get, post, del, loading };
};

export const useGitOperations = () => {
  const { get, post, del } = useGitApi();

  const fetchGitStatus = useCallback(() => get("/v1/git/status"), [get]);
  const fetchWorktrees = useCallback(() => get("/v1/git/worktrees"), [get]);
  const fetchRepositories = useCallback(() => get("/v1/git/github/repos"), [get]);
  const fetchClaudeSessions = useCallback(() => get("/v1/claude/sessions"), [get]);
  const fetchActiveSessions = useCallback(() => get("/v1/sessions/active"), [get]);
  const fetchBranches = useCallback((repoId: string) => get(`/v1/git/branches/${encodeURIComponent(repoId)}`), [get]);
  
  const checkSyncConflicts = useCallback((worktreeId: string) => get(`/v1/git/worktrees/${worktreeId}/sync/check`), [get]);
  const checkMergeConflicts = useCallback((worktreeId: string) => get(`/v1/git/worktrees/${worktreeId}/merge/check`), [get]);
  
  const deleteWorktree = useCallback((id: string) => del(`/v1/git/worktrees/${id}`), [del]);
  const syncWorktree = useCallback((id: string) => post(`/v1/git/worktrees/${id}/sync`, { strategy: "rebase" }), [post]);
  const mergeWorktree = useCallback((id: string, squash: boolean = true) => post(`/v1/git/worktrees/${id}/merge`, { squash }), [post]);
  const createPreview = useCallback((id: string) => post(`/v1/git/worktrees/${id}/preview`), [post]);
  const createPullRequest = useCallback((id: string, title: string, body: string) => post(`/v1/git/worktrees/${id}/pr`, { title, body }), [post]);

  return {
    fetchGitStatus,
    fetchWorktrees,
    fetchRepositories,
    fetchClaudeSessions,
    fetchActiveSessions,
    fetchBranches,
    checkSyncConflicts,
    checkMergeConflicts,
    deleteWorktree,
    syncWorktree,
    mergeWorktree,
    createPreview,
    createPullRequest,
  };
};