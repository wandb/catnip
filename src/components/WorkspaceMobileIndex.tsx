import { useState, useEffect } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useAppStore } from "@/stores/appStore";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { useClaudeApi } from "@/hooks/useClaudeApi";
import type { ClaudeSessionSummary } from "@/lib/claude-api";

interface WorkspaceCardProps {
  name: string;
  branch: string;
  repoName: string;
  available: boolean;
  claudeSessionSummary?: ClaudeSessionSummary;
  lastAssistantMessage?: string;
  commitCount?: number;
  isDirty?: boolean;
  pullRequestTitle?: string;
}

function WorkspaceCard({
  name,
  branch,
  repoName,
  available,
  claudeSessionSummary,
  lastAssistantMessage,
  commitCount,
  isDirty,
  pullRequestTitle,
}: WorkspaceCardProps) {
  const parts = name.split("/");
  const project = parts[0];
  const workspace = parts[1];

  const hasSession = claudeSessionSummary && claudeSessionSummary.turnCount > 0;
  const isActive = claudeSessionSummary?.isActive ?? false;

  // Clean up branch name by removing leading slash
  const cleanBranch = branch.startsWith("/") ? branch.slice(1) : branch;

  // Format diff stats
  const diffStats = commitCount && commitCount > 0 ? `+${commitCount}` : null;

  return (
    <Card className="p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <h3 className="font-semibold text-lg">{cleanBranch}</h3>
          <div className="text-sm text-muted-foreground">
            {repoName}/{workspace}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {diffStats && (
            <span className="text-xs font-mono text-muted-foreground">
              {diffStats}
            </span>
          )}
          {isDirty && (
            <Badge variant="outline" className="text-xs">
              Modified
            </Badge>
          )}
          {isActive && (
            <Badge
              variant="secondary"
              className="bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100"
            >
              Active
            </Badge>
          )}
        </div>
      </div>

      {pullRequestTitle && (
        <div className="text-sm font-medium text-muted-foreground line-clamp-2">
          {pullRequestTitle}
        </div>
      )}

      {hasSession && lastAssistantMessage && !pullRequestTitle && (
        <div className="space-y-2">
          <div className="text-sm text-muted-foreground">Last response:</div>
          <div className="text-sm bg-muted p-3 rounded-md line-clamp-3">
            {lastAssistantMessage.length > 150
              ? lastAssistantMessage.substring(0, 150) + "..."
              : lastAssistantMessage}
          </div>
        </div>
      )}

      <div className="flex gap-2">
        {available ? (
          <Button asChild className="flex-1">
            <Link
              to="/workspace/$project/$workspace"
              params={{ project, workspace }}
              search={{ prompt: undefined }}
            >
              {hasSession ? "Continue" : "Open"}
            </Link>
          </Button>
        ) : (
          <Button disabled className="flex-1">
            Unavailable
          </Button>
        )}
      </div>
    </Card>
  );
}

export function WorkspaceMobileIndex() {
  const [claudeSessions, setClaudeSessions] = useState<
    Record<string, ClaudeSessionSummary>
  >({});
  const [latestMessages, setLatestMessages] = useState<Record<string, string>>(
    {},
  );
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const { getAllWorktreeSessionSummaries, getWorktreeLatestAssistantMessage } =
    useClaudeApi();

  const worktreesCount = useAppStore(
    (state) => state.getWorktreesList().length,
  );
  const getWorktreesList = useAppStore((state) => state.getWorktreesList);
  const getRepositoryById = useAppStore((state) => state.getRepositoryById);

  useEffect(() => {
    let isMounted = true;

    const loadClaudeData = async () => {
      try {
        // Get all Claude session summaries
        const sessions = await getAllWorktreeSessionSummaries();
        if (!isMounted) return;
        setClaudeSessions(sessions);

        // Get latest assistant messages for worktrees with sessions
        const messagePromises = Object.keys(sessions).map(
          async (worktreePath) => {
            if (sessions[worktreePath]?.turnCount > 0) {
              try {
                const message =
                  await getWorktreeLatestAssistantMessage(worktreePath);
                return { worktreePath, message };
              } catch (error) {
                console.warn(
                  `Failed to get latest message for ${worktreePath}:`,
                  error,
                );
                return { worktreePath, message: "" };
              }
            }
            return { worktreePath, message: "" };
          },
        );

        const messageResults = await Promise.all(messagePromises);
        if (!isMounted) return;

        const messagesMap = messageResults.reduce(
          (acc, { worktreePath, message }) => {
            if (message) {
              acc[worktreePath] = message;
            }
            return acc;
          },
          {} as Record<string, string>,
        );

        setLatestMessages(messagesMap);
      } catch (error) {
        if (!isMounted) return;
        console.error("Failed to load Claude data:", error);
      } finally {
        if (isMounted) {
          setLoading(false);
        }
      }
    };

    const currentWorktreesCount = getWorktreesList().length;
    if (currentWorktreesCount > 0) {
      void loadClaudeData();
    } else {
      setLoading(false);
    }

    return () => {
      isMounted = false;
    };
  }, []); // Only run once on mount

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <LoadingSpinner message="Loading workspaces..." size="lg" />
      </div>
    );
  }

  if (worktreesCount === 0) {
    return (
      <div className="flex h-screen items-center justify-center p-4">
        <div className="text-center space-y-4">
          <h2 className="text-xl font-semibold">No workspaces</h2>
          <p className="text-muted-foreground">
            Create a workspace to get started
          </p>
        </div>
      </div>
    );
  }

  const worktrees = getWorktreesList();

  // Filter to only show available workspaces and sort by last_accessed
  const availableWorktrees = worktrees
    .filter((worktree) => {
      const repository = getRepositoryById(worktree.repo_id);
      return repository && repository.available;
    })
    .sort((a, b) => {
      const aAccessed = new Date(a.last_accessed || a.created_at).getTime();
      const bAccessed = new Date(b.last_accessed || b.created_at).getTime();
      return bAccessed - aAccessed; // Most recent first
    });

  return (
    <div className="min-h-screen bg-background">
      <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="p-4">
          <h1 className="text-xl font-semibold">Workspaces</h1>
          <p className="text-sm text-muted-foreground">
            {availableWorktrees.length} workspaces
          </p>
        </div>
      </div>

      <div className="p-4 space-y-4 pb-20">
        {availableWorktrees.map((worktree) => {
          const repository = getRepositoryById(worktree.repo_id);
          if (!repository) return null;

          // Fallback to extract repo name from worktree name if repository.name is empty
          const repoName =
            repository.name || worktree.name.split("/")[0] || "Unknown";

          return (
            <WorkspaceCard
              key={worktree.id}
              name={worktree.name}
              branch={worktree.branch}
              repoName={repoName}
              available={repository.available}
              claudeSessionSummary={claudeSessions[worktree.path]}
              lastAssistantMessage={latestMessages[worktree.path]}
              commitCount={worktree.commit_count}
              isDirty={worktree.is_dirty}
              pullRequestTitle={worktree.pull_request_title}
            />
          );
        })}
      </div>

      {/* Fixed New Workspace Button */}
      <div className="fixed bottom-0 left-0 right-0 bg-background border-t p-4">
        <Button
          onClick={() => navigate({ to: "/workspace/new" })}
          className="w-full"
        >
          New Workspace
        </Button>
      </div>
    </div>
  );
}
