import { useMemo } from "react";
import {
  FileText,
  GitBranch,
  CheckCircle,
  Circle,
  AlertCircle,
  Plus,
  Minus,
  RotateCw,
  Eye,
  Terminal,
  Globe,
  ExternalLink,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarSeparator,
} from "@/components/ui/sidebar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useWorktreeDiff } from "@/hooks/useWorktreeDiff";
import { useGitApi } from "@/hooks/useGitApi";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { WorkspaceActions } from "@/components/WorkspaceActions";
import { useAppStore } from "@/stores/appStore";
import type { Worktree, LocalRepository } from "@/lib/git-api";

interface WorkspaceRightSidebarProps {
  worktree: Worktree;
  repository: LocalRepository;
  showDiffView: boolean;
  setShowDiffView: (showDiff: boolean) => void;
  showPortPreview: number | null;
  setShowPortPreview: (port: number | null) => void;
  setSelectedFile?: (file: string | undefined) => void;
  onSync?: (id: string) => void;
}

function GitStatus({ worktree }: { worktree: Worktree }) {
  const getStatusColor = () => {
    if (worktree.has_conflicts) return "text-red-500";
    if (worktree.is_dirty) return "text-yellow-500";
    return "text-green-500";
  };

  const getStatusText = () => {
    if (worktree.has_conflicts) return "Conflicts";
    if (worktree.is_dirty) return "Dirty";
    return "Clean";
  };

  return (
    <SidebarGroup>
      <SidebarGroupContent>
        <div className="space-y-3">
          {/* Branch info */}
          <div className="flex items-center gap-2">
            <GitBranch className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm font-medium truncate">
              {worktree.branch || "detached"}
            </span>
          </div>

          {/* Status and commit counts on same line */}
          <div className="flex items-center justify-between">
            <span className={`text-xs ${getStatusColor()}`}>
              {getStatusText()}
            </span>
            {(worktree.commit_count > 0 || worktree.commits_behind > 0) && (
              <div className="flex items-center gap-1">
                {worktree.commit_count > 0 && (
                  <Badge variant="outline" className="text-xs">
                    +{worktree.commit_count} ahead
                  </Badge>
                )}
                {worktree.commits_behind > 0 && (
                  <Badge variant="outline" className="text-xs">
                    -{worktree.commits_behind} behind
                  </Badge>
                )}
              </div>
            )}
          </div>
        </div>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

function TodosList({ worktree }: { worktree: Worktree }) {
  const todos = worktree.todos || [];

  const completedTodos = todos.filter((todo) => todo.status === "completed");
  const pendingTodos = todos.filter((todo) => todo.status !== "completed");

  if (todos.length === 0) {
    return (
      <SidebarGroup>
        <SidebarGroupLabel>Todos</SidebarGroupLabel>
        <SidebarGroupContent>
          <div className="text-sm text-muted-foreground p-2">
            No todos found
          </div>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  return (
    <SidebarGroup>
      <SidebarGroupLabel>
        <div className="flex items-center justify-between w-full">
          <span>Todos</span>
          <Badge variant="secondary" className="text-xs">
            {completedTodos.length}/{todos.length}
          </Badge>
        </div>
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <ScrollArea className="h-48">
          <div className="space-y-0.5">
            {/* Render todos in original order while preserving styling logic */}
            {todos.map((todo, _index) => {
              const isCompleted = todo.status === "completed";

              if (isCompleted) {
                // Completed todo styling
                return (
                  <div
                    key={todo.id}
                    className="flex items-start gap-2 px-2 py-1 rounded-md"
                  >
                    <CheckCircle className="h-3 w-3 text-green-500 mt-0.5 flex-shrink-0" />
                    <div className="flex-1 min-w-0">
                      <p className="text-xs break-words text-muted-foreground line-through">
                        {todo.content}
                      </p>
                    </div>
                  </div>
                );
              } else {
                // Pending todo styling - find if this is the most recent uncompleted
                const pendingIndex = pendingTodos.findIndex(
                  (t) => t.id === todo.id,
                );
                const isMostRecent = pendingIndex === 0;

                return (
                  <div
                    key={todo.id}
                    className="flex items-start gap-2 px-2 py-1 rounded-md hover:bg-muted/50"
                  >
                    <Circle
                      className={`h-3 w-3 mt-0.5 flex-shrink-0 ${isMostRecent ? "text-foreground" : "text-muted-foreground"}`}
                    />
                    <div className="flex-1 min-w-0">
                      <p
                        className={`text-xs break-words ${isMostRecent ? "text-foreground" : "text-muted-foreground"}`}
                      >
                        {todo.content}
                      </p>
                    </div>
                  </div>
                );
              }
            })}
          </div>
        </ScrollArea>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

function ChangedFiles({
  worktree,
  repository,
  showDiffView,
  setShowDiffView,
  setShowPortPreview,
  setSelectedFile,
  onSync,
}: {
  worktree: Worktree;
  repository: LocalRepository;
  showDiffView: boolean;
  setShowDiffView: (showDiff: boolean) => void;
  setShowPortPreview: (port: number | null) => void;
  setSelectedFile?: (file: string | undefined) => void;
  onSync?: (id: string) => void;
}) {
  const { deleteWorktree } = useGitApi();
  const navigate = useNavigate();
  const [deleteDialog, setDeleteDialog] = useState<{
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

  // Handle delete with confirmation
  const handleConfirmDelete = (id: string, name: string) => {
    setDeleteDialog({
      open: true,
      worktreeId: id,
      worktreeName: name,
      hasChanges: worktree.is_dirty,
      commitCount: worktree.commit_count,
    });
  };

  const handleDeleteConfirmed = async () => {
    try {
      await deleteWorktree(deleteDialog.worktreeId);
      // Close dialog after successful deletion
      setDeleteDialog({
        open: false,
        worktreeId: "",
        worktreeName: "",
        hasChanges: false,
        commitCount: 0,
      });
      // Redirect to workspace index to find next available workspace or show onboarding
      void navigate({ to: "/workspace" });
    } catch (error) {
      console.error("Failed to delete worktree:", error);
      // Keep dialog open on error so user can retry
    }
  };
  const { diffStats, loading, error } = useWorktreeDiff(
    worktree.id,
    worktree.commit_hash,
    worktree.is_dirty,
  );

  const fileDiffs = diffStats?.file_diffs || [];

  if (loading) {
    return (
      <SidebarGroup>
        <SidebarGroupLabel>Changed Files</SidebarGroupLabel>
        <SidebarGroupContent>
          <div className="text-sm text-muted-foreground p-2 flex items-center gap-2">
            <RotateCw className="h-3 w-3 animate-spin" />
            Loading changes...
          </div>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  if (error) {
    return (
      <SidebarGroup>
        <SidebarGroupLabel>Changed Files</SidebarGroupLabel>
        <SidebarGroupContent>
          <div className="text-sm text-red-500 p-2">Failed to load changes</div>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  if (fileDiffs.length === 0) {
    const changesList = [];
    if (deleteDialog.hasChanges) changesList.push("uncommitted changes");
    if (deleteDialog.commitCount > 0)
      changesList.push(`${deleteDialog.commitCount} commits`);

    return (
      <>
        <SidebarGroup>
          <SidebarGroupLabel>
            <div className="flex items-center justify-between w-full">
              <span>Changed Files</span>
              <div className="flex items-center gap-2">
                <Badge variant="secondary" className="text-xs">
                  0
                </Badge>
                {worktree.pull_request_url && (
                  <a
                    href={worktree.pull_request_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={`flex items-center gap-1 text-xs text-blue-500 hover:text-blue-600 transition-colors ${
                      worktree.pull_request_state === "CLOSED" ||
                      worktree.pull_request_state === "MERGED"
                        ? "line-through opacity-60"
                        : ""
                    }`}
                    title="Open pull request"
                  >
                    <span>
                      PR{" "}
                      {worktree.pull_request_url.match(/\/pull\/(\d+)/)?.[1] ||
                        "?"}
                    </span>
                    <ExternalLink className="h-3 w-3" />
                  </a>
                )}
                <WorkspaceActions
                  mode="workspace"
                  worktree={worktree}
                  repository={repository}
                  onSync={onSync}
                  onDelete={handleConfirmDelete}
                />
              </div>
            </div>
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <div className="text-sm text-muted-foreground p-2">No changes</div>
          </SidebarGroupContent>
        </SidebarGroup>

        <AlertDialog
          open={deleteDialog.open}
          onOpenChange={(open) => setDeleteDialog({ ...deleteDialog, open })}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete Workspace</AlertDialogTitle>
              <AlertDialogDescription>
                {changesList.length > 0 ? (
                  <>
                    Delete workspace "{deleteDialog.worktreeName}"? This
                    workspace has <strong>{changesList.join(" and ")}</strong>.
                    This action cannot be undone.
                  </>
                ) : (
                  <>
                    Delete workspace "{deleteDialog.worktreeName}"? This action
                    cannot be undone.
                  </>
                )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                onClick={handleDeleteConfirmed}
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

  const getFileStatusIcon = (changeType: string) => {
    switch (changeType) {
      case "modified":
        return <AlertCircle className="h-3 w-3 text-yellow-500" />;
      case "added":
        return <Plus className="h-3 w-3 text-green-500" />;
      case "deleted":
        return <Minus className="h-3 w-3 text-red-500" />;
      case "renamed":
        return <RotateCw className="h-3 w-3 text-blue-500" />;
      default:
        return <Circle className="h-3 w-3 text-muted-foreground" />;
    }
  };

  const getFileStatusLabel = (changeType: string) => {
    return changeType.charAt(0).toUpperCase() + changeType.slice(1);
  };

  const getFileStatusBadge = (changeType: string) => {
    switch (changeType) {
      case "modified":
        return "M";
      case "added":
        return "A";
      case "deleted":
        return "D";
      case "renamed":
        return "R";
      default:
        return "?";
    }
  };

  const changesList = [];
  if (deleteDialog.hasChanges) changesList.push("uncommitted changes");
  if (deleteDialog.commitCount > 0)
    changesList.push(`${deleteDialog.commitCount} commits`);

  return (
    <>
      <SidebarGroup>
        <SidebarGroupLabel>
          <div className="flex items-center justify-between w-full">
            <span>Changed Files</span>
            <div className="flex items-center gap-2">
              <Badge variant="secondary" className="text-xs">
                {fileDiffs.length}
              </Badge>
              {worktree.pull_request_url && (
                <a
                  href={worktree.pull_request_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={`flex items-center gap-1 text-xs text-blue-500 hover:text-blue-600 transition-colors ${
                    worktree.pull_request_state === "CLOSED" ||
                    worktree.pull_request_state === "MERGED"
                      ? "line-through opacity-60"
                      : ""
                  }`}
                  title="Open pull request"
                >
                  <span>
                    PR{" "}
                    {worktree.pull_request_url.match(/\/pull\/(\d+)/)?.[1] ||
                      "?"}
                  </span>
                  <ExternalLink className="h-3 w-3" />
                </a>
              )}
              <WorkspaceActions
                mode="workspace"
                worktree={worktree}
                repository={repository}
                onSync={onSync}
                onDelete={handleConfirmDelete}
              />
            </div>
          </div>
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <ScrollArea className="h-48">
            <div className="space-y-0.5">
              {fileDiffs.map((file, index) => {
                const fileName =
                  file.file_path.split("/").pop() || file.file_path;

                return (
                  <Tooltip key={index}>
                    <TooltipTrigger asChild>
                      <div
                        className="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-muted/50 cursor-pointer"
                        onClick={() => {
                          setShowDiffView(true);
                          setShowPortPreview(null);
                          setSelectedFile?.(file.file_path);
                        }}
                      >
                        {getFileStatusIcon(file.change_type)}
                        <FileText className="h-3 w-3 text-muted-foreground flex-shrink-0" />
                        <span className="text-sm truncate flex-1">
                          {fileName}
                        </span>
                        <Badge variant="outline" className="text-xs">
                          {getFileStatusBadge(file.change_type)}
                        </Badge>
                      </div>
                    </TooltipTrigger>
                    <TooltipContent side="left" align="center">
                      <div className="space-y-1">
                        <div className="font-medium">
                          {getFileStatusLabel(file.change_type)}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {file.file_path}
                        </div>
                      </div>
                    </TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          </ScrollArea>
        </SidebarGroupContent>
        {fileDiffs.length > 0 && (
          <div className="px-2 py-2">
            <Button
              variant={showDiffView ? "default" : "outline"}
              size="sm"
              onClick={() => {
                setShowDiffView(!showDiffView);
                // Close port preview when showing diff view
                if (!showDiffView) {
                  setShowPortPreview(null);
                }
              }}
              className="w-full h-8 text-xs"
              title={showDiffView ? "Show Claude Terminal" : "View Diff"}
            >
              {showDiffView ? (
                <>
                  <Terminal className="h-3 w-3 mr-2" />
                  Show Claude
                </>
              ) : (
                <>
                  <Eye className="h-3 w-3 mr-2" />
                  View Diff
                </>
              )}
            </Button>
          </div>
        )}
      </SidebarGroup>

      <AlertDialog
        open={deleteDialog.open}
        onOpenChange={(open) => setDeleteDialog({ ...deleteDialog, open })}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Workspace</AlertDialogTitle>
            <AlertDialogDescription>
              {changesList.length > 0 ? (
                <>
                  Delete workspace "{deleteDialog.worktreeName}"? This workspace
                  has <strong>{changesList.join(" and ")}</strong>. This action
                  cannot be undone.
                </>
              ) : (
                <>
                  Delete workspace "{deleteDialog.worktreeName}"? This action
                  cannot be undone.
                </>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeleteConfirmed}
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

function WorkspacePorts({
  worktree,
  showPortPreview,
  setShowPortPreview,
  setShowDiffView,
}: {
  worktree: Worktree;
  showPortPreview: number | null;
  setShowPortPreview: (port: number | null) => void;
  setShowDiffView: (showDiff: boolean) => void;
}) {
  // Subscribe to ports Map (reference changes on update), then memoize to array
  const portsMap = useAppStore((state) => state.ports);
  const allPorts = useMemo(() => Array.from(portsMap.values()), [portsMap]);

  // Filter ports for this workspace
  const workspacePorts = useMemo(() => {
    return allPorts.filter((port) =>
      port.workingDir?.startsWith(worktree.path),
    );
  }, [allPorts, worktree.path]);

  const openInNewWindow = (p: { port: number; hostPort?: number }) => {
    if (p.hostPort) {
      window.open(`http://localhost:${p.hostPort}/`, "_blank");
    } else {
      window.open(`/${p.port}/`, "_blank");
    }
  };

  const previewPort = (port: number) => {
    setShowPortPreview(port);
    // Close diff view when showing port preview
    setShowDiffView(false);
  };

  if (workspacePorts.length === 0) {
    return (
      <SidebarGroup>
        <SidebarGroupLabel>Ports</SidebarGroupLabel>
        <SidebarGroupContent>
          <div className="text-sm text-muted-foreground p-2">
            No active ports in this workspace
          </div>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  return (
    <SidebarGroup>
      <SidebarGroupLabel>
        <div className="flex items-center justify-between w-full">
          <span>Ports</span>
          <Badge variant="secondary" className="text-xs">
            {workspacePorts.length}
          </Badge>
        </div>
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <ScrollArea className="h-32">
          <div className="space-y-1">
            {workspacePorts.map((port) => (
              <div
                key={port.port}
                className={`flex items-center gap-2 p-2 rounded-md hover:bg-muted/50 cursor-pointer group w-full min-w-0 ${
                  showPortPreview === port.port ? "bg-muted" : ""
                }`}
                onClick={() => previewPort(port.port)}
                title={`Preview port ${port.port} - ${port.title || port.service || "Unknown service"}`}
              >
                <Globe className="h-3 w-3 text-blue-500 flex-shrink-0" />
                <div className="flex-1 min-w-0 overflow-hidden">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-sm font-medium flex-shrink-0">
                      :{port.port}
                    </span>
                    {port.service && (
                      <Badge
                        variant="outline"
                        className="text-xs max-w-[96px] truncate overflow-hidden"
                        title={port.service}
                      >
                        {port.service}
                      </Badge>
                    )}
                  </div>
                  {(port.title || port.hostPort) && (
                    <p className="text-xs text-muted-foreground truncate whitespace-nowrap max-w-[170px]">
                      {port.title}
                      {port.hostPort
                        ? ` • forwarded to localhost:${port.hostPort}`
                        : ""}
                    </p>
                  )}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 flex-shrink-0 ml-auto"
                  onClick={(e) => {
                    e.stopPropagation();
                    openInNewWindow(port);
                  }}
                  title="Open in new window"
                >
                  <ExternalLink className="h-3 w-3" />
                </Button>
              </div>
            ))}
          </div>
        </ScrollArea>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

export function WorkspaceRightSidebar({
  worktree,
  repository,
  showDiffView,
  setShowDiffView,
  showPortPreview,
  setShowPortPreview,
  setSelectedFile,
  onSync,
}: WorkspaceRightSidebarProps) {
  return (
    <Sidebar
      collapsible="none"
      className="sticky top-0 hidden h-svh border-l lg:flex w-64 flex-shrink-0"
      side="right"
    >
      <SidebarContent>
        <GitStatus worktree={worktree} />
        <SidebarSeparator className="mx-0" />
        <TodosList worktree={worktree} />
        <SidebarSeparator className="mx-0" />
        <ChangedFiles
          worktree={worktree}
          repository={repository}
          showDiffView={showDiffView}
          setShowDiffView={setShowDiffView}
          setShowPortPreview={setShowPortPreview}
          setSelectedFile={setSelectedFile}
          onSync={onSync}
        />
        <SidebarSeparator className="mx-0" />
        <WorkspacePorts
          worktree={worktree}
          showPortPreview={showPortPreview}
          setShowPortPreview={setShowPortPreview}
          setShowDiffView={setShowDiffView}
        />
      </SidebarContent>
    </Sidebar>
  );
}
