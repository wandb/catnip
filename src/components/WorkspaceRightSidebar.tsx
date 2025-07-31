import {
  FileText,
  GitBranch,
  CheckCircle,
  Circle,
  AlertCircle,
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
import { ScrollArea } from "@/components/ui/scroll-area";
import type { Worktree, LocalRepository } from "@/lib/git-api";

interface WorkspaceRightSidebarProps {
  worktree: Worktree;
  repository: LocalRepository;
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

          {/* Status badge */}
          <div className="flex items-center gap-2">
            <div
              className={`w-2 h-2 rounded-full ${getStatusColor().replace("text-", "bg-")}`}
            />
            <span className={`text-sm ${getStatusColor()}`}>
              {getStatusText()}
            </span>
          </div>

          {/* Commit counts */}
          {(worktree.commit_count > 0 || worktree.commits_behind > 0) && (
            <div className="space-y-1">
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
                    className={`h-4 w-4 mt-0.5 flex-shrink-0 ${isMostRecent ? "text-foreground" : "text-muted-foreground"}`}
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
                <CheckCircle className="h-4 w-4 text-green-500 mt-0.5 flex-shrink-0" />
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

function ChangedFiles({ worktree }: { worktree: Worktree }) {
  const dirtyFiles = worktree.dirty_files || [];

  if (dirtyFiles.length === 0) {
    return (
      <SidebarGroup>
        <SidebarGroupLabel>Changed Files</SidebarGroupLabel>
        <SidebarGroupContent>
          <div className="text-sm text-muted-foreground p-2">No changes</div>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  const getFileStatusIcon = (status: string) => {
    switch (status) {
      case "M":
        return <AlertCircle className="h-3 w-3 text-yellow-500" />;
      case "A":
        return <Circle className="h-3 w-3 text-green-500" />;
      case "D":
        return <Circle className="h-3 w-3 text-red-500" />;
      case "R":
        return <Circle className="h-3 w-3 text-blue-500" />;
      default:
        return <Circle className="h-3 w-3 text-muted-foreground" />;
    }
  };

  const getFileStatusLabel = (status: string) => {
    switch (status) {
      case "M":
        return "Modified";
      case "A":
        return "Added";
      case "D":
        return "Deleted";
      case "R":
        return "Renamed";
      default:
        return "Changed";
    }
  };

  return (
    <SidebarGroup>
      <SidebarGroupLabel>
        <div className="flex items-center justify-between w-full">
          <span>Changed Files</span>
          <Badge variant="secondary" className="text-xs">
            {dirtyFiles.length}
          </Badge>
        </div>
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <ScrollArea className="h-48">
          <div className="space-y-1">
            {dirtyFiles.map((file, index) => (
              <div
                key={index}
                className="flex items-center gap-2 p-2 rounded-md hover:bg-muted/50 cursor-pointer"
                title={`${getFileStatusLabel(file.status)}: ${file.path}`}
              >
                {getFileStatusIcon(file.status)}
                <FileText className="h-3 w-3 text-muted-foreground flex-shrink-0" />
                <span className="text-sm truncate flex-1">{file.path}</span>
                <Badge variant="outline" className="text-xs">
                  {file.status}
                </Badge>
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
        <ChangedFiles worktree={worktree} />
      </SidebarContent>
    </Sidebar>
  );
}
