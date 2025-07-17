import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { DiffViewer } from "@/components/DiffViewer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertTriangle,
  ChevronDown,
  Eye,
  FileText,
  GitBranch,
  GitMerge,
  MoreHorizontal,
  RefreshCw,
  Terminal,
  Trash2,
  Copy,
  Check,
} from "lucide-react";
import {
  type Worktree,
  type WorktreeDiffStats,
  type PullRequestInfo,
  type LocalRepository,
} from "@/lib/git-api";
import { type WorktreeSummary } from "@/lib/worktree-summary";
import { getRelativeTime, getDuration } from "@/lib/git-utils";
import type { ConflictStatus } from "@/hooks/useGitState";

interface ClaudeSession {
  sessionStartTime?: string | Date;
  sessionEndTime?: string | Date;
  isActive: boolean;
  turnCount: number;
  lastCost: number;
  header?: string;
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
  onConfirmDelete: (
    id: string,
    name: string,
    isDirty: boolean,
    commitCount: number,
  ) => void;
  onBranchFromWorktree: (worktreeId: string, name: string) => void;
}

interface CommitHashDisplayProps {
  commitHash: string;
  prStatus?: PullRequestInfo;
}

function CommitHashDisplay({ commitHash, prStatus }: CommitHashDisplayProps) {
  const [copiedHash, setCopiedHash] = useState<string | null>(null);

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedHash(text);
      setTimeout(() => setCopiedHash(null), 2000);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  };

  if (prStatus?.exists && prStatus.url) {
    const prCommitUrl = `${prStatus.url}/commits/${commitHash}`;
    return (
      <a
        href={prCommitUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="font-mono text-xs text-muted-foreground hover:text-foreground hover:underline transition-colors inline-flex items-center gap-1 group"
      >
        {commitHash.slice(0, 7)}
        <svg
          className="w-3 h-3 opacity-0 group-hover:opacity-100 transition-opacity"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
          />
        </svg>
      </a>
    );
  }

  const isCopied = copiedHash === commitHash;
  return (
    <button
      onClick={() => copyToClipboard(commitHash)}
      className="font-mono text-xs text-muted-foreground hover:text-foreground hover:bg-muted/50 rounded px-1 py-0.5 transition-colors inline-flex items-center gap-1 group cursor-pointer"
      title={commitHash}
    >
      {commitHash.slice(0, 7)}
      {isCopied ? (
        <Check className="w-3 h-3 text-green-500 opacity-100 transition-opacity" />
      ) : (
        <Copy className="w-3 h-3 opacity-0 group-hover:opacity-100 transition-opacity" />
      )}
    </button>
  );
}

interface StatusBadgesProps {
  worktree: Worktree;
  hasConflicts: boolean;
  claudeSession?: ClaudeSession;
  repositoryUrl?: string;
}

function StatusBadges({
  worktree,
  hasConflicts,
  claudeSession,
  repositoryUrl,
}: StatusBadgesProps) {
  let repoUrl = repositoryUrl;
  if (repoUrl && repoUrl.startsWith("file:///live/")) {
    repoUrl = repoUrl.slice(13);
  }

  return (
    <div className="flex items-center gap-2">
      <Badge variant="outline" className="text-xs">
        {repoUrl}::{worktree.branch}
      </Badge>
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
        <Badge
          variant="secondary"
          className="text-xs bg-green-100 text-green-800 border-green-200"
        >
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

interface SessionHistoryItemProps {
  historyEntry: any;
  index: number;
  prStatus?: PullRequestInfo;
}

function SessionHistoryItem({
  historyEntry,
  index,
  prStatus,
}: SessionHistoryItemProps) {
  return (
    <div className="px-2 py-1.5 text-sm">
      <div className="flex items-center justify-between w-full">
        <div className="flex flex-col min-w-0">
          <div className="flex items-center justify-between w-full">
            <span className="truncate font-medium">{historyEntry.title}</span>
            {historyEntry.commit_hash && (
              <span className="ml-2 shrink-0">
                <CommitHashDisplay
                  commitHash={historyEntry.commit_hash}
                  prStatus={prStatus}
                />
              </span>
            )}
          </div>
          <span className="text-xs text-muted-foreground">
            {new Date(historyEntry.timestamp).toLocaleString()}
          </span>
        </div>
        {index === 0 && (
          <Badge variant="secondary" className="ml-2 text-xs shrink-0">
            Current
          </Badge>
        )}
      </div>
    </div>
  );
}

interface WorktreeActionDropdownProps {
  worktree: Worktree;
  mergeConflicts: Record<string, ConflictStatus>;
  prStatus?: PullRequestInfo;
  onSync: (id: string) => void;
  onMerge: (id: string, name: string) => void;
  onCreatePreview: (id: string, branch: string) => void;
  onConfirmDelete: (
    id: string,
    name: string,
    isDirty: boolean,
    commitCount: number,
  ) => void;
  onOpenPrDialog: (worktreeId: string, branchName: string) => void;
  onBranchFromWorktree: (worktreeId: string, name: string) => void;
}

function WorktreeActionDropdown({
  worktree,
  mergeConflicts,
  prStatus,
  onSync,
  onMerge,
  onCreatePreview,
  onConfirmDelete,
  onOpenPrDialog,
  onBranchFromWorktree,
}: WorktreeActionDropdownProps) {
  const handleDeleteClick = () => {
    onConfirmDelete(
      worktree.id,
      worktree.name,
      worktree.is_dirty,
      worktree.commit_count,
    );
  };

  return (
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

        <DropdownMenuItem
          onClick={() => onBranchFromWorktree(worktree.id, worktree.name)}
        >
          <GitBranch size={16} />
          Branch from this worktree
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

        {worktree.repo_id.startsWith("local/") && (
          <DropdownMenuItem
            onClick={() => onCreatePreview(worktree.id, worktree.branch)}
            className="text-purple-600"
          >
            <Eye size={16} />
            Create Preview
          </DropdownMenuItem>
        )}

        <DropdownMenuItem
          onClick={() => onOpenPrDialog(worktree.id, worktree.branch)}
          className={
            prStatus?.has_commits_ahead === false
              ? "text-muted-foreground"
              : "text-green-600"
          }
          disabled={
            prStatus?.has_commits_ahead === false || worktree.commit_count === 0
          }
          title={
            prStatus?.has_commits_ahead === false
              ? "No new commits to push to GitHub"
              : worktree.commit_count === 0
                ? "No commits in this worktree"
                : prStatus?.exists
                  ? "Update existing pull request on GitHub"
                  : "Create new pull request on GitHub"
          }
        >
          <GitMerge size={16} />
          {prStatus?.exists ? "Update PR (GitHub)" : "Create PR (GitHub)"}
        </DropdownMenuItem>

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
              : `Merge ${worktree.commit_count} commits`}
          </DropdownMenuItem>
        )}

        <DropdownMenuSeparator />

        <DropdownMenuItem onClick={handleDeleteClick} variant="destructive">
          <Trash2 size={16} />
          Delete Worktree
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

interface WorktreeHeaderProps {
  worktree: Worktree;
  hasConflicts: boolean;
  claudeSession?: ClaudeSession;
  repositoryUrl?: string;
}

function WorktreeHeader({
  worktree,
  hasConflicts,
  claudeSession,
  repositoryUrl,
}: WorktreeHeaderProps) {
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
        <StatusBadges
          worktree={worktree}
          hasConflicts={hasConflicts}
          claudeSession={claudeSession}
          repositoryUrl={repositoryUrl}
        />
      </div>
    </div>
  );
}

interface WorktreeClaudeStatusProps {
  worktree: Worktree;
  claudeSession?: ClaudeSession;
  prStatus?: PullRequestInfo;
}

interface SessionTitleProps {
  worktree: Worktree;
  isActive: boolean;
  prStatus?: PullRequestInfo;
}

function SessionTitle({ worktree, isActive, prStatus }: SessionTitleProps) {
  const { session_title, session_title_history = [] } = worktree;

  if (
    !session_title &&
    (!session_title_history || session_title_history.length === 0)
  ) {
    return null;
  }

  const displayTitle =
    session_title?.title ||
    session_title_history[session_title_history.length - 1]?.title;
  if (!displayTitle) {
    return null;
  }

  return (
    <div className="mt-2">
      {session_title_history && session_title_history.length > 1 ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className="h-auto p-1 justify-start hover:bg-muted"
            >
              <div className="flex items-center gap-2">
                {isActive && (
                  <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
                )}
                <span
                  className="text-sm font-medium text-foreground"
                  title={displayTitle}
                >
                  {displayTitle}
                </span>
                <ChevronDown size={12} className="text-muted-foreground" />
              </div>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            side="right"
            align="start"
            className="w-96 max-h-80 overflow-y-auto"
          >
            <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
              Session history
            </div>
            <DropdownMenuSeparator />
            {session_title_history
              .slice()
              .reverse()
              .map((historyEntry, index) => (
                <SessionHistoryItem
                  key={index}
                  historyEntry={historyEntry}
                  index={index}
                  prStatus={prStatus}
                />
              ))}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : (
        <div className="flex items-center gap-2 p-1">
          {isActive && (
            <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
          )}
          <div className="flex flex-col">
            <span
              className="text-sm font-medium text-foreground"
              title={displayTitle}
            >
              {displayTitle}
            </span>
            {session_title && (
              <span className="text-xs text-muted-foreground">
                {new Date(session_title.timestamp).toLocaleString()}
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function WorktreeClaudeStatus({
  worktree,
  claudeSession,
  prStatus,
}: WorktreeClaudeStatusProps) {
  if (!claudeSession) {
    return (
      <div>
        <p className="text-xs text-muted-foreground">No Claude sessions</p>
        <SessionTitle
          worktree={worktree}
          isActive={false}
          prStatus={prStatus}
        />
      </div>
    );
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
      <SessionTitle
        worktree={worktree}
        isActive={claudeSession.isActive}
        prStatus={prStatus}
      />
      <p className="text-xs text-muted-foreground mt-2">{sessionStatusText}</p>
    </div>
  );
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
  onConfirmDelete: (
    id: string,
    name: string,
    isDirty: boolean,
    commitCount: number,
  ) => void;
  onOpenPrDialog: (worktreeId: string, branchName: string) => void;
  onBranchFromWorktree: (worktreeId: string, name: string) => void;
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
  onOpenPrDialog,
  onBranchFromWorktree,
}: WorktreeActionsProps) {
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

      <WorktreeActionDropdown
        worktree={worktree}
        mergeConflicts={mergeConflicts}
        prStatus={prStatus}
        onSync={onSync}
        onMerge={onMerge}
        onCreatePreview={onCreatePreview}
        onConfirmDelete={onConfirmDelete}
        onOpenPrDialog={onOpenPrDialog}
        onBranchFromWorktree={onBranchFromWorktree}
      />
    </div>
  );
}

interface WorktreeRowPropsWithPR extends WorktreeRowProps {
  prStatuses?: Record<string, PullRequestInfo | undefined>;
  repositories?: Record<string, LocalRepository>;
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
  onBranchFromWorktree,
  prStatuses,
  repositories,
}: WorktreeRowPropsWithPR) {
  const sessionPath = worktree.path;
  const claudeSession = claudeSessions[sessionPath];
  const hasConflicts = Boolean(
    syncConflicts[worktree.id]?.has_conflicts ??
      mergeConflicts[worktree.id]?.has_conflicts,
  );
  const summary = worktreeSummaries[worktree.id];
  // const diffStat = diffStats[worktree.id];
  const prStatus = prStatuses?.[worktree.id];
  const repositoryUrl = repositories?.[worktree.repo_id]?.url;

  const openPrDialog = (worktreeId: string, branchName: string) => {
    // Check if PR already exists
    const isUpdate = prStatus?.exists ?? false;

    // Use pre-generated summary if available, or existing PR data if updating
    const defaultTitle =
      isUpdate && prStatus?.title
        ? prStatus.title
        : summary?.status === "completed"
          ? summary.title
          : `Pull request from ${branchName}`;

    const defaultDescription =
      isUpdate && prStatus?.body
        ? prStatus.body
        : summary?.status === "completed"
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

  const handleBranchFromWorktree = (worktreeId: string, name: string) => {
    onBranchFromWorktree(worktreeId, name);
  };

  // const totalAdditions = diffStat?.file_diffs?.filter(diff => diff.change_type === 'added').length ?? 0;
  // const totalDeletions = diffStat?.file_diffs?.filter(diff => diff.change_type === 'deleted').length ?? 0;

  return (
    <div className="border rounded-lg p-4 mb-4 bg-card">
      <div className="flex items-center justify-between">
        <div className="flex-1">
          <WorktreeHeader
            worktree={worktree}
            hasConflicts={hasConflicts}
            claudeSession={claudeSession}
            repositoryUrl={repositoryUrl}
          />

          <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
            <span className="text-xs">{worktree.commit_hash.slice(0, 8)}</span>
            <span className="text-xs">
              source branch:{" "}
              <span className="font-bold">{worktree.source_branch}</span>
            </span>
            {worktree.commit_count > 0 && (
              <span>
                {worktree.commit_count} commit
                {worktree.commit_count !== 1 ? "s" : ""}
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

          <div className="flex items-center gap-4">
            <div className="text-xs text-muted-foreground">
              <WorktreeClaudeStatus
                worktree={worktree}
                claudeSession={claudeSession}
                prStatus={prStatus}
              />
            </div>
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
          onBranchFromWorktree={handleBranchFromWorktree}
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
