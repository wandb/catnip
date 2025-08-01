import { createFileRoute, Link } from "@tanstack/react-router";
import { useParams } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { useAppStore } from "@/stores/appStore";
import { WorkspaceLeftSidebar } from "@/components/WorkspaceLeftSidebar";
import { WorkspaceRightSidebar } from "@/components/WorkspaceRightSidebar";
import { WorkspaceMainContent } from "@/components/WorkspaceMainContent";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";

function WorkspacePage() {
  const { project, workspace } = useParams({
    from: "/workspace/$project/$workspace",
  });

  // State for toggling between Claude terminal and diff view
  const [showDiffView, setShowDiffView] = useState(false);
  // State for showing port preview
  const [showPortPreview, setShowPortPreview] = useState<number | null>(null);

  // Construct the workspace name from URL params
  const workspaceName = `${project}/${workspace}`;

  // Use stable selectors to avoid infinite loops - only get counts first
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );
  const repositoriesCount = useAppStore(
    (state) => state.getRepositoriesList().length,
  );
  const initialLoading = useAppStore((state) => state.initialLoading);
  // Subscribe to the actual worktrees map to get updates when individual worktrees change
  const worktrees = useAppStore((state) => state.worktrees);

  // Find the worktree by name using useMemo and direct store access
  const worktree = useMemo(() => {
    if (worktreesCount === 0) return undefined;
    const worktreesList = useAppStore.getState().getWorktreesList();
    return worktreesList.find((w) => w.name === workspaceName);
  }, [workspaceName, worktreesCount, worktrees]);

  // Find the repository by repo_id using useMemo and direct store access
  const repository = useMemo(() => {
    if (!worktree || repositoriesCount === 0) return undefined;
    const repositoriesList = useAppStore.getState().getRepositoriesList();
    return repositoriesList.find((r) => r.id === worktree.repo_id);
  }, [worktree, repositoriesCount]);

  if (initialLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-center space-y-4">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mx-auto"></div>
          <p className="text-muted-foreground">Loading workspace...</p>
        </div>
      </div>
    );
  }

  if (!worktree || !repository) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-center space-y-4">
          <div className="text-red-500 text-sm">Workspace not found</div>
          <div className="text-xs text-muted-foreground space-y-1">
            <div>Looking for: {workspaceName}</div>
            <div>Available workspaces: {worktreesCount}</div>
          </div>
          {worktreesCount > 0 && (
            <div className="text-xs text-muted-foreground">
              <p>Available workspaces:</p>
              <ul className="mt-2 space-y-1">
                {useAppStore
                  .getState()
                  .getWorktreesList()
                  .slice(0, 5)
                  .map((wt) => {
                    const parts = wt.name.split("/");
                    return (
                      <li key={wt.id} className="text-left">
                        <Link
                          to="/workspace/$project/$workspace"
                          params={{
                            project: parts[0],
                            workspace: parts[1],
                          }}
                          className="text-blue-400 hover:text-blue-300"
                        >
                          {wt.name}
                        </Link>
                      </li>
                    );
                  })}
              </ul>
            </div>
          )}
        </div>
      </div>
    );
  }

  // Render the full workspace layout with sidebars and main content
  return (
    <SidebarProvider>
      <div className="flex h-screen w-full">
        <WorkspaceLeftSidebar />
        <SidebarInset className="flex-1 min-w-0">
          <WorkspaceMainContent
            worktree={worktree}
            repository={repository}
            showDiffView={showDiffView}
            setShowDiffView={setShowDiffView}
            showPortPreview={showPortPreview}
            setShowPortPreview={setShowPortPreview}
          />
        </SidebarInset>
        <WorkspaceRightSidebar
          worktree={worktree}
          repository={repository}
          showDiffView={showDiffView}
          setShowDiffView={setShowDiffView}
          showPortPreview={showPortPreview}
          setShowPortPreview={setShowPortPreview}
        />
      </div>
    </SidebarProvider>
  );
}

export const Route = createFileRoute("/workspace/$project/$workspace")({
  component: WorkspacePage,
});
