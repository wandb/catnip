import { useNavigate } from "@tanstack/react-router";
import { Folder, Plus, Settings, ChevronRight } from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { useAppStore } from "@/stores/appStore";
import { useState, useMemo } from "react";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import { useGlobalKeyboardShortcuts } from "@/hooks/use-keyboard-shortcuts";
import { SettingsDialog } from "@/components/SettingsDialog";

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

interface RepositoryWithWorktrees extends Repository {
  worktrees: Worktree[];
  projectName: string;
  kittyCount: number;
  lastActivity: string;
}

export function RepositoryListSidebar() {
  const navigate = useNavigate();

  // Global keyboard shortcuts
  const { newWorkspaceDialogOpen, setNewWorkspaceDialogOpen } =
    useGlobalKeyboardShortcuts();

  const [settingsOpen, setSettingsOpen] = useState(false);

  // Use stable selectors to avoid infinite loops
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
    const repoIds = new Set(worktreesList.map((w: Worktree) => w.repo_id));

    return Array.from(repoIds)
      .map((repoId) => {
        const repo = getRepository(repoId) as Repository | undefined;
        const worktrees = getWorktreesByRepo(repoId) as Worktree[];
        
        if (!repo) return null;
        
        // Get the most recent worktree for this project
        const sortedWorktrees = worktrees.sort((a, b) => {
          const aAccessed = new Date(a.last_accessed).getTime();
          const bAccessed = new Date(b.last_accessed).getTime();
          return bAccessed - aAccessed;
        });
        
        const mostRecentWorktree = sortedWorktrees[0];
        const projectName = mostRecentWorktree ? mostRecentWorktree.name.split("/")[0] : repo?.name || repo?.id;
        
        return { 
          ...repo, 
          worktrees: sortedWorktrees, 
          projectName,
          kittyCount: worktrees.length,
          lastActivity: mostRecentWorktree ? mostRecentWorktree.last_accessed : repo.id
        } as RepositoryWithWorktrees;
      })
      .filter((repo): repo is RepositoryWithWorktrees => repo !== null)
      .sort((a, b) => {
        // Sort repositories by name in lexical order
        const nameA = a.projectName || a.name || a.id;
        const nameB = b.projectName || b.name || b.id;
        return nameA.localeCompare(nameB);
      });
  }, [worktreesCount, worktrees, getWorktreesByRepo, getRepository]);

  const handleRepositoryClick = (repo: RepositoryWithWorktrees) => {
    if (!repo.available) return;
    
    // Navigate to the project route, which will redirect to the most recent workspace
    const projectName = repo.projectName;
    void navigate({
      to: "/workspace/$project",
      params: { project: projectName },
    });
  };

  const getTimeAgo = (timestamp: string) => {
    const now = new Date();
    const time = new Date(timestamp);
    const diff = now.getTime() - time.getTime();
    const minutes = Math.floor(diff / (1000 * 60));
    const hours = Math.floor(diff / (1000 * 60 * 60));
    const days = Math.floor(diff / (1000 * 60 * 60 * 24));
    const weeks = Math.floor(diff / (1000 * 60 * 60 * 24 * 7));

    if (minutes < 1) return "Just now";
    if (minutes < 60) return `${minutes} minutes ago`;
    if (hours < 24) return `${hours} hours ago`;
    if (days < 7) return `${days === 1 ? "1 day" : `${days} days`} ago`;
    return `${weeks === 1 ? "1 week" : `${weeks} weeks`} ago`;
  };

  return (
    <>
      <Sidebar className="border-r-0">
        <SidebarHeader className="relative">
          <div className="absolute top-2 right-2 z-10 mt-0 -mr-1">
            <SidebarTrigger className="h-6 w-6" />
          </div>
          <div className="flex items-center gap-2">
            <img 
              src="/logo@2x.png" 
              alt="Catnip" 
              className="w-9 h-9" 
              role="img"
              aria-label="Catnip logo"
            />
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <div className="flex items-center justify-between mb-2">
              <SidebarGroupLabel>Repositories</SidebarGroupLabel>
              <span className="text-xs text-muted-foreground" aria-label={`${repositoriesWithWorktrees.length} repositories`}>
                {repositoriesWithWorktrees.length}
              </span>
            </div>
            <SidebarGroupContent>
              {/* New Repository Button at the top */}
              <SidebarMenu className="mb-3">
                <SidebarMenuItem>
                  <SidebarMenuButton
                    onClick={() => setNewWorkspaceDialogOpen(true)}
                    className="flex items-center gap-2 text-muted-foreground hover:text-foreground text-xs"
                    aria-label="Create new repository"
                  >
                    <Plus className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
                    <span className="truncate">New repository</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>

              <SidebarMenu>
                {repositoriesWithWorktrees.map((repo) => {
                  const isAvailable = repo.available !== false;
                  
                  return (
                    <SidebarMenuItem key={repo.id}>
                      <div className="flex items-center w-full">
                        <SidebarMenuButton
                          onClick={() => handleRepositoryClick(repo)}
                          className={`flex-1 justify-between ${!isAvailable ? "opacity-50" : ""}`}
                          disabled={!isAvailable}
                          aria-label={`Open repository ${repo.projectName}${!isAvailable ? ' (unavailable)' : ''}`}
                        >
                          <div className="flex items-center gap-2 min-w-0">
                            <Folder className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
                            <span className="truncate">{repo.projectName}</span>
                          </div>
                        </SidebarMenuButton>
                        <SidebarMenuAction className="ml-2" aria-label="Navigate to repository">
                          <ChevronRight className="h-4 w-4" aria-hidden="true" />
                        </SidebarMenuAction>
                      </div>
                      <div className="px-2 pb-1">
                        <div className="text-xs text-muted-foreground flex items-center justify-between">
                          <span aria-label={`${repo.kittyCount} workspaces`}>{repo.kittyCount} kitties</span>
                          <span>• {getTimeAgo(repo.lastActivity)}</span>
                        </div>
                      </div>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <div className="mt-auto p-2">
          <SidebarMenuButton
            onClick={() => setSettingsOpen(true)}
            className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
            aria-label="Open settings"
          >
            <Settings className="h-4 w-4" aria-hidden="true" />
            <span>Settings</span>
          </SidebarMenuButton>
        </div>
        <SidebarRail />
      </Sidebar>
      <NewWorkspaceDialog
        open={newWorkspaceDialogOpen}
        onOpenChange={setNewWorkspaceDialogOpen}
      />
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </>
  );
}