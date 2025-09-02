import { createFileRoute } from "@tanstack/react-router";
import { useAppStore } from "@/stores/appStore";
import { WorkspaceWelcome } from "@/components/WorkspaceWelcome";
import { WorkspaceMobileIndex } from "@/components/WorkspaceMobileIndex";
import { BackendErrorScreen } from "@/components/BackendErrorScreen";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { RepositoryList } from "@/components/RepositoryList";
import { useIsMobile } from "@/hooks/use-mobile";

function WorkspaceIndex() {
  const isMobile = useIsMobile();

  // Use stable selectors to avoid infinite loops
  const initialLoading = useAppStore((state) => state.initialLoading);
  const loadError = useAppStore((state) => state.loadError);
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );

  // Show error screen if backend is unavailable
  if (loadError) {
    return <BackendErrorScreen />;
  }

  // Show loading while checking for workspaces
  if (initialLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <LoadingSpinner message="Loading repositories..." size="lg" />
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

  // Show repository list for desktop users
  return <RepositoryList />;
}

export const Route = createFileRoute("/workspace/")({
  component: WorkspaceIndex,
});
