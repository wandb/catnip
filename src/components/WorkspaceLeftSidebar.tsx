import { Link, useParams } from "@tanstack/react-router";
import { ChevronRight, Folder, GitBranch, Plus, Settings } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
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
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAppStore } from "@/stores/appStore";
import { useState, useMemo, useEffect } from "react";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import { useGlobalKeyboardShortcuts } from "@/hooks/use-keyboard-shortcuts";
import { SettingsDialog } from "@/components/SettingsDialog";
import type { Worktree } from "@/lib/git-api";

export function WorkspaceLeftSidebar() {
  const { project, workspace } = useParams({
    from: "/workspace/$project/$workspace",
  });

  // Global keyboard shortcuts
  const { newWorkspaceDialogOpen, setNewWorkspaceDialogOpen } =
    useGlobalKeyboardShortcuts();

  const [settingsOpen, setSettingsOpen] = useState(false);

  // Use stable selectors to avoid infinite loops
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );
  // Subscribe to the actual worktrees map to get updates when individual worktrees change
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
        return repo ? { ...repo, worktrees: getWorktreesByRepo(repoId) } : null;
      })
      .filter((repo): repo is NonNullable<typeof repo> => repo !== null);
  }, [worktreesCount, worktrees, getWorktreesByRepo, getRepository]);

  // Find current worktree to get its repo_id for expanded state
  const currentWorkspaceName = `${project}/${workspace}`;

  const [expandedRepos, setExpandedRepos] = useState<Set<string>>(new Set());
  const [selectedRepoForNewWorkspace, setSelectedRepoForNewWorkspace] =
    useState<{
      url: string;
      branch: string;
    } | null>(null);

  // Keep all repositories with worktrees expanded by default
  useEffect(() => {
    setExpandedRepos(new Set(repositoriesWithWorktrees.map((repo) => repo.id)));
  }, [repositoriesWithWorktrees]);

  const toggleRepo = (repoIdToToggle: string) => {
    const newExpanded = new Set(expandedRepos);
    if (newExpanded.has(repoIdToToggle)) {
      newExpanded.delete(repoIdToToggle);
    } else {
      newExpanded.add(repoIdToToggle);
    }
    setExpandedRepos(newExpanded);
  };

  const getWorktreeStatus = (worktree: Worktree) => {
    // Use the claude_activity_state to determine the status
    switch (worktree.claude_activity_state) {
      case "active":
        return { color: "bg-green-500 animate-pulse", label: "active" };
      case "running":
        return { color: "bg-blue-500 animate-pulse", label: "running" };
      case "inactive":
      default:
        return { color: "bg-gray-500", label: "inactive" };
    }
  };

  const handleAddWorkspace = (repo: any) => {
    let repoUrl = repo.url || repo.id;

    // Convert file:// URLs to local/ format for the modal
    if (repoUrl.startsWith("file://")) {
      repoUrl = repo.id; // Use the repo.id which should be in local/... format
    }

    setSelectedRepoForNewWorkspace({
      url: repoUrl,
      branch: repo.default_branch || "main",
    });
    setNewWorkspaceDialogOpen(true);
  };

  return (
    <>
      <Sidebar className="border-r-0">
        <SidebarHeader className="relative">
          <div className="absolute top-2 right-2 z-10 mt-0 -mr-1">
            <SidebarTrigger className="h-6 w-6" />
          </div>
          <div className="flex items-center gap-2">
            <img src="/logo@2x.png" alt="Catnip" className="w-9 h-9" />
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Workspaces</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {repositoriesWithWorktrees.map((repo) => {
                  const worktrees = repo.worktrees;
                  const isExpanded = expandedRepos.has(repo.id);

                  // Get project name from the first worktree
                  const projectName =
                    worktrees.length > 0
                      ? worktrees[0].name.split("/")[0]
                      : repo.name;

                  return (
                    <Collapsible
                      key={repo.id}
                      open={isExpanded}
                      onOpenChange={() => toggleRepo(repo.id)}
                    >
                      <SidebarMenuItem>
                        <div className="flex items-center w-full">
                          <SidebarMenuButton
                            onClick={() => toggleRepo(repo.id)}
                            className="flex-1"
                          >
                            <Folder className="h-4 w-4" />
                            <span className="truncate">{projectName}</span>
                          </SidebarMenuButton>
                          <SidebarMenuAction
                            onClick={(e) => {
                              e.stopPropagation();
                              handleAddWorkspace(repo);
                            }}
                            className="hover:bg-accent"
                            showOnHover
                          >
                            <Plus className="h-4 w-4" />
                          </SidebarMenuAction>
                          <CollapsibleTrigger asChild>
                            <SidebarMenuAction
                              className="data-[state=open]:rotate-90"
                              showOnHover
                            >
                              <ChevronRight />
                            </SidebarMenuAction>
                          </CollapsibleTrigger>
                        </div>
                        <CollapsibleContent>
                          <SidebarMenuSub className="mx-0 mr-0">
                            {worktrees.map((worktree: Worktree) => {
                              const isActive =
                                worktree.name === currentWorkspaceName;
                              const nameParts = worktree.name.split("/");
                              const status = getWorktreeStatus(worktree);
                              return (
                                <SidebarMenuSubItem key={worktree.id}>
                                  <SidebarMenuSubButton
                                    asChild
                                    isActive={isActive}
                                  >
                                    <Link
                                      to="/workspace/$project/$workspace"
                                      params={{
                                        project: nameParts[0],
                                        workspace: nameParts[1],
                                      }}
                                      className="flex items-center gap-1.5 pr-2"
                                    >
                                      <div
                                        className={`w-2 h-2 rounded-full ${status.color} flex-shrink-0`}
                                        title={status.label}
                                      />
                                      <span className="truncate">
                                        {worktree.name.split("/")[1] ||
                                          worktree.name}
                                      </span>
                                      {worktree.branch && (
                                        <Tooltip>
                                          <TooltipTrigger asChild>
                                            <div className="ml-auto flex items-center gap-0.5">
                                              <GitBranch className="h-3 w-3 text-muted-foreground/70" />
                                              <span className="text-xs text-muted-foreground truncate max-w-24">
                                                {worktree.branch}
                                              </span>
                                            </div>
                                          </TooltipTrigger>
                                          <TooltipContent
                                            side="right"
                                            align="center"
                                          >
                                            <div className="text-xs">
                                              {worktree.branch}
                                            </div>
                                          </TooltipContent>
                                        </Tooltip>
                                      )}
                                    </Link>
                                  </SidebarMenuSubButton>
                                </SidebarMenuSubItem>
                              );
                            })}
                          </SidebarMenuSub>
                        </CollapsibleContent>
                      </SidebarMenuItem>
                    </Collapsible>
                  );
                })}
              </SidebarMenu>
              {/* Global New Workspace Button */}
              <SidebarMenu className="mt-3">
                <SidebarMenuItem>
                  <SidebarMenuButton
                    onClick={() => setNewWorkspaceDialogOpen(true)}
                    className="flex items-center gap-2 text-muted-foreground hover:text-foreground text-xs"
                  >
                    <Plus className="h-4 w-4 flex-shrink-0" />
                    <span className="truncate">New workspace</span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      ⌘N
                    </span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <div className="mt-auto p-2">
          <SidebarMenuButton
            onClick={() => setSettingsOpen(true)}
            className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
          >
            <Settings className="h-4 w-4" />
            <span>Settings</span>
          </SidebarMenuButton>
        </div>
        <SidebarRail />
      </Sidebar>
      <NewWorkspaceDialog
        open={newWorkspaceDialogOpen}
        onOpenChange={(open) => {
          setNewWorkspaceDialogOpen(open);
          if (!open) {
            setSelectedRepoForNewWorkspace(null);
          }
        }}
        initialRepoUrl={selectedRepoForNewWorkspace?.url}
        initialBranch={selectedRepoForNewWorkspace?.branch}
      />
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </>
  );
}
