import { Link, useParams } from "@tanstack/react-router";
import {
  ArrowLeft,
  GitBranch,
  Plus,
  Settings,
  Trash2,
  ExternalLink,
  MoreHorizontal,
  ChevronsLeft,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { useAppStore } from "@/stores/appStore";
import { useState, useMemo } from "react";
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
import { formatDistanceToNow } from "date-fns";
import { useIsMobile } from "@/hooks/use-mobile";
import { useSidebar } from "@/hooks/use-sidebar";

export function WorkspaceLeftSidebar() {
  const { project, workspace } = useParams({
    from: "/workspace/$project/$workspace",
  });

  // Sidebar controls
  const { toggleSidebar } = useSidebar();

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
  const isMobile = useIsMobile();

  // Use stable selectors to avoid infinite loops
  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );
  // Subscribe to the actual worktrees map to get updates when individual worktrees change
  const worktrees = useAppStore((state) => state.worktrees);
  const getWorktreesByRepo = useAppStore((state) => state.getWorktreesByRepo);
  const getRepository = useAppStore((state) => state.getRepositoryById);

  // Find current worktree and repository
  const currentWorkspaceName = `${project}/${workspace}`;
  const currentWorktree = useMemo(() => {
    const worktreesList = useAppStore.getState().getWorktreesList();
    return worktreesList.find((w) => w.name === currentWorkspaceName);
  }, [currentWorkspaceName, worktrees]);

  // Get current repository and its worktrees
  const currentRepository = useMemo(() => {
    if (!currentWorktree) return null;
    return getRepository(currentWorktree.repo_id);
  }, [currentWorktree, getRepository]);

  const repositoryWorktrees = useMemo(() => {
    if (!currentRepository) return [];
    const worktrees = getWorktreesByRepo(currentRepository.id);
    // Sort worktrees by created_at (descending), fallback to last_accessed (descending)
    return worktrees.sort((a, b) => {
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
  }, [currentRepository, getWorktreesByRepo, worktrees]);

  // For mobile, get all repositories with worktrees
  const allRepositoriesWithWorktrees = useMemo(() => {
    if (!isMobile || worktreesCount === 0) return [];

    const worktreesList = useAppStore.getState().getWorktreesList();
    const repoIds = new Set(worktreesList.map((w) => w.repo_id));

    return Array.from(repoIds)
      .map((repoId) => {
        const repo = getRepository(repoId);
        const worktrees = getWorktreesByRepo(repoId);
        const sortedWorktrees = worktrees.sort((a, b) => {
          const aCreated = new Date(a.created_at).getTime();
          const bCreated = new Date(b.created_at).getTime();
          if (aCreated !== bCreated) {
            return bCreated - aCreated;
          }
          const aAccessed = new Date(a.last_accessed).getTime();
          const bAccessed = new Date(b.last_accessed).getTime();
          if (aAccessed !== bAccessed) {
            return bAccessed - aAccessed;
          }
          return a.name.localeCompare(b.name);
        });
        return repo ? { ...repo, worktrees: sortedWorktrees } : null;
      })
      .filter((repo): repo is NonNullable<typeof repo> => repo !== null)
      .sort((a, b) => {
        const nameA = a.name || a.id;
        const nameB = b.name || b.id;
        return nameA.localeCompare(nameB);
      });
  }, [isMobile, worktreesCount, worktrees, getWorktreesByRepo, getRepository]);

  const [selectedRepoForNewWorkspace, setSelectedRepoForNewWorkspace] =
    useState<{
      url: string;
      branch: string;
    } | null>(null);

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

  const handleAddWorkspace = () => {
    if (!currentRepository) return;

    let repoUrl = currentRepository.url || currentRepository.id;

    // Convert file:// URLs to local/ format for the modal
    if (repoUrl.startsWith("file://")) {
      repoUrl = currentRepository.id; // Use the repo.id which should be in local/... format
    }

    setSelectedRepoForNewWorkspace({
      url: repoUrl,
      branch: currentRepository.default_branch || "main",
    });
    setNewWorkspaceDialogOpen(true);
  };

  const getTimeAgo = (worktree: Worktree) => {
    const lastActivity = worktree.last_accessed || worktree.created_at;
    if (!lastActivity) return null;
    return formatDistanceToNow(new Date(lastActivity), { addSuffix: true });
  };

  // Generate a workspace title with proper priority
  const getWorkspaceTitle = (worktree: Worktree) => {
    // Priority 1: Use PR title if available
    if (worktree.pull_request_title) {
      return worktree.pull_request_title;
    }

    // Priority 2: Use session title if available
    if (worktree.session_title?.title) {
      return worktree.session_title.title;
    }

    // Priority 3: Use workspace name
    const workspaceName = worktree.name.split("/")[1] || worktree.name;
    return workspaceName;
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

      // Close dialog after successful deletion
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

  // For mobile, render the old multi-repository view
  if (isMobile) {
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
              <SidebarGroupLabel>All Workspaces</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {allRepositoriesWithWorktrees.map((repo) => {
                    const worktrees = repo.worktrees;
                    const isAvailable = repo.available !== false;

                    return worktrees.map((worktree: Worktree) => {
                      const isActive = worktree.name === currentWorkspaceName;
                      const nameParts = worktree.name.split("/");
                      const status = getWorktreeStatus(worktree);

                      return (
                        <SidebarMenuItem key={worktree.id}>
                          <SidebarMenuButton
                            asChild={isAvailable}
                            isActive={isActive}
                            className={!isAvailable ? "opacity-50" : ""}
                          >
                            {isAvailable ? (
                              <Link
                                to="/workspace/$project/$workspace"
                                params={{
                                  project: nameParts[0],
                                  workspace: nameParts[1],
                                }}
                                search={{ prompt: undefined }}
                                className="flex items-center gap-2"
                              >
                                <div
                                  className={`w-2 h-2 rounded-full ${status.color} flex-shrink-0`}
                                  title={status.label}
                                />
                                <div className="flex-1 min-w-0">
                                  <div className="font-medium truncate">
                                    {getWorkspaceTitle(worktree)}
                                  </div>
                                  <div className="text-xs text-muted-foreground flex items-center gap-1">
                                    <GitBranch className="h-3 w-3 flex-shrink-0" />
                                    {worktree.pull_request_url ? (
                                      <button
                                        type="button"
                                        onClick={(e) => {
                                          e.stopPropagation();
                                          e.preventDefault();
                                          window.open(
                                            worktree.pull_request_url,
                                            "_blank",
                                            "noopener,noreferrer",
                                          );
                                        }}
                                        className="truncate text-blue-500 hover:text-blue-600 transition-colors flex items-center gap-0.5 bg-transparent border-none p-0 cursor-pointer"
                                        title={`${worktree.branch} (PR #${worktree.pull_request_url.match(/\/pull\/(\d+)/)?.[1] || "?"})`}
                                      >
                                        <span className="truncate">
                                          {worktree.branch}
                                        </span>
                                        <ExternalLink className="h-2.5 w-2.5 flex-shrink-0" />
                                      </button>
                                    ) : (
                                      <span
                                        className="truncate"
                                        title={worktree.branch}
                                      >
                                        {worktree.branch}
                                      </span>
                                    )}
                                  </div>
                                </div>
                              </Link>
                            ) : (
                              <span className="flex items-center gap-2">
                                <div
                                  className={`w-2 h-2 rounded-full ${status.color} flex-shrink-0`}
                                  title={status.label}
                                />
                                <div className="flex-1 min-w-0">
                                  <div className="font-medium truncate">
                                    {getWorkspaceTitle(worktree)}
                                  </div>
                                  <div className="text-xs text-muted-foreground flex items-center gap-1">
                                    <GitBranch className="h-3 w-3 flex-shrink-0" />
                                    <span
                                      className="truncate"
                                      title={worktree.branch}
                                    >
                                      {worktree.branch}
                                    </span>
                                  </div>
                                </div>
                              </span>
                            )}
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      );
                    });
                  })}
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

  // Desktop view - single repository
  const projectName = currentWorktree ? currentWorktree.name.split("/")[0] : "";
  const isAvailable = currentRepository?.available !== false;

  return (
    <>
      <Sidebar className="border-r-0 w-80">
        <SidebarHeader className="relative">
          <div className="absolute top-2 right-2 z-10 mt-0 -mr-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0"
              onClick={toggleSidebar}
            >
              <ChevronsLeft className="h-4 w-4" />
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <img src="/logo@2x.png" alt="Catnip" className="w-9 h-9" />
          </div>
        </SidebarHeader>
        <SidebarContent>
          {/* Repo name with back button */}
          <div className="px-3 pb-2">
            <div className="flex items-center gap-2">
              <Button
                variant="ghost"
                size="sm"
                className="h-8 w-8 p-0 text-muted-foreground hover:text-foreground"
                onClick={() => navigate({ to: "/workspace/repos" })}
              >
                <ArrowLeft className="h-4 w-4" />
              </Button>
              <span className="font-medium text-foreground">{projectName}</span>
            </div>
          </div>

          <SidebarGroup>
            <SidebarGroupLabel className="flex items-center justify-between">
              <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                WORKSPACES
              </span>
              {isAvailable && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-muted-foreground hover:text-foreground flex items-center gap-1"
                  onClick={handleAddWorkspace}
                >
                  <Plus className="h-3 w-3" />
                  <span className="text-xs">New workspace</span>
                </Button>
              )}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              {!isAvailable && (
                <div className="px-3 py-2 mb-2">
                  <div className="bg-yellow-50 dark:bg-yellow-950/20 border border-yellow-200 dark:border-yellow-900 rounded-lg p-2">
                    <p className="text-xs text-yellow-800 dark:text-yellow-200">
                      Repository not available. Run{" "}
                      <code className="px-1 py-0.5 bg-yellow-100 dark:bg-yellow-900/50 rounded text-xs font-mono">
                        catnip run
                      </code>{" "}
                      from the git repo.
                    </p>
                  </div>
                </div>
              )}
              <SidebarMenu>
                {repositoryWorktrees.map((worktree: Worktree) => {
                  const isActive = worktree.name === currentWorkspaceName;
                  const nameParts = worktree.name.split("/");
                  const status = getWorktreeStatus(worktree);
                  const timeAgo = getTimeAgo(worktree);
                  const title = getWorkspaceTitle(worktree);

                  return (
                    <SidebarMenuItem key={worktree.id}>
                      <SidebarMenuButton
                        asChild={isAvailable}
                        isActive={isActive}
                        className={`h-auto py-3 ${!isAvailable ? "opacity-50 cursor-not-allowed" : ""}`}
                        onClick={
                          !isAvailable
                            ? (e) => {
                                e.preventDefault();
                                setUnavailableRepoAlert({
                                  open: true,
                                  repoName: projectName,
                                  repoId: currentRepository?.id || "",
                                  worktrees: repositoryWorktrees,
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
                            search={{ prompt: undefined }}
                            className="flex items-start gap-3 w-full"
                          >
                            <div
                              className={`w-2 h-2 rounded-full ${status.color} flex-shrink-0 mt-1.5`}
                              title={status.label}
                            />
                            <div className="flex-1 min-w-0 relative">
                              {/* Floating Actions Menu */}
                              <div className="absolute top-0 right-0 z-10">
                                <DropdownMenu>
                                  <DropdownMenuTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      className="h-6 w-6 p-0 opacity-0 group-hover:opacity-100 transition-opacity"
                                      onClick={(e) => {
                                        e.stopPropagation();
                                        e.preventDefault();
                                      }}
                                    >
                                      <MoreHorizontal className="h-3.5 w-3.5" />
                                    </Button>
                                  </DropdownMenuTrigger>
                                  <DropdownMenuContent align="end">
                                    <DropdownMenuItem
                                      onClick={(e) => {
                                        e.stopPropagation();
                                        e.preventDefault();
                                        handleSingleWorkspaceDelete(worktree);
                                      }}
                                      className="text-red-600"
                                    >
                                      <Trash2 className="h-4 w-4 mr-2" />
                                      Delete Workspace
                                    </DropdownMenuItem>
                                  </DropdownMenuContent>
                                </DropdownMenu>
                              </div>

                              {/* Title */}
                              <div className="mb-0.5 pr-8">
                                <span
                                  className={`font-medium truncate block ${
                                    worktree.pull_request_state === "CLOSED" ||
                                    worktree.pull_request_state === "MERGED"
                                      ? "line-through opacity-60"
                                      : ""
                                  }`}
                                >
                                  {title}
                                </span>
                              </div>

                              {/* Sub-header - Full Width */}
                              <div className="flex items-center gap-2 text-xs text-muted-foreground overflow-hidden">
                                <div className="flex items-center gap-1 min-w-0 max-w-[60%]">
                                  <GitBranch className="h-3 w-3 flex-shrink-0" />
                                  {worktree.pull_request_url ? (
                                    <button
                                      type="button"
                                      onClick={(e) => {
                                        e.stopPropagation();
                                        e.preventDefault();
                                        window.open(
                                          worktree.pull_request_url,
                                          "_blank",
                                          "noopener,noreferrer",
                                        );
                                      }}
                                      className={`truncate text-blue-500 hover:text-blue-600 transition-colors flex items-center gap-0.5 bg-transparent border-none p-0 cursor-pointer ${
                                        worktree.pull_request_state ===
                                          "CLOSED" ||
                                        worktree.pull_request_state === "MERGED"
                                          ? "line-through opacity-60"
                                          : ""
                                      }`}
                                      title={`${worktree.branch} (PR #${worktree.pull_request_url.match(/\/pull\/(\d+)/)?.[1] || "?"})`}
                                    >
                                      <span className="truncate">
                                        {worktree.branch}
                                      </span>
                                      <ExternalLink className="h-2.5 w-2.5 flex-shrink-0" />
                                    </button>
                                  ) : (
                                    <span
                                      className="truncate"
                                      title={worktree.branch}
                                    >
                                      {worktree.branch}
                                    </span>
                                  )}
                                </div>
                                {timeAgo && (
                                  <>
                                    <span className="text-muted-foreground/50 flex-shrink-0">
                                      ·
                                    </span>
                                    <span className="truncate">{timeAgo}</span>
                                  </>
                                )}
                              </div>
                            </div>
                          </Link>
                        ) : (
                          <span className="flex items-start gap-3 w-full">
                            <div
                              className={`w-2 h-2 rounded-full ${status.color} flex-shrink-0 mt-1.5`}
                              title={status.label}
                            />
                            <div className="flex-1 min-w-0">
                              <div
                                className={`font-medium truncate mb-0.5 ${
                                  worktree.pull_request_state === "CLOSED" ||
                                  worktree.pull_request_state === "MERGED"
                                    ? "line-through opacity-60"
                                    : ""
                                }`}
                              >
                                {title}
                              </div>
                              <div className="flex items-center gap-2 text-xs text-muted-foreground overflow-hidden">
                                <div className="flex items-center gap-1 min-w-0 max-w-[50%]">
                                  <GitBranch className="h-3 w-3 flex-shrink-0" />
                                  <span
                                    className="truncate"
                                    title={worktree.branch}
                                  >
                                    {worktree.branch}
                                  </span>
                                </div>
                                {timeAgo && (
                                  <>
                                    <span className="text-muted-foreground/50 flex-shrink-0">
                                      ·
                                    </span>
                                    <span className="truncate">{timeAgo}</span>
                                  </>
                                )}
                              </div>
                            </div>
                          </span>
                        )}
                      </SidebarMenuButton>
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

      {/* Single Workspace Delete Confirmation Dialog */}
      <AlertDialog
        open={singleWorkspaceDeleteDialog.open}
        onOpenChange={(open) =>
          setSingleWorkspaceDeleteDialog({
            ...singleWorkspaceDeleteDialog,
            open,
          })
        }
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Workspace</AlertDialogTitle>
            <AlertDialogDescription>
              {(() => {
                const changesList = [];
                if (singleWorkspaceDeleteDialog.hasChanges)
                  changesList.push("uncommitted changes");
                if (singleWorkspaceDeleteDialog.commitCount > 0)
                  changesList.push(
                    `${singleWorkspaceDeleteDialog.commitCount} commits`,
                  );

                return changesList.length > 0 ? (
                  <>
                    Delete workspace "{singleWorkspaceDeleteDialog.worktreeName}
                    "? This workspace has{" "}
                    <strong>{changesList.join(" and ")}</strong>. This action
                    cannot be undone.
                  </>
                ) : (
                  <>
                    Delete workspace "{singleWorkspaceDeleteDialog.worktreeName}
                    "? This action cannot be undone.
                  </>
                );
              })()}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleSingleWorkspaceDeleteConfirmed}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete Workspace
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
