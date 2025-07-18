import { Link, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  MoreHorizontal,
  Terminal,
  GitBranch,
  Copy,
  Check,
  Trash2,
  Clock,
  User,
} from "lucide-react";
import { type Worktree } from "@/lib/git-api";
import { getRelativeTime } from "@/lib/git-utils";
import { useState } from "react";

interface WorkspaceCardProps {
  worktree: Worktree;
  onDelete?: (id: string, name: string) => void;
}

export function WorkspaceCard({ worktree, onDelete }: WorkspaceCardProps) {
  const [prompt, setPrompt] = useState("");
  const navigate = useNavigate();

  const handlePromptSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (prompt.trim()) {
      void navigate({
        to: "/terminal/$sessionId",
        params: { sessionId: encodeURIComponent(worktree.name) },
        search: {
          agent: "claude",
          prompt: prompt,
        },
      });
    }
  };

  return (
    <div className="w-[350px] h-[350px] border rounded-lg bg-card hover:bg-muted flex flex-col justify-between p-4 transition-colors relative group">
      {/* Header with title and actions */}
      <div className="space-y-3">
        <div className="flex items-start justify-between">
          <Link
            to="/terminal/$sessionId"
            params={{ sessionId: encodeURIComponent(worktree.name) }}
            className="flex-1 min-w-0"
          >
            <h2 className="text-xl font-semibold break-all hover:underline">
              {worktree.name}
            </h2>
          </Link>
          <div className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity">
            <WorkspaceActions worktree={worktree} onDelete={onDelete} />
          </div>
        </div>

        {/* Branch info */}
        <div className="flex items-center gap-2">
          <GitBranch size={14} className="text-muted-foreground" />
          <Badge variant="outline" className="text-xs">
            {worktree.branch}
          </Badge>
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
        </div>

        {/* Commit info */}
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Commit:</span>
            <CommitHashDisplay commitHash={worktree.commit_hash} />
          </div>

          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Source:</span>
            <span className="text-sm font-medium">
              {worktree.source_branch}
            </span>
          </div>
        </div>

        {/* Prompt input form */}
        <form onSubmit={handlePromptSubmit} className="space-y-2">
          <Textarea
            placeholder="Ask Claude to help with this workspace..."
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            className="text-sm resize-none min-h-[40px] max-h-[80px]"
            rows={1}
            style={{
              height: "auto",
              minHeight: "40px",
              maxHeight: "80px",
              overflowY: prompt.length > 0 ? "auto" : "hidden",
            }}
            onInput={(e) => {
              const target = e.target as HTMLTextAreaElement;
              target.style.height = "auto";
              target.style.height = Math.min(target.scrollHeight, 80) + "px";
            }}
          />
          <Button
            type="submit"
            size="sm"
            className="w-full"
            disabled={!prompt.trim()}
          >
            Chat with Claude
          </Button>
        </form>
      </div>

      {/* Footer with stats */}
      <div className="space-y-2">
        <div className="flex items-center justify-between text-sm text-muted-foreground">
          <div className="flex items-center gap-1">
            <Clock size={14} />
            <span>{getRelativeTime(worktree.created_at)}</span>
          </div>
          <div>
            {worktree.commit_count} commit
            {worktree.commit_count === 1 ? "" : "s"}
          </div>
        </div>

        {worktree.commits_behind > 0 && (
          <div className="text-xs text-orange-600">
            {worktree.commits_behind} commits behind {worktree.source_branch}
          </div>
        )}
      </div>
    </div>
  );
}

interface CommitHashDisplayProps {
  commitHash: string;
}

function CommitHashDisplay({ commitHash }: CommitHashDisplayProps) {
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

  const isCopied = copiedHash === commitHash;
  return (
    <button
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        void copyToClipboard(commitHash);
      }}
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

interface WorkspaceActionsProps {
  worktree: Worktree;
  onDelete?: (id: string, name: string) => void;
}

function WorkspaceActions({ worktree, onDelete }: WorkspaceActionsProps) {
  const handleDelete = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (onDelete) {
      onDelete(worktree.id, worktree.name);
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 w-8 p-0"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
          }}
        >
          <MoreHorizontal size={16} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
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

        <DropdownMenuItem asChild>
          <Link
            to="/terminal/$sessionId"
            params={{ sessionId: encodeURIComponent(worktree.name) }}
            search={{ agent: "claude" }}
            className="flex items-center gap-2"
          >
            <User size={16} />
            Open with Claude
          </Link>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        {onDelete && (
          <DropdownMenuItem onClick={handleDelete} className="text-red-600">
            <Trash2 size={16} />
            Delete Workspace
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
