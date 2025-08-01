import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAppStore } from "@/stores/appStore";

function WorkspaceRedirect() {
  const navigate = useNavigate();
  const hasRedirected = useRef(false);

  // Use stable selectors to avoid infinite loops
  const initialLoading = useAppStore((state) => state.initialLoading);
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );

  useEffect(() => {
    if (hasRedirected.current || initialLoading) {
      return; // Prevent multiple redirects or wait for data to load
    }

    if (worktreesCount > 0) {
      // Get the first worktree without creating a new array reference
      const firstWorktree = useAppStore.getState().getWorktreesList()[0];

      // Extract project/workspace from the workspace name (e.g., "vibes/tiger")
      const nameParts = firstWorktree.name.split("/");
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

    // If no workspaces, redirect to terminal
    hasRedirected.current = true;
    void navigate({ to: "/terminal" });
  }, [initialLoading, worktreesCount, navigate]);

  return (
    <div className="flex h-screen items-center justify-center">
      <div className="text-center space-y-4">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mx-auto"></div>
        <p className="text-muted-foreground">Finding workspace...</p>
        {!initialLoading && worktreesCount === 0 && (
          <p className="text-sm text-muted-foreground">
            No workspaces found, redirecting to terminal...
          </p>
        )}
      </div>
    </div>
  );
}

export const Route = createFileRoute("/workspace/")({
  component: WorkspaceRedirect,
});
