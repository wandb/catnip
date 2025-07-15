import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { DiffViewer } from "@/components/DiffViewer";
import { 
  DropdownMenu, 
  DropdownMenuContent, 
  DropdownMenuItem, 
  DropdownMenuSeparator, 
  DropdownMenuTrigger 
} from "@/components/ui/dropdown-menu";
import { 
  AlertTriangle, 
  Eye, 
  FileText, 
  GitMerge, 
  MoreHorizontal, 
  RefreshCw, 
  Terminal, 
  Trash2 
} from "lucide-react";
import { type Worktree } from "@/lib/git-api";
import { getRelativeTime, getDuration } from "@/lib/git-utils";

interface WorktreeRowProps {
  worktree: Worktree;
  activeSessions: Record<string, any>;
  claudeSessions: Record<string, any>;
  syncConflicts: Record<string, any>;
  mergeConflicts: Record<string, any>;
  openDiffWorktreeId: string | null;
  onToggleDiff: (worktreeId: string) => void;
  onSync: (id: string) => void;
  onMerge: (id: string, name: string) => void;
  onCreatePreview: (id: string, branch: string) => void;
  onDelete: (id: string) => void;
  onConfirmDelete: (id: string, name: string, isDirty: boolean, commitCount: number) => void;
}

export function WorktreeRow({
  worktree,
  activeSessions,
  claudeSessions,
  syncConflicts,
  mergeConflicts,
  openDiffWorktreeId,
  onToggleDiff,
  onSync,
  onMerge,
  onCreatePreview,
  onDelete,
  onConfirmDelete,
}: WorktreeRowProps) {
  const sessionPath = worktree.path;
  const claudeSession = claudeSessions[sessionPath];
  const hasConflicts = syncConflicts[worktree.id]?.has_conflicts || mergeConflicts[worktree.id]?.has_conflicts;

  const renderClaudeSessionStatus = () => {
    if (!claudeSession) {
      return <p className="text-xs text-muted-foreground">No Claude sessions</p>;
    }

    if (claudeSession.sessionStartTime && !claudeSession.isActive) {
      // Finished session
      return (
        <p>
          Finished: {getRelativeTime(claudeSession.sessionEndTime || claudeSession.sessionStartTime)} • 
          Lasted: {getDuration(claudeSession.sessionStartTime, claudeSession.sessionEndTime || claudeSession.sessionStartTime)}
        </p>
      );
    } else if (claudeSession.sessionStartTime && claudeSession.isActive) {
      // Active session with timing
      return (
        <p>
          Running: {getDuration(claudeSession.sessionStartTime, new Date())}
        </p>
      );
    } else if (claudeSession.isActive) {
      // Active session without timestamp
      return <p>Running: recently started</p>;
    } else {
      // Completed session without timestamp
      return <p>Session completed (timing data unavailable)</p>;
    }
  };

  const handleDeleteClick = () => {
    if (worktree.is_dirty || worktree.commit_count > 0) {
      onConfirmDelete(worktree.id, worktree.name, worktree.is_dirty, worktree.commit_count);
    } else {
      onDelete(worktree.id);
    }
  };

  return (
    <div className="space-y-0">
      <div className="flex items-center justify-between p-3 border rounded-lg">
        <div className="flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-medium">{worktree.name}</span>
            {activeSessions[sessionPath] && (
              <div
                className="w-2 h-2 bg-green-500 rounded-full animate-pulse"
                title="Active session running"
              />
            )}
            <Badge variant="outline">
              {worktree.repo_id}@{worktree.source_branch || "unknown"}
            </Badge>
            {worktree.is_dirty ? (
              <Badge variant="destructive">Dirty</Badge>
            ) : (
              <Badge variant="secondary" className="text-xs bg-green-100 text-green-800 border-green-200">
                Clean
              </Badge>
            )}
            {worktree.commit_count > 0 && (
              <Badge variant="secondary">+{worktree.commit_count} commits</Badge>
            )}
            {worktree.commits_behind > 0 && (
              <Badge variant="outline" className="border-orange-200 text-orange-800 bg-orange-50">
                {worktree.commits_behind} behind
                {syncConflicts[worktree.id]?.has_conflicts && " ⚠️"}
              </Badge>
            )}
            {hasConflicts && (
              <Badge variant="outline" className="border-red-200 text-red-800 bg-red-50">
                Conflicts detected
              </Badge>
            )}
            {claudeSession && (
              <>
                <Badge variant="secondary" className="text-xs">
                  {claudeSession.turnCount} turns
                </Badge>
                {claudeSession.lastCost > 0 && (
                  <Badge variant="secondary" className="text-xs">
                    ${claudeSession.lastCost.toFixed(4)}
                  </Badge>
                )}
              </>
            )}
          </div>
          <div className="text-xs text-muted-foreground space-y-1">
            <Link
              to="/terminal/$sessionId"
              params={{ sessionId: worktree.name }}
              search={{ agent: undefined }}
              className="cursor-pointer hover:text-primary underline-offset-4 hover:underline"
            >
              {worktree.path}
            </Link>
            <div className="space-y-1">
              {renderClaudeSessionStatus()}
            </div>
          </div>
        </div>
        <div className="flex gap-2">
          <Link
            to="/terminal/$sessionId"
            params={{ sessionId: worktree.name }}
            search={{ agent: "claude" }}
          >
            <Button variant="outline" size="sm" asChild>
              <span>Vibe</span>
            </Button>
          </Link>
          {(worktree.is_dirty || worktree.commit_count > 0) && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => onToggleDiff(worktree.id)}
              title="View diff against source branch"
              className={
                openDiffWorktreeId === worktree.id
                  ? "text-blue-600 border-blue-200 bg-blue-50"
                  : "text-gray-600 border-gray-200 hover:bg-gray-50"
              }
            >
              <FileText size={16} />
            </Button>
          )}
          
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm" className="relative">
                <MoreHorizontal size={16} />
                {hasConflicts && (
                  <div className="absolute -top-1 -right-1 w-2 h-2 bg-red-500 rounded-full" />
                )}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link
                  to="/terminal/$sessionId"
                  params={{ sessionId: worktree.name }}
                  className="cursor-pointer"
                >
                  <Terminal size={16} />
                  Open Terminal
                </Link>
              </DropdownMenuItem>
              
              <DropdownMenuSeparator />
              
              {worktree.commits_behind > 0 && (
                <DropdownMenuItem
                  onClick={() => onSync(worktree.id)}
                  className={
                    syncConflicts[worktree.id]?.has_conflicts
                      ? "text-red-600"
                      : "text-orange-600"
                  }
                >
                  {syncConflicts[worktree.id]?.has_conflicts ? (
                    <AlertTriangle size={16} />
                  ) : (
                    <RefreshCw size={16} />
                  )}
                  {syncConflicts[worktree.id]?.has_conflicts
                    ? `Sync (${worktree.commits_behind} commits, conflicts)`
                    : `Sync ${worktree.commits_behind} commits`
                  }
                </DropdownMenuItem>
              )}
              
              {worktree.repo_id.startsWith("local/") && (
                <DropdownMenuItem
                  onClick={() => onCreatePreview(worktree.id, worktree.branch)}
                  className="text-purple-600"
                >
                  <Eye size={16} />
                  Create Preview
                </DropdownMenuItem>
              )}
              
              {worktree.repo_id.startsWith("local/") && worktree.commit_count > 0 && (
                <DropdownMenuItem
                  onClick={() => onMerge(worktree.id, worktree.name)}
                  className={
                    mergeConflicts[worktree.id]?.has_conflicts
                      ? "text-red-600"
                      : "text-blue-600"
                  }
                >
                  {mergeConflicts[worktree.id]?.has_conflicts ? (
                    <AlertTriangle size={16} />
                  ) : (
                    <GitMerge size={16} />
                  )}
                  {mergeConflicts[worktree.id]?.has_conflicts
                    ? `Merge ${worktree.commit_count} commits (conflicts)`
                    : `Merge ${worktree.commit_count} commits`
                  }
                </DropdownMenuItem>
              )}
              
              <DropdownMenuSeparator />
              
              <DropdownMenuItem onClick={handleDeleteClick} variant="destructive">
                <Trash2 size={16} />
                Delete Worktree
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      <DiffViewer
        worktreeId={worktree.id}
        isOpen={openDiffWorktreeId === worktree.id}
        onClose={() => onToggleDiff(worktree.id)}
      />
    </div>
  );
}