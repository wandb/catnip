import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { useAppStore } from "@/stores/appStore";
import { WorkspaceWelcome } from "@/components/WorkspaceWelcome";
import { WorkspaceMobileIndex } from "@/components/WorkspaceMobileIndex";
import { BackendErrorScreen } from "@/components/BackendErrorScreen";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { useIsMobile } from "@/hooks/use-mobile";

function WorkspaceIndex() {
  const navigate = useNavigate();
  const isMobile = useIsMobile();

  // Use stable selectors to avoid infinite loops
  const initialLoading = useAppStore((state) => state.initialLoading);
  const loadError = useAppStore((state) => state.loadError);
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );

  // Effect to redirect to most recent workspace (desktop) or repos (mobile)
  useEffect(() => {
    if (!initialLoading && !loadError && worktreesCount > 0) {
      // On mobile, always redirect to repos
      if (isMobile) {
        void navigate({ to: "/workspace/repos", replace: true });
        return;
      }

      // Desktop behavior: redirect to most recent workspace
      const worktreesList = useAppStore.getState().getWorktreesList();

      // Sort by last_accessed (descending) to get most recent
      const sortedWorktrees = [...worktreesList].sort((a, b) => {
        const aAccessed = new Date(a.last_accessed || a.created_at).getTime();
        const bAccessed = new Date(b.last_accessed || b.created_at).getTime();
        return bAccessed - aAccessed;
      });

      const mostRecentWorktree = sortedWorktrees[0];
      if (mostRecentWorktree) {
        const nameParts = mostRecentWorktree.name.split("/");
        if (nameParts.length >= 2) {
          void navigate({
            to: "/workspace/$project/$workspace",
            params: {
              project: nameParts[0],
              workspace: nameParts[1],
            },
            search: { prompt: undefined },
            replace: true,
          });
          return;
        }
      }

      // Fallback to repos if we can't find a valid workspace
      void navigate({ to: "/workspace/repos", replace: true });
    }
  }, [initialLoading, loadError, worktreesCount, navigate, isMobile]);

  // Show error screen if backend is unavailable
  if (loadError) {
    return <BackendErrorScreen />;
  }

  // Show loading while checking for workspaces
  if (initialLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <LoadingSpinner message="Loading workspaces..." size="lg" />
      </div>
    );
  }

  // Show welcome screen if no workspaces
  if (worktreesCount === 0) {
    return <WorkspaceWelcome />;
  }

  // Show mobile index if on mobile - keeps the same behavior
  if (isMobile) {
    return <WorkspaceMobileIndex />;
  }

  // Show loading while redirecting
  return (
    <div className="flex h-screen items-center justify-center">
      <LoadingSpinner message="Redirecting to workspace..." size="lg" />
    </div>
  );
}

export const Route = createFileRoute("/workspace/")({
  component: WorkspaceIndex,
});
