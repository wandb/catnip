import { useState, useEffect, useRef } from "react";
import { gitApi, type WorktreeDiffStats } from "@/lib/git-api";

/**
 * Hook that fetches and caches worktree diff stats when commit_hash or is_dirty changes
 */
export function useWorktreeDiff(
  worktreeId: string,
  commitHash: string,
  isDirty: boolean,
) {
  const [diffStats, setDiffStats] = useState<WorktreeDiffStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastFetchRef = useRef<string>("");

  useEffect(() => {
    // Create a unique key for this fetch based on worktree state
    const fetchKey = `${worktreeId}-${commitHash}-${isDirty}`;

    // Skip if we already fetched for this exact state
    if (fetchKey === lastFetchRef.current) {
      return;
    }

    const fetchDiff = async () => {
      if (!worktreeId) return;

      setLoading(true);
      setError(null);

      try {
        const data = await gitApi.fetchWorktreeDiffStats(worktreeId);
        setDiffStats(data);
        lastFetchRef.current = fetchKey;
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to fetch diff");
        setDiffStats(null);
      } finally {
        setLoading(false);
      }
    };

    void fetchDiff();
  }, [worktreeId, commitHash, isDirty]);

  // Reset when worktree changes
  useEffect(() => {
    setDiffStats(null);
    setError(null);
    lastFetchRef.current = "";
  }, [worktreeId]);

  return {
    diffStats,
    loading,
    error,
    refetch: () => {
      lastFetchRef.current = "";
      const fetchKey = `${worktreeId}-${commitHash}-${isDirty}`;
      if (fetchKey !== lastFetchRef.current) {
        // Trigger re-fetch
        setDiffStats(null);
      }
    },
  };
}
