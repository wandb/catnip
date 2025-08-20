import { useState, useEffect, useRef } from "react";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { TodoDisplay } from "@/components/TodoDisplay";
import { DiffViewer } from "@/components/DiffViewer";
import { useAppStore } from "@/stores/appStore";
import type { Worktree, LocalRepository } from "@/lib/git-api";

interface WorkspaceMobileProps {
  worktree: Worktree;
  repository: LocalRepository;
}

export function WorkspaceMobile({
  worktree,
  repository,
}: WorkspaceMobileProps) {
  const [prompt, setPrompt] = useState("");
  const [phase, setPhase] = useState<"input" | "todos" | "summary">("input");
  const wsRef = useRef<WebSocket | null>(null);

  const currentWorktree = useAppStore((state) => state.worktrees[worktree.id]);

  useEffect(() => {
    if (!currentWorktree) return;
    if (phase === "input") {
      if (currentWorktree.claude_activity_state === "active") {
        setPhase("todos");
      } else if (currentWorktree.claude_activity_state === "running") {
        setPhase("summary");
      }
    } else if (
      phase === "todos" &&
      currentWorktree.claude_activity_state === "running"
    ) {
      setPhase("summary");
    }
  }, [currentWorktree?.claude_activity_state, phase]);

  useEffect(() => {
    return () => {
      wsRef.current?.close();
    };
  }, []);

  const startSession = () => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const params = new URLSearchParams();
    params.set("session", worktree.name);
    params.set("agent", "claude");
    const url = `${protocol}//${window.location.host}/v1/pty?${params.toString()}`;
    const ws = new WebSocket(url);
    ws.onopen = () => {
      ws.send(JSON.stringify({ type: "prompt", data: prompt, submit: true }));
    };
    wsRef.current = ws;
    setPhase("todos");
  };

  if (phase === "input") {
    return (
      <div className="p-4 space-y-4">
        <div className="text-sm text-muted-foreground">
          {repository.name} / {worktree.branch}
        </div>
        <Textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="Describe your task..."
          className="min-h-[120px]"
        />
        <Button
          onClick={startSession}
          disabled={!prompt.trim()}
          className="w-full"
        >
          Start
        </Button>
      </div>
    );
  }

  if (phase === "todos") {
    return (
      <div className="p-4 space-y-4">
        <div className="text-sm text-muted-foreground">
          Claude is working...
        </div>
        <TodoDisplay todos={currentWorktree?.todos || []} />
      </div>
    );
  }

  return (
    <div className="p-4 space-y-4">
      <DiffViewer worktreeId={worktree.id} isOpen={true} onClose={() => {}} />
    </div>
  );
}
