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
  Loader2,
  MoreHorizontal, 
  RefreshCw, 
  Sparkles,
  Terminal, 
  Trash2 
} from "lucide-react";
import { type Worktree, type WorktreeDiffStats, type PullRequestInfo } from "@/lib/git-api";
import { type WorktreeSummary } from "@/lib/worktree-summary";
import { getRelativeTime, getDuration } from "@/lib/git-utils";

interface ClaudeSession {
  sessionStartTime?: string | Date;
  sessionEndTime?: string | Date;
  isActive: boolean;
  turnCount: number;
  lastCost: number;
  header?: string;
}

interface ConflictStatus {
  has_conflicts: boolean;
}

interface WorktreeRowProps {
  worktree: Worktree;
  claudeSessions: Record<string, ClaudeSession>;
  syncConflicts: Record<string, ConflictStatus>;
  mergeConflicts: Record<string, ConflictStatus>;
  worktreeSummaries: Record<string, WorktreeSummary>;
  diffStats: Record<string, WorktreeDiffStats | undefined>;
  openDiffWorktreeId: string | null;
  setPrDialog: (dialog: {
    open: boolean;
    worktreeId: string;
    branchName: string;
    title: string;
    description: string;
    isUpdate: boolean;
  }) => void;
  onToggleDiff: (worktreeId: string) => void;
  onSync: (id: string) => void;
  onMerge: (id: string, name: string) => void;
  onCreatePreview: (id: string, branch: string) => void;
  onConfirmDelete: (id: string, name: string, isDirty: boolean, commitCount: number) => void;
}

interface WorktreeHeaderProps {
  worktree: Worktree;
  hasConflicts: boolean;
  claudeSession?: ClaudeSession;
}

function WorktreeHeader({ worktree, hasConflicts, claudeSession }: WorktreeHeaderProps) {
  return (
    <div className="flex items-center gap-3">
      <div className="flex items-center gap-2">
        <Link
          to="/terminal/$sessionId"
          params={{ sessionId: encodeURIComponent(worktree.name) }}
          className="text-lg font-medium hover:underline"
        >
          {worktree.name}
        </Link>
        <Badge variant="outline" className="text-xs">
          {worktree.branch}
        </Badge>
      </div>
      {hasConflicts && (
        <Badge variant="destructive" className="text-xs">
          <AlertTriangle size={12} className="mr-1" />
          Conflicts
        </Badge>
      )}
      {worktree.is_dirty ? (
        <Badge variant="destructive" className="text-xs">
          Dirty
        </Badge>
      ) : (
        <Badge variant="secondary" className="text-xs bg-green-100 text-green-800 border-green-200">
          Clean
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
  );
}



interface WorktreeClaudeStatusProps {
  claudeSession?: ClaudeSession;
}

function WorktreeClaudeStatus({ claudeSession }: WorktreeClaudeStatusProps) {
  if (!claudeSession) {
    return <p className="text-xs text-muted-foreground">No Claude sessions</p>;
  }

  const sessionStatusText = (() => {
    if (claudeSession.sessionStartTime && !claudeSession.isActive) {
      // Finished session
      return `Finished: ${getRelativeTime(claudeSession.sessionEndTime ?? claudeSession.sessionStartTime)} • Lasted: ${getDuration(claudeSession.sessionStartTime, claudeSession.sessionEndTime ?? claudeSession.sessionStartTime)}`;
    } else if (claudeSession.sessionStartTime && claudeSession.isActive) {
      // Active session with timing
      return `Running: ${getDuration(claudeSession.sessionStartTime, new Date())}`;
    } else if (claudeSession.isActive) {
      // Active session without timestamp
      return "Running: recently started";
    } else {
      // Completed session without timestamp
      return "Completed session";
    }
  })();

  return (
    <div>
      <p className="text-xs text-muted-foreground">
        {sessionStatusText}
      </p>
      {claudeSession.header && (
        <p className="text-xs font-medium text-foreground mt-2" title={claudeSession.header}>
          {claudeSession.header}
        </p>
      )}
    </div>
  );
}

interface WorktreeSummaryStatusProps {
  worktree: Worktree;
  summary?: WorktreeSummary;
}

function WorktreeSummaryStatus({ worktree, summary }: WorktreeSummaryStatusProps) {
  // Only show summary for local repos with more than 1 commit
  if (!worktree.repo_id.startsWith("local/") || worktree.commit_count <= 1) {
    return null;
  }

  if (!summary) {
    return null;
  }

  switch (summary.status) {
    case 'pending':
      return (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Sparkles className="h-3 w-3" />
          <span>Summary pending...</span>
        </div>
      );
    case 'generating':
      return (
        <div className="flex items-center gap-2 text-xs text-blue-600">
          <Loader2 className="h-3 w-3 animate-spin" />
          <span>Generating summary...</span>
        </div>
      );
    case 'completed':
      return (
        <div className="flex items-center gap-2 text-xs text-green-600">
          <Sparkles className="h-3 w-3" />
          <span>Summary ready for PR</span>
        </div>
      );
    case 'error':
      return (
        <div></div>
      );
    default:
      return null;
  }
}

interface WorktreeActionsProps {
  worktree: Worktree;
  mergeConflicts: Record<string, ConflictStatus>;
  diffStats: Record<string, WorktreeDiffStats | undefined>;
  openDiffWorktreeId: string | null;
  prStatus?: PullRequestInfo;
  onToggleDiff: (worktreeId: string) => void;
  onSync: (id: string) => void;
  onMerge: (id: string, name: string) => void;
  onCreatePreview: (id: string, branch: string) => void;
  onConfirmDelete: (id: string, name: string, isDirty: boolean, commitCount: number) => void;
  onOpenPrDialog: (worktreeId: string, branchName: string) => void;
}

function WorktreeActions({ 
  worktree, 
  mergeConflicts, 
  diffStats,
  openDiffWorktreeId, 
  prStatus,
  onToggleDiff, 
  onSync, 
  onMerge, 
  onCreatePreview, 
  onConfirmDelete,
  onOpenPrDialog 
}: WorktreeActionsProps) {
  const handleDeleteClick = () => {
    onConfirmDelete(worktree.id, worktree.name, worktree.is_dirty, worktree.commit_count);
  };

  const hasDiff = (diffStats[worktree.id]?.file_diffs?.length ?? 0) > 0;

  return (
    <div className="flex items-center gap-2">
      {hasDiff && (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onToggleDiff(worktree.id)}
          className={openDiffWorktreeId === worktree.id ? "bg-muted" : ""}
        >
          <FileText size={16} />
          View Diff
        </Button>
      )}
      
      <Link
        to="/terminal/$sessionId"
        params={{ sessionId: worktree.name }}
        search={{ agent: "claude" }}
      >
        <Button variant="outline" size="sm" asChild>
          <span>Vibe</span>
        </Button>
      </Link>
      
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm">
            <MoreHorizontal size={16} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => onSync(worktree.id)}>
            <RefreshCw size={16} />
            Sync with {worktree.source_branch}
          </DropdownMenuItem>
          
          <DropdownMenuSeparator />
          
          <DropdownMenuItem asChild>
            <Link
              to="/terminal/$sessionId"
              params={{ sessionId: encodeURIComponent(worktree.name) }}
              className="flex items-center gap-2"
            >
              <Terminal size={16} />
              Open Terminal
            </Link>
          </DropdownMenuItem>
          
          {!worktree.repo_id.startsWith("local/") && (
            <DropdownMenuItem
              onClick={() => onCreatePreview(worktree.id, worktree.branch)}
              className="text-blue-600"
            >
              <Eye size={16} />
              Create Preview
            </DropdownMenuItem>
          )}

          {worktree.repo_id.startsWith("local/") && worktree.commit_count > 0 && (
            <DropdownMenuItem
              onClick={() => onOpenPrDialog(worktree.id, worktree.branch)}
              className={prStatus?.has_commits_ahead === false ? "text-muted-foreground" : "text-green-600"}
              disabled={prStatus?.has_commits_ahead === false}
            >
              <GitMerge size={16} />
              {prStatus?.has_commits_ahead === false 
                ? "No new commits" 
                : prStatus?.exists 
                  ? "Update PR (GitHub)" 
                  : "Create PR (GitHub)"}
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
  );
}

interface WorktreeRowPropsWithPR extends WorktreeRowProps {
  prStatuses?: Record<string, PullRequestInfo | undefined>;
}

export function WorktreeRow({
  worktree,
  claudeSessions,
  syncConflicts,
  mergeConflicts,
  worktreeSummaries,
  diffStats,
  openDiffWorktreeId,
  setPrDialog,
  onToggleDiff,
  onSync,
  onMerge,
  onCreatePreview,
  onConfirmDelete,
  prStatuses,
}: WorktreeRowPropsWithPR) {
  const sessionPath = worktree.path;
  const claudeSession = claudeSessions[sessionPath];
  const hasConflicts = Boolean(syncConflicts[worktree.id]?.has_conflicts ?? mergeConflicts[worktree.id]?.has_conflicts);
  const summary = worktreeSummaries[worktree.id];
  // const diffStat = diffStats[worktree.id];
  const prStatus = prStatuses?.[worktree.id];

  const openPrDialog = (worktreeId: string, branchName: string) => {
    // Check if PR already exists
    const isUpdate = prStatus?.exists ?? false;
    
    // Use pre-generated summary if available, or existing PR data if updating
    const defaultTitle = isUpdate && prStatus?.title 
      ? prStatus.title 
      : summary?.status === 'completed' 
        ? summary.title 
        : `Pull request from ${branchName}`;
    
    const defaultDescription = isUpdate && prStatus?.body
      ? prStatus.body
      : summary?.status === 'completed' 
        ? summary.summary 
        : `Automated pull request created from worktree ${branchName}`;
    
    setPrDialog({
      open: true,
      worktreeId,
      branchName,
      title: defaultTitle,
      description: defaultDescription,
      isUpdate,
    });
  };

  // const totalAdditions = diffStat?.file_diffs?.filter(diff => diff.change_type === 'added').length ?? 0;
  // const totalDeletions = diffStat?.file_diffs?.filter(diff => diff.change_type === 'deleted').length ?? 0;

  return (
    <div className="border rounded-lg p-4 mb-4 bg-card">
      <div className="flex items-center justify-between">
        <div className="flex-1">
          <WorktreeHeader worktree={worktree} hasConflicts={hasConflicts} claudeSession={claudeSession} />
          
          <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
            <span className="text-xs">
              {worktree.commit_hash.slice(0, 8)}
            </span>
            <span className="text-xs">
              source branch: <span className="font-bold">{worktree.source_branch}</span>
            </span>
            {worktree.commit_count > 0 && (
              <span>
                {worktree.commit_count} commit{worktree.commit_count !== 1 ? 's' : ''}
              </span>
            )}
            {/* TODO: Add total changes */}
            {/* {diffStat && (diffStat.file_diffs?.length ?? 0) > 0 && (
              <span className="flex items-center gap-1 font-mono text-xs">
                <span className="text-green-600">+{totalAdditions}</span>
                /<span className="text-red-600">-{totalDeletions}</span>
              </span>
            )} */}
            {worktree.commits_behind > 0 && (
              <span className="text-orange-600">
                {worktree.commits_behind} behind
              </span>
            )}
          </div>

          <div className="flex items-center gap-4 mt-1">
            <div className="text-xs text-muted-foreground">
              <WorktreeClaudeStatus claudeSession={claudeSession} />
            </div>
            <WorktreeSummaryStatus 
              worktree={worktree} 
              summary={summary} 
            />
          </div>
        </div>
        
        <WorktreeActions
          worktree={worktree}
          mergeConflicts={mergeConflicts}
          diffStats={diffStats}
          openDiffWorktreeId={openDiffWorktreeId}
          prStatus={prStatus}
          onToggleDiff={onToggleDiff}
          onSync={onSync}
          onMerge={onMerge}
          onCreatePreview={onCreatePreview}
          onConfirmDelete={onConfirmDelete}
          onOpenPrDialog={openPrDialog}
        />
      </div>
      <DiffViewer
        worktreeId={worktree.id}
        isOpen={openDiffWorktreeId === worktree.id}
        onClose={() => onToggleDiff(worktree.id)}
      />
    </div>
  );
}