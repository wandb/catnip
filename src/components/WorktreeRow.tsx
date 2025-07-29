import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { DiffViewer } from "@/components/DiffViewer";
import { WorkspaceActions } from "@/components/WorkspaceActions";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  ChevronDown,
  FileText,
  Copy,
  Check,
  Loader2,
  AlertTriangle,
} from "lucide-react";
import {
  type Worktree,
  type WorktreeDiffStats,
  type PullRequestInfo,
  type LocalRepository,
} from "@/lib/git-api";
import { type WorktreeSummary } from "@/lib/worktree-summary";
import { getRelativeTime, getDuration } from "@/lib/git-utils";
// ConflictStatus type moved - conflicts now tracked directly on worktree.has_conflicts

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
  _syncConflicts: Record<string, any>; // ConflictStatus type removed
  _mergeConflicts: Record<string, any>; // ConflictStatus type removed
  worktreeSummaries: Record<string, WorktreeSummary>;
  diffStats: Record<string, WorktreeDiffStats | undefined>;
  diffStatsLoading: boolean;
  openDiffWorktreeId: string | null;
  setPrDialog: React.Dispatch<
    React.SetStateAction<{
      open: boolean;
      worktreeId: string;
      branchName: string;
      title: string;
      description: string;
      isUpdate: boolean;
      isGenerating?: boolean;
    }>
  >;
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
  claudeSession?: ClaudeSession;
  repositoryUrl?: string;
  prStatus?: PullRequestInfo;
}

function StatusBadges({
  worktree,
  claudeSession,
  repositoryUrl,
  prStatus,
}: StatusBadgesProps) {
  let repoUrl = repositoryUrl;
  if (repoUrl && repoUrl.startsWith("file:///live/")) {
    repoUrl = repoUrl.slice(13);
  }

  const badgeContent = `${repoUrl}::${worktree.branch}`;
  const hasOpenPR = prStatus?.exists && prStatus.url;

  return (
    <div className="flex items-center gap-2">
      {hasOpenPR ? (
        <a
          href={prStatus.url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-block"
        >
          <Badge
            variant="outline"
            className="text-xs bg-sky-50 border-sky-200 text-sky-800 hover:bg-sky-100 transition-colors cursor-pointer"
          >
            {badgeContent}
          </Badge>
        </a>
      ) : (
        <Badge variant="outline" className="text-xs">
          {badgeContent}
        </Badge>
      )}
      {!worktree.cache_status?.is_cached && worktree.is_dirty === undefined ? (
        <Skeleton className="w-12 h-6" />
      ) : worktree.has_conflicts ? (
        <Badge variant="destructive" className="text-xs">
          <AlertTriangle size={12} className="mr-1" />
          Conflicts
        </Badge>
      ) : worktree.is_dirty ? (
        <Badge
          variant="secondary"
          className="text-xs bg-orange-100 text-orange-800 border-orange-200"
        >
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
      {worktree.cache_status?.is_loading && (
        <Badge variant="secondary" className="text-xs">
          <Loader2 className="w-3 h-3 mr-1 animate-spin" />
          Updating...
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

interface WorktreeHeaderProps {
  worktree: Worktree;
  claudeSession?: ClaudeSession;
  repositoryUrl?: string;
  prStatus?: PullRequestInfo;
}

function WorktreeHeader({
  worktree,
  claudeSession,
  repositoryUrl,
  prStatus,
}: WorktreeHeaderProps) {
  return (
    <div className="flex items-center gap-3">
      <div className="flex items-center gap-2">
        <Link
          to="/terminal/$sessionId"
          params={{ sessionId: worktree.name }}
          className="text-lg font-medium hover:underline"
        >
          {worktree.name}
        </Link>
        <StatusBadges
          worktree={worktree}
          claudeSession={claudeSession}
          repositoryUrl={repositoryUrl}
          prStatus={prStatus}
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
    <div className="mt-2 flex items-center gap-2">
      {isActive ? (
        <div
          className="w-2 h-2 bg-green-500 rounded-full animate-pulse"
          title="Active"
        />
      ) : (
        <div className="w-2 h-2 bg-gray-500 rounded-full" title="Inactive" />
      )}
      {session_title_history && session_title_history.length >= 1 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className="h-auto p-1 justify-start hover:bg-muted"
            >
              <div className="flex items-center gap-2">
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
        <div className="flex items-center gap-2 mt-1">
          <p className="text-xs text-muted-foreground">No Claude sessions</p>
        </div>
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
  mergeConflicts: Record<string, any>; // ConflictStatus type removed
  diffStats: Record<string, WorktreeDiffStats | undefined>;
  diffStatsLoading: boolean;
  openDiffWorktreeId: string | null;
  diffLoading: boolean;
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
  isSyncing?: boolean;
  isMerging?: boolean;
}

function WorktreeActions({
  worktree,
  mergeConflicts,
  diffStats,
  diffStatsLoading,
  openDiffWorktreeId,
  diffLoading,
  prStatus,
  onToggleDiff,
  onSync,
  onMerge,
  onCreatePreview,
  onConfirmDelete,
  onOpenPrDialog,
  isSyncing = false,
  isMerging = false,
}: WorktreeActionsProps) {
  const hasDiff = (diffStats[worktree.id]?.file_diffs?.length ?? 0) > 0;
  const isLoading =
    diffStatsLoading || (diffLoading && openDiffWorktreeId === worktree.id);

  return (
    <div className="flex items-center gap-2">
      <div
        title={
          isLoading
            ? "Loading diff..."
            : !hasDiff
              ? "No changes to show"
              : undefined
        }
      >
        <Button
          variant="outline"
          size="sm"
          onClick={() => onToggleDiff(worktree.id)}
          disabled={isLoading || !hasDiff}
          className={openDiffWorktreeId === worktree.id ? "bg-muted" : ""}
        >
          {diffStatsLoading ? (
            <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current mr-2"></div>
          ) : (
            <FileText size={16} className="mr-2" />
          )}
          {openDiffWorktreeId === worktree.id ? "Hide" : "View"} Diff
        </Button>
      </div>

      <Link
        to="/terminal/$sessionId"
        params={{ sessionId: worktree.name }}
        search={{ agent: "claude" }}
      >
        <Button variant="outline" size="sm" asChild>
          <span className="flex items-center gap-1.5">
            <img src="/anthropic.png" alt="Claude" className="w-4 h-4" />
            Vibe
          </span>
        </Button>
      </Link>

      <WorkspaceActions
        mode="worktree"
        worktree={worktree}
        mergeConflicts={mergeConflicts}
        prStatus={prStatus}
        onSync={onSync}
        onMerge={onMerge}
        onCreatePreview={onCreatePreview}
        onConfirmDelete={onConfirmDelete}
        onOpenPrDialog={onOpenPrDialog}
        isSyncing={isSyncing}
        isMerging={isMerging}
      />
    </div>
  );
}

interface WorktreeRowPropsWithPR extends WorktreeRowProps {
  prStatuses?: Record<string, PullRequestInfo | undefined>;
  repositories?: Record<string, LocalRepository>;
  isSyncing?: boolean;
  isMerging?: boolean;
}

export function WorktreeRow({
  worktree,
  claudeSessions,
  _syncConflicts,
  _mergeConflicts,
  worktreeSummaries,
  diffStats,
  diffStatsLoading,
  openDiffWorktreeId,
  setPrDialog,
  onToggleDiff,
  onSync,
  onMerge,
  onCreatePreview,
  onConfirmDelete,
  prStatuses,
  repositories,
  isSyncing = false,
  isMerging = false,
}: WorktreeRowPropsWithPR) {
  const [diffLoading, setDiffLoading] = useState(false);
  const [lastClaudeCall, setLastClaudeCall] = useState<number>(0);

  const sessionPath = worktree.path;
  const claudeSession = claudeSessions[sessionPath];
  const summary = worktreeSummaries[worktree.id];

  // Keep this to satisfy the interface - may be used in debugging
  console.debug("Sync conflicts available:", _syncConflicts);

  // const diffStat = diffStats[worktree.id];
  const prStatus = prStatuses?.[worktree.id];
  const repositoryUrl = repositories?.[worktree.repo_id]?.url;

  const openPrDialog = async (worktreeId: string, branchName: string) => {
    console.log("🚀 openPrDialog called", { worktreeId, branchName });
    console.log("📊 PR Status:", prStatus);
    console.log("📋 Summary:", summary);
    console.log("⏰ lastClaudeCall:", lastClaudeCall);

    // Check if PR already exists
    const isUpdate = prStatus?.exists ?? false;
    console.log("🔄 isUpdate:", isUpdate);

    // If this is an update to an existing PR, use the existing PR data
    if (isUpdate && prStatus?.title) {
      console.log("✅ Using existing PR data");
      setPrDialog({
        open: true,
        worktreeId,
        branchName,
        title: prStatus.title,
        description: prStatus.body || "",
        isUpdate,
        isGenerating: false,
      });
      return;
    }

    // Check throttling - only allow Claude call once every 10 seconds
    const now = Date.now();
    const shouldCallClaude = now - lastClaudeCall > 10000; // 10 seconds
    console.log(
      "🤖 shouldCallClaude:",
      shouldCallClaude,
      "time since last call:",
      now - lastClaudeCall,
    );

    if (!shouldCallClaude) {
      console.log("⏸️ Throttled - using fallback data");
      // Use fallback data without calling Claude
      const fallbackTitle =
        summary?.status === "completed" && summary.title
          ? summary.title
          : `Pull request from ${branchName}`;

      const fallbackDescription =
        summary?.status === "completed" && summary.summary
          ? summary.summary
          : `Automated pull request created from worktree ${branchName}`;

      console.log("📝 Fallback data:", { fallbackTitle, fallbackDescription });

      setPrDialog({
        open: true,
        worktreeId,
        branchName,
        title: fallbackTitle,
        description: fallbackDescription,
        isUpdate,
        isGenerating: false,
      });
      return;
    }

    // Open dialog with loading state and call Claude
    console.log("🔄 Calling Claude API for PR generation");
    setPrDialog({
      open: true,
      worktreeId,
      branchName,
      title: "",
      description: "",
      isUpdate,
      isGenerating: true,
    });

    // Update throttle timestamp
    setLastClaudeCall(now);

    try {
      // Prepare prompt for Claude - it already has the session context
      const prompt = `I need you to generate a pull request title and description for the branch "${branchName}" based on all the changes we've made in this session.

Please respond with JSON in the following format:
\`\`\`json
{
  "title": "Brief, descriptive title of the changes",
  "description": "Focused description of what was changed and why, formatted in markdown"
}
\`\`\`

Make the title concise but descriptive. Keep the description focused but informative - use 1-3 paragraphs explaining:
- What was changed
- Why it was changed
- Any key implementation notes

Avoid overly lengthy explanations or step-by-step implementation details.`;

      // Call Claude API
      const requestBody = {
        prompt: prompt,
        working_directory: `/workspace/${worktree.name}`,
        resume: true,
        max_turns: 1,
      };
      console.log("📤 Sending to Claude API:", requestBody);

      const response = await fetch("/v1/claude/messages", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(requestBody),
      });

      if (response.ok) {
        const data = await response.json();
        console.log("✅ Claude API response received:", data);

        // Extract JSON from Claude's response
        let parsedData = { title: "", description: "" };
        try {
          // The response is in data.response field
          const responseText = data.response || data.message || "";
          console.log("🔍 Parsing response text:", responseText);

          // Look for JSON in code fence (handle newlines properly)
          const jsonMatch = responseText.match(/```json\s*([\s\S]*?)\s*```/m);
          if (jsonMatch) {
            console.log("🎯 Extracted JSON from code fence:", jsonMatch[1]);
            parsedData = JSON.parse(jsonMatch[1]);
          } else {
            console.log(
              "🔍 No code fence found, trying to parse whole response as JSON",
            );
            // Try parsing the whole response as JSON
            parsedData = JSON.parse(responseText);
          }
        } catch (e) {
          console.error("Failed to parse Claude's response as JSON:", e);
          // Fallback to using the raw response
          parsedData = {
            title: `PR: ${branchName}`,
            description:
              data.response || data.message || "Generated PR content",
          };
        }

        // Update dialog with generated content
        setPrDialog((prev) => ({
          ...prev,
          title: parsedData.title || `Pull request from ${branchName}`,
          description:
            parsedData.description || `Changes from worktree ${branchName}`,
          isGenerating: false,
        }));
      } else {
        console.error("❌ Claude API failed with status:", response.status);
        const errorText = await response.text();
        console.error("❌ Error details:", errorText);

        // Fallback to summary or defaults
        const fallbackTitle =
          summary?.status === "completed" && summary.title
            ? summary.title
            : `Pull request from ${branchName}`;

        const fallbackDescription =
          summary?.status === "completed" && summary.summary
            ? summary.summary
            : `Automated pull request created from worktree ${branchName}`;

        setPrDialog((prev) => ({
          ...prev,
          title: fallbackTitle,
          description: fallbackDescription,
          isGenerating: false,
        }));
      }
    } catch (error) {
      console.error("Error generating PR details:", error);
      // Fallback to summary or defaults
      const fallbackTitle =
        summary?.status === "completed" && summary.title
          ? summary.title
          : `Pull request from ${branchName}`;

      const fallbackDescription =
        summary?.status === "completed" && summary.summary
          ? summary.summary
          : `Automated pull request created from worktree ${branchName}`;

      setPrDialog((prev) => ({
        ...prev,
        title: fallbackTitle,
        description: fallbackDescription,
        isGenerating: false,
      }));
    }
  };

  // const totalAdditions = diffStat?.file_diffs?.filter(diff => diff.change_type === 'added').length ?? 0;
  // const totalDeletions = diffStat?.file_diffs?.filter(diff => diff.change_type === 'deleted').length ?? 0;

  return (
    <div className="border rounded-lg p-4 mb-4 bg-card">
      <div className="flex items-center justify-between">
        <div className="flex-1">
          <WorktreeHeader
            worktree={worktree}
            claudeSession={claudeSession}
            repositoryUrl={repositoryUrl}
            prStatus={prStatus}
          />

          <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
            <span className="text-xs">{worktree.commit_hash.slice(0, 8)}</span>
            <span className="text-xs">
              source branch:{" "}
              <span className="font-bold">{worktree.source_branch}</span>
            </span>
            {!worktree.cache_status?.is_cached &&
            worktree.commit_count === undefined ? (
              <Skeleton className="w-16 h-4" />
            ) : (
              worktree.commit_count > 0 && (
                <span>
                  {worktree.commit_count} commit
                  {worktree.commit_count !== 1 ? "s" : ""}
                </span>
              )
            )}
            {/* TODO: Add total changes */}
            {/* {diffStat && (diffStat.file_diffs?.length ?? 0) > 0 && (
              <span className="flex items-center gap-1 font-mono text-xs">
                <span className="text-green-600">+{totalAdditions}</span>
                /<span className="text-red-600">-{totalDeletions}</span>
              </span>
            )} */}
            {!worktree.cache_status?.is_cached &&
            worktree.commits_behind === undefined ? (
              <Skeleton className="w-12 h-4" />
            ) : (
              worktree.commits_behind > 0 && (
                <span className="text-orange-600">
                  {worktree.commits_behind} behind
                </span>
              )
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
          mergeConflicts={_mergeConflicts}
          diffStats={diffStats}
          diffStatsLoading={diffStatsLoading}
          openDiffWorktreeId={openDiffWorktreeId}
          diffLoading={diffLoading}
          prStatus={prStatus}
          onToggleDiff={onToggleDiff}
          onSync={onSync}
          onMerge={onMerge}
          onCreatePreview={onCreatePreview}
          onConfirmDelete={onConfirmDelete}
          onOpenPrDialog={openPrDialog}
          isSyncing={isSyncing}
          isMerging={isMerging}
        />
      </div>
      <DiffViewer
        worktreeId={worktree.id}
        isOpen={openDiffWorktreeId === worktree.id}
        onClose={() => onToggleDiff(worktree.id)}
        onLoadingChange={setDiffLoading}
      />
    </div>
  );
}
