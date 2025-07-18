import { Link, useNavigate } from "@tanstack/react-router";
import { type Worktree } from "@/lib/git-api";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { useState } from "react";

interface WorkspaceCardProps {
  worktree: Worktree;
}

export function WorkspaceCard({ worktree }: WorkspaceCardProps) {
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
    <div className="w-[350px] h-[350px] border rounded-lg bg-card hover:bg-muted flex flex-col justify-between p-4 transition-colors">
      <div className="space-y-3">
        <Link
          to="/terminal/$sessionId"
          params={{ sessionId: encodeURIComponent(worktree.name) }}
          className="block space-y-1 hover:opacity-80 transition-opacity"
        >
          <h2 className="text-xl font-semibold break-all">{worktree.name}</h2>
          <p className="text-sm text-muted-foreground break-all">
            {worktree.branch}
          </p>
        </Link>

        <form onSubmit={handlePromptSubmit} className="space-y-2">
          <Textarea
            placeholder="Ask Claude to help with this workspace..."
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            className="text-sm resize-none min-h-[40px] max-h-[120px]"
            rows={1}
            style={{
              height: "auto",
              minHeight: "40px",
              maxHeight: "120px",
              overflowY: prompt.length > 0 ? "auto" : "hidden",
            }}
            onInput={(e) => {
              const target = e.target as HTMLTextAreaElement;
              target.style.height = "auto";
              target.style.height = Math.min(target.scrollHeight, 120) + "px";
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

      <div className="text-sm text-muted-foreground">
        {worktree.commit_count} commit{worktree.commit_count === 1 ? "" : "s"}
      </div>
    </div>
  );
}
