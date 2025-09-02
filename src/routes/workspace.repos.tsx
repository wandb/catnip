import { createFileRoute } from "@tanstack/react-router";
import { useAppStore } from "@/stores/appStore";
import { WorkspaceWelcome } from "@/components/WorkspaceWelcome";
import { WorkspaceMobileIndex } from "@/components/WorkspaceMobileIndex";
import { BackendErrorScreen } from "@/components/BackendErrorScreen";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { RepositoryList } from "@/components/RepositoryList";
import { SidebarProvider, SidebarInset } from "@/components/ui/sidebar";
import { useIsMobile } from "@/hooks/use-mobile";

function WorkspaceRepos() {
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

  // Show repository list in sidebar layout for desktop users
  return (
    <SidebarProvider>
      <div className="flex h-screen w-full">
        {/* Sidebar with repository list */}
        <div className="w-80 border-r bg-sidebar">
          <RepositoryList />
        </div>
        {/* Main content area */}
        <SidebarInset className="flex-1 min-w-0">
          <div className="flex h-full items-center justify-center bg-background">
            <div className="text-center space-y-4">
              <h2 className="text-2xl font-semibold text-muted-foreground">
                Select a Repository
              </h2>
              <p className="text-sm text-muted-foreground">
                Choose a repository from the sidebar to view its workspaces
              </p>
            </div>
          </div>
        </SidebarInset>
      </div>
    </SidebarProvider>
  );
}

export const Route = createFileRoute("/workspace/repos")({
  component: WorkspaceRepos,
});
