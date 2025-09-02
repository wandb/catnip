import { useState, useMemo } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Folder, Plus, AlertTriangle, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useAppStore } from "@/stores/appStore";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import type { Worktree } from "@/lib/git-api";
import { formatDistanceToNow } from "date-fns";

export function RepositoryList() {
  const [newWorkspaceDialogOpen, setNewWorkspaceDialogOpen] = useState(false);
  const navigate = useNavigate();

  // Get repositories and worktrees from store
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length
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
        const aMostRecent = a.worktrees[0]?.last_accessed || a.worktrees[0]?.created_at;
        const bMostRecent = b.worktrees[0]?.last_accessed || b.worktrees[0]?.created_at;
        
        if (aMostRecent && bMostRecent) {
          return new Date(bMostRecent).getTime() - new Date(aMostRecent).getTime();
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
    const activeCount = repo.worktrees.filter((w: Worktree) => 
      w.claude_activity_state === "active"
    ).length;
    const runningCount = repo.worktrees.filter((w: Worktree) => 
      w.claude_activity_state === "running"
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
    <div className="container mx-auto p-6 max-w-4xl">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Repositories</h1>
          <p className="text-muted-foreground mt-2">
            {repositoriesWithWorktrees.length} {repositoriesWithWorktrees.length === 1 ? 'repository' : 'repositories'}
          </p>
        </div>
        <Button
          onClick={() => setNewWorkspaceDialogOpen(true)}
          size="lg"
          className="gap-2"
        >
          <Plus className="h-5 w-5" />
          New repository
        </Button>
      </div>

      <div className="grid gap-4">
        {repositoriesWithWorktrees.map((repo) => {
          const projectName = repo.worktrees[0]?.name.split("/")[0] || repo.name;
          const isAvailable = repo.available !== false;
          const status = getRepositoryStatus(repo);
          const lastActivity = getMostRecentActivity(repo);

          return (
            <Card
              key={repo.id}
              className={`cursor-pointer transition-all hover:shadow-md ${
                !isAvailable ? "opacity-60" : ""
              }`}
              onClick={() => isAvailable && handleRepositoryClick(repo)}
            >
              <CardHeader className="pb-4">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="p-2 bg-muted rounded-lg">
                      <Folder className="h-5 w-5" />
                    </div>
                    <div>
                      <CardTitle className="text-xl flex items-center gap-2">
                        {projectName}
                        {!isAvailable && (
                          <AlertTriangle className="h-4 w-4 text-yellow-500" />
                        )}
                      </CardTitle>
                      <CardDescription className="mt-1">
                        {repo.worktrees.length} {repo.worktrees.length === 1 ? 'workspace' : 'workspaces'}
                        {lastActivity && <span> · {lastActivity}</span>}
                      </CardDescription>
                    </div>
                  </div>
                  {status && (
                    <div className="flex items-center gap-2">
                      <div className={`w-2 h-2 rounded-full ${status.color} animate-pulse`} />
                      <span className="text-sm text-muted-foreground">{status.label}</span>
                    </div>
                  )}
                </div>
              </CardHeader>
              {!isAvailable && (
                <CardContent className="pt-0">
                  <div className="bg-yellow-50 dark:bg-yellow-950/20 border border-yellow-200 dark:border-yellow-900 rounded-lg p-3">
                    <p className="text-sm text-yellow-800 dark:text-yellow-200">
                      Repository not available in container. Run{" "}
                      <code className="px-1.5 py-0.5 bg-yellow-100 dark:bg-yellow-900/50 rounded text-xs font-mono">
                        catnip run
                      </code>{" "}
                      from the git repo on your host.
                    </p>
                  </div>
                </CardContent>
              )}
            </Card>
          );
        })}

        {repositoriesWithWorktrees.length === 0 && (
          <Card className="p-12 text-center">
            <CardContent>
              <Folder className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
              <h3 className="text-lg font-semibold mb-2">No repositories yet</h3>
              <p className="text-muted-foreground mb-4">
                Create your first repository to get started
              </p>
              <Button onClick={() => setNewWorkspaceDialogOpen(true)} size="lg">
                <Plus className="h-5 w-5 mr-2" />
                New repository
              </Button>
            </CardContent>
          </Card>
        )}
      </div>

      <NewWorkspaceDialog
        open={newWorkspaceDialogOpen}
        onOpenChange={setNewWorkspaceDialogOpen}
      />
    </div>
  );
}