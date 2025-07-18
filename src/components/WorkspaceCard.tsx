import { Link } from "@tanstack/react-router";
import { type Worktree } from "@/lib/git-api";

interface WorkspaceCardProps {
  worktree: Worktree;
}

export function WorkspaceCard({ worktree }: WorkspaceCardProps) {
  return (
    <Link
      to="/terminal/$sessionId"
      params={{ sessionId: encodeURIComponent(worktree.name) }}
      className="w-[350px] h-[350px] border rounded-lg bg-card hover:bg-muted flex flex-col justify-between p-4 transition-colors"
    >
      <div className="space-y-1">
        <h2 className="text-xl font-semibold break-all">{worktree.name}</h2>
        <p className="text-sm text-muted-foreground break-all">
          {worktree.branch}
        </p>
      </div>
      <div className="text-sm text-muted-foreground">
        {worktree.commit_count} commit{worktree.commit_count === 1 ? "" : "s"}
      </div>
    </Link>
  );
}
