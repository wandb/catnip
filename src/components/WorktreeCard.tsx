import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DiffViewer } from "@/components/DiffViewer";
import {
  GitMerge,
  Eye,
  AlertTriangle,
  FileText,
  MoreHorizontal,
  Terminal,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { getRelativeTime, getDuration } from "@/lib/timeUtils";
import { Worktree } from "@/types/git";

interface WorktreeCardProps {
  worktree: Worktree;
  activeSessions: Record<string, any>;
  claudeSessions: Record<string, any>;
  syncConflicts: Record<string, any>;
  mergeConflicts: Record<string, any>;
  openDiffWorktreeId: string | null;
  onToggleDiff: (worktreeId: string) => void;
  onSyncWorktree: (id: string) => void;
  onCreatePreview: (id: string, branchName: string) => void;
  onOpenPrDialog: (worktreeId: string, branchName: string) => void;
  onMergeWorktree: (id: string, name: string) => void;
  onDeleteWorktree: (id: string, hasChanges: boolean, changesList: string[]) => void;
}

export const WorktreeCard = ({
  worktree,
  activeSessions,
  claudeSessions,
  syncConflicts,
  mergeConflicts,
  openDiffWorktreeId,
  onToggleDiff,
  onSyncWorktree,
  onCreatePreview,
  onOpenPrDialog,
  onMergeWorktree,
  onDeleteWorktree,
}: WorktreeCardProps) => {
  const hasConflicts = syncConflicts[worktree.id]?.has_conflicts || mergeConflicts[worktree.id]?.has_conflicts;
  const hasChanges = worktree.is_dirty || worktree.commit_count > 0;

  const renderClaudeSession = () => {
    const session = claudeSessions[worktree.path];
    if (!session) {
      return <p className="text-xs text-muted-foreground">No Claude sessions</p>;
    }

    if (session.sessionStartTime && !session.isActive) {
      return (
        <p>
          Finished: {getRelativeTime(session.sessionEndTime || session.sessionStartTime)} • 
          Lasted: {getDuration(session.sessionStartTime, session.sessionEndTime || session.sessionStartTime)}
        </p>
      );
    }

    if (session.sessionStartTime && session.isActive) {
      return (
        <p>
          Running: {getDuration(session.sessionStartTime, new Date())}
        </p>
      );
    }

    return session.isActive ? (
      <p>Running: recently started</p>
    ) : (
      <p>Session completed (timing data unavailable)</p>
    );
  };

  const handleDelete = () => {
    if (hasChanges) {
      const changesList = [];
      if (worktree.is_dirty) changesList.push("uncommitted changes");
      if (worktree.commit_count > 0) changesList.push(`${worktree.commit_count} commits`);
      onDeleteWorktree(worktree.id, true, changesList);
    } else {
      onDeleteWorktree(worktree.id, false, []);
    }
  };

  return (
    <div className="space-y-0">
      <div className="flex items-center justify-between p-3 border rounded-lg">
        <div className="flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-medium">{worktree.name}</span>
            {activeSessions[worktree.path] && (
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
            
            {claudeSessions[worktree.path] && (
              <>
                <Badge variant="secondary" className="text-xs">
                  {claudeSessions[worktree.path].turnCount} turns
                </Badge>
                {claudeSessions[worktree.path].lastCost > 0 && (
                  <Badge variant="secondary" className="text-xs">
                    ${claudeSessions[worktree.path].lastCost.toFixed(4)}
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
            {renderClaudeSession()}
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
          
          {hasChanges && (
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
                  onClick={() => onSyncWorktree(worktree.id)}
                  className={syncConflicts[worktree.id]?.has_conflicts ? "text-red-600" : "text-orange-600"}
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
              
              {worktree.commit_count > 0 && (
                <DropdownMenuItem
                  onClick={() => onOpenPrDialog(worktree.id, worktree.branch)}
                  className="text-green-600"
                >
                  <GitMerge size={16} />
                  Create PR (GitHub)
                </DropdownMenuItem>
              )}
              
              {worktree.repo_id.startsWith("local/") && worktree.commit_count > 0 && (
                <DropdownMenuItem
                  onClick={() => onMergeWorktree(worktree.id, worktree.name)}
                  className={mergeConflicts[worktree.id]?.has_conflicts ? "text-red-600" : "text-blue-600"}
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
              
              <DropdownMenuItem onClick={handleDelete} variant="destructive">
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
};