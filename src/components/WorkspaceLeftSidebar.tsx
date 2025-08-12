import { Link, useParams } from "@tanstack/react-router";
import {
  ChevronRight,
  Folder,
  GitBranch,
  Plus,
  Settings,
  AlertTriangle,
  Trash2,
} from "lucide-react";
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
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useAppStore } from "@/stores/appStore";
import { useState, useMemo, useEffect } from "react";
import { NewWorkspaceDialog } from "@/components/NewWorkspaceDialog";
import { useGlobalKeyboardShortcuts } from "@/hooks/use-keyboard-shortcuts";
import { SettingsDialog } from "@/components/SettingsDialog";
import type { Worktree } from "@/lib/git-api";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogAction,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import { useGitApi } from "@/hooks/useGitApi";
import { useNavigate } from "@tanstack/react-router";

export function WorkspaceLeftSidebar() {
  const { project, workspace } = useParams({
    from: "/workspace/$project/$workspace",
  });

  // Global keyboard shortcuts
  const { newWorkspaceDialogOpen, setNewWorkspaceDialogOpen } =
    useGlobalKeyboardShortcuts();

  const [settingsOpen, setSettingsOpen] = useState(false);
  const [unavailableRepoAlert, setUnavailableRepoAlert] = useState<{
    open: boolean;
    repoName: string;
    repoId: string;
    worktrees: Worktree[];
  }>({ open: false, repoName: "", repoId: "", worktrees: [] });

  const [deleteConfirmDialog, setDeleteConfirmDialog] = useState<{
    open: boolean;
    worktrees: Worktree[];
    repoName: string;
  }>({ open: false, worktrees: [], repoName: "" });

  const [singleWorkspaceDeleteDialog, setSingleWorkspaceDeleteDialog] =
    useState<{
      open: boolean;
      worktreeId: string;
      worktreeName: string;
      hasChanges: boolean;
      commitCount: number;
    }>({
      open: false,
      worktreeId: "",
      worktreeName: "",
      hasChanges: false,
      commitCount: 0,
    });

  const { deleteWorktree } = useGitApi();
  const navigate = useNavigate();

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
        const worktrees = getWorktreesByRepo(repoId);
        // Sort worktrees by created_at (descending), fallback to last_accessed (descending), then name
        const sortedWorktrees = worktrees.sort((a, b) => {
          // Primary sort: created_at (descending - most recent first)
          const aCreated = new Date(a.created_at).getTime();
          const bCreated = new Date(b.created_at).getTime();
          if (aCreated !== bCreated) {
            return bCreated - aCreated;
          }

          // Secondary sort: last_accessed (descending - most recent first)
          const aAccessed = new Date(a.last_accessed).getTime();
          const bAccessed = new Date(b.last_accessed).getTime();
          if (aAccessed !== bAccessed) {
            return bAccessed - aAccessed;
          }

          // Tertiary sort: name (lexical)
          return a.name.localeCompare(b.name);
        });
        return repo ? { ...repo, worktrees: sortedWorktrees } : null;
      })
      .filter((repo): repo is NonNullable<typeof repo> => repo !== null)
      .sort((a, b) => {
        // Sort repositories by name in lexical order
        const nameA = a.name || a.id;
        const nameB = b.name || b.id;
        return nameA.localeCompare(nameB);
      });
  }, [worktreesCount, worktrees, getWorktreesByRepo, getRepository]);

  // Find current worktree to get its repo_id for expanded state
  const currentWorkspaceName = `${project}/${workspace}`;

  const [expandedRepos, setExpandedRepos] = useState<Set<string>>(new Set());
  const [selectedRepoForNewWorkspace, setSelectedRepoForNewWorkspace] =
    useState<{
      url: string;
      branch: string;
    } | null>(null);

  // Keep only available repositories expanded by default
  useEffect(() => {
    const availableRepos = repositoriesWithWorktrees
      .filter((repo) => repo.available !== false)
      .map((repo) => repo.id);
    setExpandedRepos(new Set(availableRepos));
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

  const handleDeleteWorkspaces = () => {
    // Show delete confirmation dialog
    setDeleteConfirmDialog({
      open: true,
      worktrees: unavailableRepoAlert.worktrees,
      repoName: unavailableRepoAlert.repoName,
    });
  };

  const handleDeleteConfirmed = async () => {
    try {
      // Delete all worktrees for this repository
      for (const worktree of deleteConfirmDialog.worktrees) {
        await deleteWorktree(worktree.id);
      }
      setDeleteConfirmDialog({ open: false, worktrees: [], repoName: "" });
      setUnavailableRepoAlert({
        open: false,
        repoName: "",
        repoId: "",
        worktrees: [],
      });

      // Navigate to workspace index if we deleted the current workspace
      const currentWorkspaceName = `${project}/${workspace}`;
      const wasCurrentDeleted = deleteConfirmDialog.worktrees.some(
        (w) => w.name === currentWorkspaceName,
      );
      if (wasCurrentDeleted) {
        void navigate({ to: "/workspace" });
      }
    } catch (error) {
      console.error("Failed to delete workspaces:", error);
    }
  };

  // Handle delete single workspace with confirmation
  const handleSingleWorkspaceDelete = (worktree: Worktree) => {
    setSingleWorkspaceDeleteDialog({
      open: true,
      worktreeId: worktree.id,
      worktreeName: worktree.name,
      hasChanges: worktree.is_dirty,
      commitCount: worktree.commit_count,
    });
  };

  const handleSingleWorkspaceDeleteConfirmed = async () => {
    try {
      await deleteWorktree(singleWorkspaceDeleteDialog.worktreeId);
      setSingleWorkspaceDeleteDialog({
        open: false,
        worktreeId: "",
        worktreeName: "",
        hasChanges: false,
        commitCount: 0,
      });

      // Navigate to workspace index if we deleted the current workspace
      const currentWorkspaceName = `${project}/${workspace}`;
      if (singleWorkspaceDeleteDialog.worktreeName === currentWorkspaceName) {
        void navigate({ to: "/workspace" });
      }
    } catch (error) {
      console.error("Failed to delete workspace:", error);
      // Keep dialog open on error so user can retry
    }
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
                  const isAvailable = repo.available !== false; // Default to true if not specified

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
                            className={`flex-1 ${!isAvailable ? "opacity-50" : ""}`}
                          >
                            <Folder className="h-4 w-4" />
                            <span className="truncate">{projectName}</span>
                          </SidebarMenuButton>
                          <CollapsibleTrigger asChild>
                            <SidebarMenuAction className="data-[state=open]:rotate-90">
                              <ChevronRight />
                            </SidebarMenuAction>
                          </CollapsibleTrigger>
                          {isAvailable ? (
                            <SidebarMenuAction
                              onClick={(e) => {
                                e.stopPropagation();
                                handleAddWorkspace(repo);
                              }}
                              className="hover:bg-accent mr-5"
                            >
                              <Plus className="h-4 w-4" />
                            </SidebarMenuAction>
                          ) : (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <SidebarMenuAction
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    setUnavailableRepoAlert({
                                      open: true,
                                      repoName: projectName,
                                      repoId: repo.id,
                                      worktrees: worktrees,
                                    });
                                  }}
                                  className="mr-5 text-yellow-500 hover:bg-accent cursor-pointer"
                                >
                                  <AlertTriangle className="h-4 w-4" />
                                </SidebarMenuAction>
                              </TooltipTrigger>
                              <TooltipContent
                                side="left"
                                className="max-w-xs space-y-1"
                              >
                                <div className="text-sm">
                                  Repo {projectName} isn't available in the
                                  container.
                                </div>
                                <div className="text-sm">
                                  Run{" "}
                                  <code className="inline-block bg-secondary text-secondary-foreground px-1.5 py-0.5 rounded text-xs font-mono">
                                    catnip run
                                  </code>{" "}
                                  from the git repo on your host to make it
                                  available.
                                </div>
                                <div className="text-sm text-muted-foreground">
                                  Click to open options
                                </div>
                              </TooltipContent>
                            </Tooltip>
                          )}
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
                                    asChild={isAvailable}
                                    isActive={isActive}
                                    className={
                                      !isAvailable
                                        ? "opacity-50 cursor-not-allowed"
                                        : ""
                                    }
                                    onClick={
                                      !isAvailable
                                        ? (e) => {
                                            e.preventDefault();
                                            setUnavailableRepoAlert({
                                              open: true,
                                              repoName: projectName,
                                              repoId: repo.id,
                                              worktrees: worktrees,
                                            });
                                          }
                                        : undefined
                                    }
                                  >
                                    {isAvailable ? (
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
                                    ) : (
                                      <span className="flex items-center gap-1.5 pr-2">
                                        <div
                                          className={`w-2 h-2 rounded-full ${status.color} flex-shrink-0`}
                                          title={status.label}
                                        />
                                        <span className="truncate">
                                          {worktree.name.split("/")[1] ||
                                            worktree.name}
                                        </span>
                                        {worktree.branch && (
                                          <span className="ml-auto flex items-center gap-0.5">
                                            <GitBranch className="h-3 w-3 text-muted-foreground/70" />
                                            <span className="text-xs text-muted-foreground truncate max-w-24">
                                              {worktree.branch}
                                            </span>
                                          </span>
                                        )}
                                      </span>
                                    )}
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
      <AlertDialog
        open={unavailableRepoAlert.open}
        onOpenChange={(open) =>
          setUnavailableRepoAlert({ ...unavailableRepoAlert, open })
        }
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Repository Not Available</AlertDialogTitle>
            <AlertDialogDescription>
              Repo {unavailableRepoAlert.repoName} isn't available in the
              container. Run `catnip run` from the git repo on your host to make
              it available within the container.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeleteWorkspaces}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete Workspaces
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog
        open={deleteConfirmDialog.open}
        onOpenChange={(open) =>
          setDeleteConfirmDialog({ ...deleteConfirmDialog, open })
        }
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Confirm Delete</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete{" "}
              {deleteConfirmDialog.worktrees.length} workspace
              {deleteConfirmDialog.worktrees.length > 1 ? "s" : ""} for "
              {deleteConfirmDialog.repoName}"? This action cannot be undone.
              {deleteConfirmDialog.worktrees.some(
                (w) => w.is_dirty || w.commit_count > 0,
              ) && (
                <div className="mt-2 text-yellow-600">
                  Warning: Some workspaces have uncommitted changes or unpushed
                  commits.
                </div>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeleteConfirmed}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete All Workspaces
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
