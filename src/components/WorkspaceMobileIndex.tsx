import { useState, useEffect } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useAppStore } from "@/stores/appStore";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { useClaudeApi } from "@/hooks/useClaudeApi";
import {
  getWorkspaceTitle,
  getStatusIndicatorClasses,
} from "@/lib/workspace-utils";
import type { ClaudeSessionSummary } from "@/lib/claude-api";

interface WorkspaceCardProps {
  name: string;
  branch: string;
  repoName: string;
  available: boolean;
  claudeSessionSummary?: ClaudeSessionSummary;
  commitCount?: number;
  isDirty?: boolean;
  worktree: any; // Full worktree object for status and title logic
}

function WorkspaceCard({
  name,
  branch,
  repoName,
  available,
  claudeSessionSummary,
  commitCount,
  isDirty,
  worktree,
}: WorkspaceCardProps) {
  const parts = name.split("/");
  const project = parts[0];
  const workspace = parts[1];

  const hasSession = claudeSessionSummary && claudeSessionSummary.turnCount > 0;

  // Clean up branch name by removing leading slash
  const cleanBranch = branch.startsWith("/") ? branch.slice(1) : branch;

  // Format diff stats
  const diffStats = commitCount && commitCount > 0 ? `+${commitCount}` : null;

  // Get workspace title using shared utility
  const title = getWorkspaceTitle(worktree);

  return (
    <Card className="p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="space-y-1 flex-1 min-w-0 mr-4">
          <div className="flex items-center gap-2">
            <div className={getStatusIndicatorClasses(worktree)} />
            <h3 className="font-semibold text-lg truncate">{title}</h3>
          </div>
          <div className="text-sm text-muted-foreground">
            {repoName}/{workspace} · {cleanBranch}
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
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
        </div>
      </div>

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
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const { getAllWorktreeSessionSummaries } = useClaudeApi();

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
              commitCount={worktree.commit_count}
              isDirty={worktree.is_dirty}
              worktree={worktree}
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
