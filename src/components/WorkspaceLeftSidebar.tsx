import { Link, useParams } from "@tanstack/react-router";
import { ChevronRight, Folder, GitBranch, Plus } from "lucide-react";
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
import { useAppStore } from "@/stores/appStore";
import { useState, useMemo, useEffect } from "react";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import { useGlobalKeyboardShortcuts } from "@/hooks/use-keyboard-shortcuts";

export function WorkspaceLeftSidebar() {
  const { project, workspace } = useParams({
    from: "/workspace/$project/$workspace",
  });

  // Global keyboard shortcuts
  const { newWorkspaceDialogOpen, setNewWorkspaceDialogOpen } =
    useGlobalKeyboardShortcuts();

  // Use stable selectors to avoid infinite loops
  const repositoriesCount = useAppStore(
    (state) => state.getRepositoriesList().length,
  );
  const getWorktreesByRepo = useAppStore((state) => state.getWorktreesByRepo);

  // Get repositories using direct store access to avoid array reference changes
  const repositories = useMemo(() => {
    if (repositoriesCount === 0) return [];
    return useAppStore.getState().getRepositoriesList();
  }, [repositoriesCount]);

  // Find current worktree to get its repo_id for expanded state
  const currentWorkspaceName = `${project}/${workspace}`;

  const [expandedRepos, setExpandedRepos] = useState<Set<string>>(new Set());

  // Keep all repositories expanded by default
  useEffect(() => {
    setExpandedRepos(new Set(repositories.map((repo) => repo.id)));
  }, [repositories]);

  const toggleRepo = (repoIdToToggle: string) => {
    const newExpanded = new Set(expandedRepos);
    if (newExpanded.has(repoIdToToggle)) {
      newExpanded.delete(repoIdToToggle);
    } else {
      newExpanded.add(repoIdToToggle);
    }
    setExpandedRepos(newExpanded);
  };

  const getWorktreeStatus = (worktree: any) => {
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

  return (
    <>
      <Sidebar className="border-r-0 relative">
        {/* Sidebar trigger in upper right corner */}
        <div className="absolute top-2 right-2 z-10">
          <SidebarTrigger className="h-6 w-6" />
        </div>
        <SidebarHeader className="px-3 py-2">
          <div className="flex items-center gap-2">
            <img src="/logo@2x.png" alt="Catnip" className="w-9 h-9" />
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Repositories</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {repositories.map((repo) => {
                  const worktrees = getWorktreesByRepo(repo.id);
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
                        <SidebarMenuButton asChild>
                          <button className="w-full">
                            <Folder className="h-4 w-4" />
                            <span className="truncate">{projectName}</span>
                            <span className="ml-auto text-xs text-muted-foreground">
                              {worktrees.length}
                            </span>
                          </button>
                        </SidebarMenuButton>
                        <CollapsibleTrigger asChild>
                          <SidebarMenuAction
                            className="data-[state=open]:rotate-90"
                            showOnHover
                          >
                            <ChevronRight />
                          </SidebarMenuAction>
                        </CollapsibleTrigger>
                        <CollapsibleContent>
                          <SidebarMenuSub className="mx-0 mr-0">
                            {worktrees.map((worktree) => {
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
                                        <div className="ml-auto flex items-center gap-0.5">
                                          <GitBranch className="h-3 w-3 text-muted-foreground/70" />
                                          <span className="text-xs text-muted-foreground truncate max-w-24">
                                            {worktree.branch}
                                          </span>
                                        </div>
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
        <SidebarRail />
      </Sidebar>
      <NewWorkspaceDialog
        open={newWorkspaceDialogOpen}
        onOpenChange={setNewWorkspaceDialogOpen}
      />
    </>
  );
}
