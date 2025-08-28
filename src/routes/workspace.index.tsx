import { createFileRoute } from "@tanstack/react-router";
import { useAppStore } from "@/stores/appStore";
import { WorkspaceWelcome } from "@/components/WorkspaceWelcome";
import { BackendErrorScreen } from "@/components/BackendErrorScreen";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { RepositoryListSidebar } from "@/components/RepositoryListSidebar";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { ErrorBoundary } from "react-error-boundary";

function ErrorFallback({ error, resetErrorBoundary }: { error: Error; resetErrorBoundary: () => void }) {
  return (
    <div className="flex h-screen items-center justify-center">
      <div className="text-center space-y-4">
        <div className="text-red-500 text-sm">Something went wrong</div>
        <div className="text-xs text-muted-foreground max-w-md">
          {error.message}
        </div>
        <button
          onClick={resetErrorBoundary}
          className="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm hover:bg-primary/90"
        >
          Try again
        </button>
      </div>
    </div>
  );
}

function WorkspaceIndex() {
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

  // Show loading while data loads
  if (initialLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-center space-y-4">
          <LoadingSpinner message="Loading repositories..." size="lg" />
          <div className="text-sm text-muted-foreground">
            Discovering your workspaces
          </div>
        </div>
      </div>
    );
  }

  // Show welcome screen if no workspaces
  if (worktreesCount === 0) {
    return <WorkspaceWelcome />;
  }

  // Show repository list layout
  return (
    <ErrorBoundary FallbackComponent={ErrorFallback}>
      <SidebarProvider>
        <div className="flex h-screen w-full">
          <RepositoryListSidebar />
          <SidebarInset className="flex-1 min-w-0">
            <main className="flex h-full items-center justify-center" role="main">
              <div className="text-center space-y-4 p-8 max-w-md">
                <div className="text-lg font-medium">Select a repository</div>
                <div className="text-sm text-muted-foreground">
                  Choose a repository from the sidebar to view its workspaces and get started.
                </div>
                <div className="text-xs text-muted-foreground">
                  You can also create a new repository by clicking the "New repository" button.
                </div>
              </div>
            </main>
          </SidebarInset>
        </div>
      </SidebarProvider>
    </ErrorBoundary>
  );
}

export const Route = createFileRoute("/workspace/")({
  component: WorkspaceIndex,
});
