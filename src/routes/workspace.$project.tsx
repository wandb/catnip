import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { useAppStore } from "@/stores/appStore";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { BackendErrorScreen } from "@/components/BackendErrorScreen";

interface Worktree {
  id: string;
  name: string;
  repo_id: string;
  last_accessed: string;
}

interface Repository {
  id: string;
  name?: string;
  available?: boolean;
}

function ProjectWorkspaceRedirect() {
  const { project } = createFileRoute("/workspace/$project").useParams();
  const navigate = useNavigate();
  const hasRedirected = useRef(false);

  // Use stable selectors to avoid infinite loops
  const initialLoading = useAppStore((state) => state.initialLoading);
  const loadError = useAppStore((state) => state.loadError);
  const getRepositoryById = useAppStore((state) => state.getRepositoryById);
  const getWorktreesList = useAppStore((state) => state.getWorktreesList);

  useEffect(() => {
    if (hasRedirected.current || initialLoading || loadError) {
      return; // Prevent multiple redirects or wait for data to load
    }

    const worktrees = getWorktreesList();
    
    // Find all worktrees for this project
    const projectWorktrees = worktrees.filter((worktree: Worktree) => {
      const nameParts = worktree.name.split("/");
      if (nameParts[0] !== project) return false;
      
      // Check if the repository is available
      const repo = getRepositoryById(worktree.repo_id) as Repository | undefined;
      return repo && repo.available !== false;
    });

    if (projectWorktrees.length === 0) {
      // No workspaces found for this project, redirect to index
      hasRedirected.current = true;
      void navigate({ to: "/workspace" });
      return;
    }

    // Find the most recent worktree for this project
    const mostRecentWorktree = projectWorktrees.reduce((latest: Worktree, current: Worktree) => {
      const latestTime = new Date(latest.last_accessed).getTime();
      const currentTime = new Date(current.last_accessed).getTime();
      return currentTime > latestTime ? current : latest;
    });

    // Redirect to the most recent workspace
    const nameParts = mostRecentWorktree.name.split("/");
    if (nameParts.length >= 2) {
      hasRedirected.current = true;
      void navigate({
        to: "/workspace/$project/$workspace",
        params: {
          project: nameParts[0],
          workspace: nameParts[1],
        },
      });
    } else {
      // Malformed workspace name, redirect to index
      hasRedirected.current = true;
      void navigate({ to: "/workspace" });
    }
  }, [project, initialLoading, loadError, navigate, getRepositoryById, getWorktreesList]);

  // Show error screen if backend is unavailable
  if (loadError) {
    return <BackendErrorScreen />;
  }

  // Show loading while finding workspace
  return (
    <div className="flex h-screen items-center justify-center">
      <div className="text-center space-y-4">
        <LoadingSpinner message="Finding workspace..." size="lg" />
        <div className="text-sm text-muted-foreground">
          Looking for recent workspaces in {project}
        </div>
      </div>
    </div>
  );
}

export const Route = createFileRoute("/workspace/$project")({
  component: ProjectWorkspaceRedirect,
});