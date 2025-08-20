import { useState, useEffect, useRef } from "react";
import { Link } from "@tanstack/react-router";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { TodoDisplay } from "@/components/TodoDisplay";
import { DiffViewer } from "@/components/DiffViewer";
import { TextContent } from "@/components/TextContent";
import { PullRequestDialog } from "@/components/PullRequestDialog";
import { useAppStore } from "@/stores/appStore";
import { useClaudeApi } from "@/hooks/useClaudeApi";
import { GitMerge, ExternalLink } from "lucide-react";
import type { Worktree, LocalRepository } from "@/lib/git-api";
import type { ClaudeSessionSummary } from "@/lib/claude-api";

interface WorkspaceMobileProps {
  worktree: Worktree;
  repository: LocalRepository;
}

export function WorkspaceMobile({
  worktree,
  repository,
}: WorkspaceMobileProps) {
  const [prompt, setPrompt] = useState("");
  const [phase, setPhase] = useState<
    "input" | "todos" | "completed" | "existing"
  >("input");
  const [showNewPrompt, setShowNewPrompt] = useState(false);
  const [claudeSession, setClaudeSession] =
    useState<ClaudeSessionSummary | null>(null);
  const [latestMessage, setLatestMessage] = useState<string>("");
  const [loading, setLoading] = useState(true);
<<<<<<< HEAD
=======
  const [prDialogOpen, setPrDialogOpen] = useState(false);
>>>>>>> 4c50d7a (Add mobile workspace components and Claude API integration)
  const wsRef = useRef<WebSocket | null>(null);

  const { getAllWorktreeSessionSummaries, getWorktreeLatestAssistantMessage } =
    useClaudeApi();
  const currentWorktree = useAppStore((state) =>
    state.worktrees.get(worktree.id),
  );

  // Load Claude session data on mount
  useEffect(() => {
    const loadClaudeData = async () => {
      try {
        const sessions = await getAllWorktreeSessionSummaries();
        const sessionData = sessions[worktree.path];

        if (sessionData && sessionData.turnCount > 0) {
          setClaudeSession(sessionData);

          // Get the latest assistant message
          try {
            const message = await getWorktreeLatestAssistantMessage(
              worktree.path,
            );
            setLatestMessage(message);
            setPhase("existing");
          } catch (error) {
            console.warn("Failed to get latest message:", error);
            setPhase("input");
          }
        } else {
          setPhase("input");
        }
      } catch (error) {
        console.error("Failed to load Claude data:", error);
        setPhase("input");
      } finally {
        setLoading(false);
      }
    };

    void loadClaudeData();
  }, [worktree.path]);

  useEffect(() => {
    if (!currentWorktree) return;
    if (phase === "input") {
      if (currentWorktree.claude_activity_state === "active") {
        setPhase("todos");
      } else if (currentWorktree.claude_activity_state === "running") {
        setPhase("completed");
        // Load latest message for completed phase
        void (async () => {
          try {
            const message = await getWorktreeLatestAssistantMessage(
              worktree.path,
            );
            setLatestMessage(message);
          } catch (error) {
            console.warn("Failed to get latest message:", error);
          }
        })();
      }
    } else if (
      phase === "todos" &&
      currentWorktree.claude_activity_state === "running"
    ) {
      setPhase("completed");
      // Load latest message for completed phase
      void (async () => {
        try {
          const message = await getWorktreeLatestAssistantMessage(
            worktree.path,
          );
          setLatestMessage(message);
        } catch (error) {
          console.warn("Failed to get latest message:", error);
        }
      })();
    }
  }, [currentWorktree?.claude_activity_state, phase, worktree.path]);

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

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-center space-y-2">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mx-auto" />
          <div className="text-sm text-muted-foreground">
            Loading session...
          </div>
        </div>
      </div>
    );
  }

  if (phase === "existing" && claudeSession && latestMessage) {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const workspace = parts[1] || parts[0];
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;

    return (
      <div className="min-h-screen bg-background flex flex-col">
        <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
          <div className="p-4 flex items-center gap-3">
            <Button asChild variant="ghost" size="sm" className="p-2">
              <Link to="/workspace">
                <span className="text-lg font-bold">‹</span>
              </Link>
            </Button>
            <div className="flex-1">
              <h1 className="text-lg font-semibold">
                {repoName}/{workspace}
              </h1>
              <p className="text-sm text-muted-foreground">{cleanBranch}</p>
            </div>
          </div>
        </div>

        <div className="flex-1 flex flex-col">
          <div className="flex-1 p-4 overflow-y-auto">
            <TextContent content={latestMessage} />
          </div>

          <div className="border-t px-4 pb-20">
            <DiffViewer
              worktreeId={worktree.id}
              isOpen={true}
              onClose={() => {}}
            />
          </div>
        </div>

        <div className="fixed bottom-0 left-0 right-0 bg-background border-t p-4">
          {showNewPrompt ? (
            <div className="space-y-4">
              <Textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="Describe what you'd like to change..."
                className="min-h-[120px]"
              />
              <div className="flex gap-2">
                <Button
                  onClick={startSession}
                  disabled={!prompt.trim()}
                  className="flex-1"
                >
                  Send
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setShowNewPrompt(false);
                    setPrompt("");
                  }}
                  className="flex-1"
                >
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <Button onClick={() => setShowNewPrompt(true)} className="w-full">
              Ask for changes
            </Button>
          )}
        </div>
      </div>
    );
  }

  if (phase === "input") {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const workspace = parts[1] || parts[0];
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;

    return (
      <div className="min-h-screen bg-background flex flex-col">
        <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
          <div className="p-4 flex items-center gap-3">
            <Button asChild variant="ghost" size="sm" className="p-2">
              <Link to="/workspace">
                <span className="text-lg font-bold">‹</span>
              </Link>
            </Button>
            <div className="flex-1">
              <h1 className="text-lg font-semibold">
                {repoName}/{workspace}
              </h1>
              <p className="text-sm text-muted-foreground">{cleanBranch}</p>
            </div>
          </div>
        </div>

        <div className="flex-1 p-4 space-y-4">
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
      </div>
    );
  }

  if (phase === "existing" && claudeSession) {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const workspace = parts[1] || parts[0];
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;

    return (
      <>
        <div className="min-h-screen bg-background flex flex-col">
          <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
            <div className="p-4 flex items-center gap-3">
              <Button asChild variant="ghost" size="sm" className="p-2">
                <Link to="/workspace">
                  <span className="text-lg font-bold">‹</span>
                </Link>
              </Button>
              <div className="flex-1">
                <h1 className="text-lg font-semibold">
                  {repoName}/{workspace}
                </h1>
                <p className="text-sm text-muted-foreground">{cleanBranch}</p>
              </div>
            </div>
          </div>

          <div className="flex-1 flex flex-col">
            <div className="flex-1 p-4 overflow-y-auto">
              <TextContent
                content={
                  latestMessage ||
                  claudeSession.header ||
                  "No session content available."
                }
              />
            </div>

            <div className="border-t px-4 pb-20">
              <DiffViewer
                worktreeId={worktree.id}
                isOpen={true}
                onClose={() => {}}
              />
            </div>
          </div>

          <div className="fixed bottom-0 left-0 right-0 bg-background border-t p-4">
            {showNewPrompt ? (
              <div className="space-y-4">
                <Textarea
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  placeholder="Describe what you'd like to change..."
                  className="min-h-[120px]"
                />
                <div className="flex gap-2">
                  <Button
                    onClick={startSession}
                    disabled={!prompt.trim()}
                    className="flex-1"
                  >
                    Send
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setShowNewPrompt(false);
                      setPrompt("");
                    }}
                    className="flex-1"
                  >
                    Cancel
                  </Button>
                </div>
              </div>
            ) : (
              <div className="flex gap-2">
                <Button
                  onClick={() => setShowNewPrompt(true)}
                  className="flex-1"
                >
                  Ask for changes
                </Button>
                <Button
                  onClick={() => {
                    console.log("PR button clicked, opening dialog...");
                    setPrDialogOpen(true);
                  }}
                  variant="outline"
                  className="flex-1"
                  disabled={
                    !worktree.commit_count || worktree.commit_count === 0
                  }
                  title={
                    !worktree.commit_count || worktree.commit_count === 0
                      ? "No commits in this worktree"
                      : worktree.pull_request_url
                        ? "View existing pull request"
                        : "Create new pull request"
                  }
                >
                  {worktree.pull_request_url ? (
                    <>
                      <ExternalLink className="h-4 w-4 mr-2" />
                      View PR
                    </>
                  ) : (
                    <>
                      <GitMerge className="h-4 w-4 mr-2" />
                      Create PR
                    </>
                  )}
                </Button>
              </div>
            )}
          </div>
        </div>

        {/* Pull Request Dialog */}
        <PullRequestDialog
          open={prDialogOpen}
          onOpenChange={setPrDialogOpen}
          worktree={worktree}
          repository={repository}
          prStatus={undefined}
          summary={undefined}
          onRefreshPrStatuses={async () => {
            console.log("Refreshing PR statuses...");
          }}
        />
      </>
    );
  }

  if (phase === "input") {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const workspace = parts[1] || parts[0];
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;

    return (
      <>
        <div className="min-h-screen bg-background flex flex-col">
          <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
            <div className="p-4 flex items-center gap-3">
              <Button asChild variant="ghost" size="sm" className="p-2">
                <Link to="/workspace">
                  <span className="text-lg font-bold">‹</span>
                </Link>
              </Button>
              <div className="flex-1">
                <h1 className="text-lg font-semibold">
                  {repoName}/{workspace}
                </h1>
                <p className="text-sm text-muted-foreground">{cleanBranch}</p>
              </div>
            </div>
          </div>

          <div className="flex-1 p-4 space-y-4">
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
        </div>

        {/* Pull Request Dialog */}
        <PullRequestDialog
          open={prDialogOpen}
          onOpenChange={setPrDialogOpen}
          worktree={worktree}
          repository={repository}
          prStatus={undefined}
          summary={undefined}
          onRefreshPrStatuses={async () => {
            console.log("Refreshing PR statuses...");
          }}
        />
      </>
    );
  }

  if (phase === "todos") {
    return (
      <>
        <div className="p-4 space-y-4">
          <div className="text-sm text-muted-foreground">
            Claude is working...
          </div>
          <TodoDisplay todos={currentWorktree?.todos || []} />
        </div>

        {/* Pull Request Dialog */}
        <PullRequestDialog
          open={prDialogOpen}
          onOpenChange={setPrDialogOpen}
          worktree={worktree}
          repository={repository}
          prStatus={undefined}
          summary={undefined}
          onRefreshPrStatuses={async () => {
            console.log("Refreshing PR statuses...");
          }}
        />
      </>
    );
  }

  if (phase === "completed") {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const workspace = parts[1] || parts[0];
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;

    return (
      <>
        <div className="min-h-screen bg-background flex flex-col">
          <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
            <div className="p-4 flex items-center gap-3">
              <Button asChild variant="ghost" size="sm" className="p-2">
                <Link to="/workspace">
                  <span className="text-lg font-bold">‹</span>
                </Link>
              </Button>
              <div className="flex-1">
                <h1 className="text-lg font-semibold">
                  {repoName}/{workspace}
                </h1>
                <p className="text-sm text-muted-foreground">{cleanBranch}</p>
              </div>
            </div>
          </div>

          <div className="flex-1 flex flex-col">
            <div className="flex-1 p-4 overflow-y-auto">
              <TextContent
                content={latestMessage || "Session completed successfully"}
              />
            </div>

            <div className="border-t px-4 pb-20">
              <DiffViewer
                worktreeId={worktree.id}
                isOpen={true}
                onClose={() => {}}
              />
            </div>
          </div>

          <div className="fixed bottom-0 left-0 right-0 bg-background border-t p-4">
            {showNewPrompt ? (
              <div className="space-y-4">
                <Textarea
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  placeholder="Describe what you'd like to change..."
                  className="min-h-[120px]"
                />
                <div className="flex gap-2">
                  <Button
                    onClick={startSession}
                    disabled={!prompt.trim()}
                    className="flex-1"
                  >
                    Send
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setShowNewPrompt(false);
                      setPrompt("");
                    }}
                    className="flex-1"
                  >
                    Cancel
                  </Button>
                </div>
              </div>
            ) : (
              <div className="flex gap-2">
                <Button
                  onClick={() => setShowNewPrompt(true)}
                  className="flex-1"
                >
                  Ask for changes
                </Button>
                <Button
                  onClick={() => {
                    console.log("PR button clicked, opening dialog...");
                    setPrDialogOpen(true);
                  }}
                  variant="outline"
                  className="flex-1"
                  disabled={
                    !worktree.commit_count || worktree.commit_count === 0
                  }
                  title={
                    !worktree.commit_count || worktree.commit_count === 0
                      ? "No commits in this worktree"
                      : worktree.pull_request_url
                        ? "View existing pull request"
                        : "Create new pull request"
                  }
                >
                  {worktree.pull_request_url ? (
                    <>
                      <ExternalLink className="h-4 w-4 mr-2" />
                      View PR
                    </>
                  ) : (
                    <>
                      <GitMerge className="h-4 w-4 mr-2" />
                      Create PR
                    </>
                  )}
                </Button>
              </div>
            )}
          </div>
        </div>

        {/* Pull Request Dialog */}
        <PullRequestDialog
          open={prDialogOpen}
          onOpenChange={setPrDialogOpen}
          worktree={worktree}
          repository={repository}
          prStatus={undefined}
          summary={undefined}
          onRefreshPrStatuses={async () => {
            console.log("Refreshing PR statuses...");
          }}
        />
      </>
    );
  }

  if (phase === "completed") {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const workspace = parts[1] || parts[0];
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;

    return (
      <div className="min-h-screen bg-background flex flex-col">
        <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
          <div className="p-4 flex items-center gap-3">
            <Button asChild variant="ghost" size="sm" className="p-2">
              <Link to="/workspace">
                <span className="text-lg font-bold">‹</span>
              </Link>
            </Button>
            <div className="flex-1">
              <h1 className="text-lg font-semibold">
                {repoName}/{workspace}
              </h1>
              <p className="text-sm text-muted-foreground">{cleanBranch}</p>
            </div>
          </div>
        </div>

        <div className="flex-1 flex flex-col">
          <div className="flex-1 p-4 overflow-y-auto">
            <TextContent
              content={latestMessage || "Session completed successfully"}
            />
          </div>

          <div className="border-t px-4 pb-20">
            <DiffViewer
              worktreeId={worktree.id}
              isOpen={true}
              onClose={() => {}}
            />
          </div>
        </div>

        <div className="fixed bottom-0 left-0 right-0 bg-background border-t p-4">
          {showNewPrompt ? (
            <div className="space-y-4">
              <Textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="Describe what you'd like to change..."
                className="min-h-[120px]"
              />
              <div className="flex gap-2">
                <Button
                  onClick={startSession}
                  disabled={!prompt.trim()}
                  className="flex-1"
                >
                  Send
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setShowNewPrompt(false);
                    setPrompt("");
                  }}
                  className="flex-1"
                >
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <Button onClick={() => setShowNewPrompt(true)} className="w-full">
              Ask for changes
            </Button>
          )}
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="p-4 space-y-4">
        <div className="text-center text-muted-foreground">
          Unknown phase: {phase}
        </div>
      </div>

      {/* Pull Request Dialog */}
      <PullRequestDialog
        open={prDialogOpen}
        onOpenChange={setPrDialogOpen}
        worktree={worktree}
        repository={repository}
        prStatus={undefined}
        summary={undefined}
        onRefreshPrStatuses={async () => {
          console.log("Refreshing PR statuses...");
        }}
      />
    </>
  );
}
