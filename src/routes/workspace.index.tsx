import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAppStore } from "@/stores/appStore";
import { WorkspaceWelcome } from "@/components/WorkspaceWelcome";
import { BackendErrorScreen } from "@/components/BackendErrorScreen";
import { LoadingSpinner } from "@/components/LoadingSpinner";

function WorkspaceRedirect() {
  const navigate = useNavigate();
  const hasRedirected = useRef(false);

  // Use stable selectors to avoid infinite loops
  const initialLoading = useAppStore((state) => state.initialLoading);
  const loadError = useAppStore((state) => state.loadError);
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );
  const getRepositoryById = useAppStore((state) => state.getRepositoryById);

  useEffect(() => {
    if (hasRedirected.current || initialLoading || loadError) {
      return; // Prevent multiple redirects or wait for data to load
    }

    if (worktreesCount > 0) {
      // Find the first available worktree
      const worktrees = useAppStore.getState().getWorktreesList();
      let firstAvailableWorktree = null;

      for (const worktree of worktrees) {
        const repo = getRepositoryById(worktree.repo_id);
        if (repo && repo.available) {
          firstAvailableWorktree = worktree;
          break;
        }
      }

      if (firstAvailableWorktree) {
        // Extract project/workspace from the workspace name (e.g., "vibes/tiger")
        const nameParts = firstAvailableWorktree.name.split("/");
        if (nameParts.length >= 2) {
          hasRedirected.current = true;
          void navigate({
            to: "/workspace/$project/$workspace",
            params: {
              project: nameParts[0],
              workspace: nameParts[1],
            },
          });
          return;
        }
      }
    }

    // Don't redirect if no available workspaces - show welcome screen instead
  }, [initialLoading, loadError, worktreesCount, navigate, getRepositoryById]);

  // Show error screen if backend is unavailable
  if (loadError) {
    return <BackendErrorScreen />;
  }

  // Show welcome screen if no workspaces
  if (!initialLoading && worktreesCount === 0) {
    return <WorkspaceWelcome />;
  }

  // Show loading while checking for workspaces
  return (
    <div className="flex h-screen items-center justify-center">
      <LoadingSpinner message="Finding workspace..." size="lg" />
    </div>
  );
}

export const Route = createFileRoute("/workspace/")({
  component: WorkspaceRedirect,
});
