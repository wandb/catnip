import { useEffect, useState } from "react";
import { useAppStore } from "@/stores/appStore";
import { gitApi, type Worktree } from "@/lib/git-api";

/**
 * Hook that combines the SSE-driven worktree cache with the existing git state
 * This provides a migration path from REST polling to real-time updates
 */
export function useWorktreeStore() {
  const [loading, setLoading] = useState(false);
  const {
    worktrees,
    getWorktreesList,
    getWorktreeById,
    setWorktrees,
    updateWorktree,
  } = useAppStore();

  // Initial load of worktrees from REST API
  // This will be enhanced by SSE events as they come in
  useEffect(() => {
    const loadInitialWorktrees = async () => {
      setLoading(true);
      try {
        const initialWorktrees = await gitApi.fetchWorktrees();

        // Transform REST response to include cache status
        const enhancedWorktrees = initialWorktrees.map(
          (worktree: Worktree) => ({
            ...worktree,
            cache_status: {
              is_cached: true, // Assume fresh data from REST
              is_loading: false,
              last_updated: Date.now(),
            },
          }),
        );

        setWorktrees(enhancedWorktrees);
      } catch (error) {
        console.error("Failed to load initial worktrees:", error);
      } finally {
        setLoading(false);
      }
    };

    // Only load if we don't have worktrees yet
    if (worktrees.size === 0) {
      void loadInitialWorktrees();
    }
  }, [worktrees.size, setWorktrees]);

  return {
    // Worktree data
    worktrees: getWorktreesList(),
    worktreeMap: worktrees,
    loading,

    // Getters
    getWorktreeById,

    // Actions
    updateWorktree,
    setWorktrees,

    // Legacy compatibility - for components that expect arrays
    getWorktreesArray: getWorktreesList,
  };
}
