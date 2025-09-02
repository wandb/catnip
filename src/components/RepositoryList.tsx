import { useState, useMemo } from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  Folder,
  Plus,
  AlertTriangle,
  ChevronRight,
  ChevronsLeft,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAppStore } from "@/stores/appStore";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import type { Worktree } from "@/lib/git-api";
import { formatDistanceToNow } from "date-fns";

export function RepositoryList() {
  const [newWorkspaceDialogOpen, setNewWorkspaceDialogOpen] = useState(false);
  const navigate = useNavigate();

  // Get repositories and worktrees from store
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );
  const worktrees = useAppStore((state) => state.worktrees);
  const getWorktreesByRepo = useAppStore((state) => state.getWorktreesByRepo);
  const getRepository = useAppStore((state) => state.getRepositoryById);

  // Get repositories that have worktrees, grouped by repository
  const repositoriesWithWorktrees = useMemo(() => {
    if (worktreesCount === 0) return [];

    const worktreesList = useAppStore.getState().getWorktreesList();
    const repoIds = new Set(worktreesList.map((w) => w.repo_id));

    return Array.from(repoIds)
      .map((repoId) => {
        const repo = getRepository(repoId);
        const worktrees = getWorktreesByRepo(repoId);
        // Sort worktrees by created_at (descending), fallback to last_accessed (descending)
        const sortedWorktrees = worktrees.sort((a, b) => {
          const aCreated = new Date(a.created_at).getTime();
          const bCreated = new Date(b.created_at).getTime();
          if (aCreated !== bCreated) {
            return bCreated - aCreated;
          }

          const aAccessed = new Date(a.last_accessed).getTime();
          const bAccessed = new Date(b.last_accessed).getTime();
          return bAccessed - aAccessed;
        });
        return repo ? { ...repo, worktrees: sortedWorktrees } : null;
      })
      .filter((repo): repo is NonNullable<typeof repo> => repo !== null)
      .sort((a, b) => {
        // Sort repositories by most recent activity
        const aMostRecent =
          a.worktrees[0]?.last_accessed || a.worktrees[0]?.created_at;
        const bMostRecent =
          b.worktrees[0]?.last_accessed || b.worktrees[0]?.created_at;

        if (aMostRecent && bMostRecent) {
          return (
            new Date(bMostRecent).getTime() - new Date(aMostRecent).getTime()
          );
        }

        // Fallback to name sorting
        const nameA = a.name || a.id;
        const nameB = b.name || b.id;
        return nameA.localeCompare(nameB);
      });
  }, [worktreesCount, worktrees, getWorktreesByRepo, getRepository]);

  const handleRepositoryClick = (repo: any) => {
    // Navigate to the most recent workspace in this repository
    const mostRecentWorktree = repo.worktrees[0];
    if (mostRecentWorktree) {
      const nameParts = mostRecentWorktree.name.split("/");
      if (nameParts.length >= 2) {
        void navigate({
          to: "/workspace/$project/$workspace",
          params: {
            project: nameParts[0],
            workspace: nameParts[1],
          },
        });
      }
    }
  };

  const getRepositoryStatus = (repo: any) => {
    const activeCount = repo.worktrees.filter(
      (w: Worktree) => w.claude_activity_state === "active",
    ).length;
    const runningCount = repo.worktrees.filter(
      (w: Worktree) => w.claude_activity_state === "running",
    ).length;

    if (activeCount > 0) {
      return { color: "bg-green-500", label: `${activeCount} active` };
    } else if (runningCount > 0) {
      return { color: "bg-blue-500", label: `${runningCount} running` };
    }
    return null;
  };

  const getMostRecentActivity = (repo: any) => {
    const mostRecent = repo.worktrees[0];
    if (!mostRecent) return null;

    const lastActivity = mostRecent.last_accessed || mostRecent.created_at;
    if (!lastActivity) return null;

    return formatDistanceToNow(new Date(lastActivity), { addSuffix: true });
  };

  return (
    <div className="flex h-full flex-col bg-sidebar">
      {/* Header */}
      <div className="px-4 py-3 border-b border-border/20 space-y-3">
        {/* Logo and collapse button */}
        <div className="flex items-center justify-between">
          <img src="/logo@2x.png" alt="Catnip" className="w-8 h-8" />
          <button className="h-6 w-6 flex items-center justify-center text-muted-foreground hover:text-foreground transition-colors">
            <ChevronsLeft className="h-4 w-4" />
          </button>
        </div>

        {/* Repositories header with badge and new repo button */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <h1 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
              REPOSITORIES
            </h1>
            <span className="bg-black/20 text-muted-foreground text-xs px-1.5 py-0.5 rounded-md font-medium">
              {repositoriesWithWorktrees.length}
            </span>
          </div>
          <Button
            onClick={() => setNewWorkspaceDialogOpen(true)}
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-muted-foreground hover:text-foreground"
          >
            <Plus className="h-3 w-3" />
            <span className="ml-1 text-xs">New repo</span>
          </Button>
        </div>
      </div>

      {/* Repository list */}
      <div className="flex-1 overflow-auto p-2 space-y-2">
        {repositoriesWithWorktrees.map((repo) => {
          const projectName =
            repo.worktrees[0]?.name.split("/")[0] || repo.name;
          const isAvailable = repo.available !== false;
          const status = getRepositoryStatus(repo);
          const lastActivity = getMostRecentActivity(repo);

          return (
            <div
              key={repo.id}
              className={`p-3 cursor-pointer transition-all hover:bg-muted/50 rounded-md ${
                !isAvailable ? "opacity-60" : ""
              }`}
              onClick={() => isAvailable && handleRepositoryClick(repo)}
            >
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 min-w-0 flex-1">
                  <Folder className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      {status && (
                        <div
                          className={`w-2 h-2 rounded-full ${status.color} animate-pulse flex-shrink-0`}
                          title={status.label}
                        />
                      )}
                      <span className="font-medium text-sm truncate">
                        {projectName}
                      </span>
                      {!isAvailable && (
                        <AlertTriangle className="h-3 w-3 text-yellow-500 flex-shrink-0" />
                      )}
                    </div>
                    <div className="text-xs text-muted-foreground flex items-center gap-1">
                      <span>{repo.worktrees.length} kitties</span>
                      {lastActivity && (
                        <>
                          <span>·</span>
                          <span className="truncate">{lastActivity}</span>
                        </>
                      )}
                    </div>
                  </div>
                </div>
                <ChevronRight className="h-4 w-4 text-muted-foreground flex-shrink-0" />
              </div>
              {!isAvailable && (
                <div className="mt-2 p-2 bg-yellow-50 dark:bg-yellow-950/20 border border-yellow-200 dark:border-yellow-900 rounded text-xs">
                  <p className="text-yellow-800 dark:text-yellow-200">
                    Not available. Run{" "}
                    <code className="px-1 py-0.5 bg-yellow-100 dark:bg-yellow-900/50 rounded font-mono text-[10px]">
                      catnip run
                    </code>{" "}
                    from repo.
                  </p>
                </div>
              )}
            </div>
          );
        })}

        {repositoriesWithWorktrees.length === 0 && (
          <div className="flex flex-col items-center justify-center p-8 text-center">
            <Folder className="h-12 w-12 text-muted-foreground mb-4" />
            <h3 className="font-semibold mb-2">No repositories yet</h3>
            <p className="text-sm text-muted-foreground mb-4">
              Create your first repository to get started
            </p>
            <Button onClick={() => setNewWorkspaceDialogOpen(true)} size="sm">
              <Plus className="h-4 w-4 mr-2" />
              New repository
            </Button>
          </div>
        )}
      </div>

      <NewWorkspaceDialog
        open={newWorkspaceDialogOpen}
        onOpenChange={setNewWorkspaceDialogOpen}
      />
    </div>
  );
}
