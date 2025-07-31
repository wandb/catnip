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
import { useWorktreeDiff } from "@/hooks/useWorktreeDiff";
import { WorkspaceActions } from "@/components/WorkspaceActions";
import type { Worktree, LocalRepository } from "@/lib/git-api";

interface WorkspaceRightSidebarProps {
  worktree: Worktree;
  repository: LocalRepository;
  showDiffView: boolean;
  setShowDiffView: (showDiff: boolean) => void;
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
          <div className="space-y-2">
            {/* Pending todos */}
            {pendingTodos.map((todo, index) => {
              // Most recent uncompleted item gets brighter styling
              const isMostRecent = index === 0;
              return (
                <div
                  key={todo.id}
                  className="flex items-start gap-2 p-2 rounded-md hover:bg-muted/50"
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
            })}

            {/* Completed todos */}
            {completedTodos.map((todo) => (
              <div
                key={todo.id}
                className="flex items-start gap-2 p-2 rounded-md"
              >
                <CheckCircle className="h-3 w-3 text-green-500 mt-0.5 flex-shrink-0" />
                <div className="flex-1 min-w-0">
                  <p className="text-xs break-words text-muted-foreground line-through">
                    {todo.content}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </ScrollArea>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

function ChangedFiles({
  worktree,
  showDiffView,
  setShowDiffView,
}: {
  worktree: Worktree;
  showDiffView: boolean;
  setShowDiffView: (showDiff: boolean) => void;
}) {
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
    return (
      <SidebarGroup>
        <SidebarGroupLabel>Changed Files</SidebarGroupLabel>
        <SidebarGroupContent>
          <div className="text-sm text-muted-foreground p-2">No changes</div>
        </SidebarGroupContent>
      </SidebarGroup>
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

  return (
    <SidebarGroup>
      <SidebarGroupLabel>
        <div className="flex items-center justify-between w-full">
          <span>Changed Files</span>
          <div className="flex items-center gap-2">
            <Badge variant="secondary" className="text-xs">
              {fileDiffs.length}
            </Badge>
            <WorkspaceActions mode="workspace" worktree={worktree} />
          </div>
        </div>
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <ScrollArea className="h-48">
          <div className="space-y-1">
            {fileDiffs.map((file, index) => (
              <div
                key={index}
                className="flex items-center gap-2 p-2 rounded-md hover:bg-muted/50 cursor-pointer"
                title={`${getFileStatusLabel(file.change_type)}: ${file.file_path}`}
              >
                {getFileStatusIcon(file.change_type)}
                <FileText className="h-3 w-3 text-muted-foreground flex-shrink-0" />
                <span className="text-sm truncate flex-1">
                  {file.file_path}
                </span>
                <Badge variant="outline" className="text-xs">
                  {getFileStatusBadge(file.change_type)}
                </Badge>
              </div>
            ))}
          </div>
        </ScrollArea>
      </SidebarGroupContent>
      {fileDiffs.length > 0 && (
        <div className="px-2 py-2">
          <Button
            variant={showDiffView ? "default" : "outline"}
            size="sm"
            onClick={() => setShowDiffView(!showDiffView)}
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
  );
}

export function WorkspaceRightSidebar({
  worktree,
  showDiffView,
  setShowDiffView,
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
          showDiffView={showDiffView}
          setShowDiffView={setShowDiffView}
        />
        <SidebarSeparator className="mx-0" />
        <SidebarGroup>
          <SidebarGroupLabel>Ports</SidebarGroupLabel>
          <SidebarGroupContent>
            <div className="text-sm text-muted-foreground p-2">
              No active ports
            </div>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  );
}
