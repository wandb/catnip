import { useState, useEffect, useRef, useMemo } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { TodoDisplay } from "@/components/TodoDisplay";
import { DiffViewer } from "@/components/DiffViewer";
import { TextContent } from "@/components/TextContent";
import { PullRequestDialog } from "@/components/PullRequestDialog";
import { useAppStore } from "@/stores/appStore";
import { useClaudeApi } from "@/hooks/useClaudeApi";
import { GitMerge, ExternalLink } from "lucide-react";
import {
  getWorkspaceTitle,
  getStatusIndicatorClasses,
} from "@/lib/workspace-utils";
import type { Worktree, LocalRepository } from "@/lib/git-api";
import type { ClaudeSessionSummary } from "@/lib/claude-api";

interface PRSummary {
  title: string;
  description: string;
}

function parsePRSummary(content: string): PRSummary | null {
  // Quick check to avoid noisy logs for non-JSON content
  const trimmed = content.trim();
  if (!trimmed.includes("{") && !trimmed.startsWith("```json")) {
    return null;
  }

  try {
    let jsonContent = trimmed;

    // Check if content is wrapped in a fenced code block
    if (jsonContent.startsWith("```json\n") && jsonContent.endsWith("\n```")) {
      // Remove the fenced code block wrapper
      jsonContent = jsonContent.slice(8, -4).trim(); // Remove ```json\n from start and \n``` from end
      console.log("Extracted JSON from fenced code block");
    } else if (
      jsonContent.startsWith("```json") &&
      jsonContent.endsWith("```")
    ) {
      // Handle case without newlines
      jsonContent = jsonContent.slice(7, -3).trim(); // Remove ```json from start and ``` from end
      console.log("Extracted JSON from fenced code block (no newlines)");
    }

    const parsed = JSON.parse(jsonContent);

    // Check if it has the required fields
    if (
      parsed &&
      typeof parsed.title === "string" &&
      typeof parsed.description === "string"
    ) {
      console.log("Found valid PR summary");
      return {
        title: parsed.title,
        description: parsed.description,
      };
    }
  } catch (error) {
    // Only log if it looked like it might be JSON
    if (trimmed.includes("{")) {
      console.log("Failed to parse potential JSON content:", error);
    }
  }

  return null;
}

function PRSummaryDisplay({ summary }: { summary: PRSummary }) {
  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-xl font-semibold mb-2">{summary.title}</h2>
      </div>
      <div className="prose prose-sm max-w-none dark:prose-invert">
        <TextContent content={summary.description} />
      </div>
    </div>
  );
}

interface WorkspaceMobileProps {
  worktree: Worktree;
  repository: LocalRepository;
  initialPrompt?: string;
}

export function WorkspaceMobile({
  worktree,
  repository,
  initialPrompt,
}: WorkspaceMobileProps) {
  const navigate = useNavigate();
  const [prompt, setPrompt] = useState("");
  const [phase, setPhase] = useState<
    "input" | "todos" | "completed" | "existing"
  >("input");
  const [showNewPrompt, setShowNewPrompt] = useState(false);
  const [claudeSession, setClaudeSession] =
    useState<ClaudeSessionSummary | null>(null);
  const [latestMessage, setLatestMessage] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [sessionStarting, setSessionStarting] = useState(false);
  const [hasBeenActive, setHasBeenActive] = useState(false);
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [contentKey, setContentKey] = useState(0); // Force content refresh
  const [hasStartedFromInitialPrompt, setHasStartedFromInitialPrompt] =
    useState(false);
  const [sessionRestarted, setSessionRestarted] = useState(false);
  const restartedContentRef = useRef<{
    latestMessage?: string;
    todos?: any[];
  }>({});
  const wsRef = useRef<WebSocket | null>(null);
  const initialPromptRef = useRef<string | undefined>(initialPrompt);

  const { getAllWorktreeSessionSummaries, getWorktreeLatestAssistantMessage } =
    useClaudeApi();
  const currentWorktree = useAppStore((state) =>
    state.worktrees.get(worktree.id),
  );

  const startSession = (promptToSend?: string) => {
    // Use the provided prompt or fall back to the state prompt
    const actualPrompt = promptToSend || prompt;
    const wasFromInitialPrompt = Boolean(promptToSend && initialPrompt);

    // Track if we're starting from an initial prompt
    if (wasFromInitialPrompt) {
      setHasStartedFromInitialPrompt(true);
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const params = new URLSearchParams();
    params.set("session", worktree.name);
    params.set("agent", "claude");
    const url = `${protocol}//${window.location.host}/v1/pty?${params.toString()}`;

    console.log("Starting PTY WebSocket session:", url);
    console.log("Will send prompt:", actualPrompt);

    setSessionStarting(true);

    // Reset hasBeenActive and set phase when starting a new session from existing workspace
    // This ensures proper state transitions
    if (phase === "completed" || phase === "existing") {
      setHasBeenActive(false);
      setPhase("todos");
      // Reset the prompt UI state
      setShowNewPrompt(false);
      // Capture the current content before marking as restarted
      restartedContentRef.current = {
        latestMessage: currentWorktree?.latest_claude_message,
        todos: currentWorktree?.todos ? [...currentWorktree.todos] : undefined,
      };
      // Mark that we restarted a session (for UI styling)
      setSessionRestarted(true);
      // Clear the prompt after starting
      if (phase === "completed") {
        setPrompt("");
      }
    }

    const ws = new WebSocket(url);

    ws.onopen = () => {
      console.log("PTY WebSocket opened, waiting for Claude to initialize...");
      setSessionStarting(false);

      // Clear the prompt parameter from URL if session started from initial prompt
      if (wasFromInitialPrompt) {
        void navigate({
          to: "/workspace/$project/$workspace",
          params: {
            project: worktree.name.split("/")[0],
            workspace: worktree.name.split("/")[1],
          },
          search: { prompt: undefined },
          replace: true,
        });
      }

      // Wait for Claude to fully initialize before sending the prompt
      setTimeout(() => {
        const promptMessage = {
          type: "prompt",
          data: actualPrompt,
          submit: true,
        };
        console.log(
          "Sending prompt message to initialized Claude:",
          promptMessage,
        );
        ws.send(JSON.stringify(promptMessage));
      }, 3000); // Wait 3 seconds for Claude to fully start
    };

    ws.onerror = (error) => {
      console.error("PTY WebSocket error:", error);
      setSessionStarting(false);
    };

    ws.onclose = (event) => {
      console.log("PTY WebSocket closed:", event.code, event.reason);
      setSessionStarting(false);
    };

    wsRef.current = ws;
    setPhase("todos");
  };

  // Pre-compute content for all phases to avoid conditional hooks
  const existingContent = useMemo(() => {
    const content =
      latestMessage || claudeSession?.header || "No session content available.";
    const prSummary = parsePRSummary(content);

    if (prSummary) {
      return <PRSummaryDisplay summary={prSummary} />;
    }

    return <TextContent content={content} />;
  }, [latestMessage, claudeSession, contentKey]);

  const completedContent = useMemo(() => {
    const content = latestMessage || "Session completed successfully";
    const prSummary = parsePRSummary(content);

    if (prSummary) {
      return <PRSummaryDisplay summary={prSummary} />;
    }

    return <TextContent content={content} />;
  }, [latestMessage, contentKey]);

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
            // If we have session data but can't get the message, likely a completed session
            // Set fallback content based on the session header
            if (sessionData.header) {
              setLatestMessage(sessionData.header);
              setPhase("existing");
            } else {
              // If we have an initial prompt and failed to get message, start fresh
              if (initialPromptRef.current?.trim()) {
                setPrompt(initialPromptRef.current.trim());
                setPhase("input");
              } else {
                setPhase("input");
              }
            }
          }
        } else {
          // If there's an initial prompt, set it and start session immediately
          if (initialPromptRef.current?.trim()) {
            const trimmedPrompt = initialPromptRef.current.trim();
            setPrompt(trimmedPrompt);
            // Skip input phase and go directly to todos, then start session
            setPhase("todos");
            // Start session after a short delay to let the UI update
            setTimeout(() => {
              startSession(trimmedPrompt);
            }, 100);
          } else {
            setPhase("input");
          }
        }
      } catch (error) {
        console.error("Failed to load Claude data:", error);

        // If there's an initial prompt, still set it even if session loading failed
        if (initialPromptRef.current?.trim()) {
          const trimmedPrompt = initialPromptRef.current.trim();
          setPrompt(trimmedPrompt);
          // Skip input phase and go directly to todos, then start session
          setPhase("todos");
          // Start session after a short delay to let the UI update
          setTimeout(() => {
            startSession(trimmedPrompt);
          }, 100);
        } else {
          setPhase("input");
        }
      } finally {
        setLoading(false);
      }
    };

    void loadClaudeData();
  }, [worktree.path]); // Removed initialPrompt from dependency array

  useEffect(() => {
    if (!currentWorktree) return;

    console.log("Activity state change:", {
      phase,
      claude_activity_state: currentWorktree.claude_activity_state,
      hasBeenActive,
      willTransitionToCompleted:
        (phase === "todos" || phase === "existing") &&
        currentWorktree.claude_activity_state === "running" &&
        hasBeenActive,
    });

    // Track when Claude becomes active
    if (currentWorktree.claude_activity_state === "active") {
      if (!hasBeenActive) {
        console.log("Claude became active for the first time");
        setHasBeenActive(true);
      }
    }

    // Handle transitions based on Claude activity state
    if (
      phase === "existing" &&
      currentWorktree.claude_activity_state === "active"
    ) {
      console.log("Existing session became active, switching to todos");
      setPhase("todos");
    } else if (
      phase === "todos" &&
      currentWorktree.claude_activity_state === "active"
    ) {
      // Already in todos and Claude is active - stay in todos
      console.log("Claude is active, staying in todos phase");
    } else if (
      (phase === "todos" || phase === "existing") &&
      (currentWorktree.claude_activity_state === "running" ||
        currentWorktree.claude_activity_state === "inactive") &&
      hasBeenActive
    ) {
      // Only transition to completed if Claude has actually been active before
      console.log(
        `Session completed (was previously active, now ${currentWorktree.claude_activity_state}), switching to completed phase`,
      );
      setPhase("completed");
      // Clear restart flag and prompt when completing
      setSessionRestarted(false);
      setShowNewPrompt(false);
      setPrompt("");
      // Load latest message for completed phase
      void (async () => {
        try {
          const message = await getWorktreeLatestAssistantMessage(
            worktree.path,
          );
          setLatestMessage(message);
          setContentKey((prev) => prev + 1);
        } catch (error) {
          console.warn(
            "Failed to get latest message for completed phase:",
            error,
          );
          // Set a fallback message when we can't retrieve the actual message
          setLatestMessage("Session completed successfully");
          setContentKey((prev) => prev + 1);
        }
      })();
    } else if (
      phase === "todos" &&
      currentWorktree.claude_activity_state === "running" &&
      hasBeenActive
    ) {
      // Transition from active to running - refresh content and switch to existing/completed
      console.log(
        "Session transitioned from active to running, refreshing content",
      );
      void (async () => {
        try {
          const message = await getWorktreeLatestAssistantMessage(
            worktree.path,
          );
          setLatestMessage(message);
          setContentKey((prev) => prev + 1); // Force content refresh
          // Clear restart flag since we got new content
          setSessionRestarted(false);
          // Check if we should go to completed or existing
          const sessions = await getAllWorktreeSessionSummaries();
          const sessionData = sessions[worktree.path];
          if (sessionData && sessionData.turnCount > 0) {
            setClaudeSession(sessionData);
            setPhase("existing");
          } else {
            setPhase("completed");
            setShowNewPrompt(false);
            setPrompt("");
          }
        } catch (error) {
          console.warn("Failed to refresh content after transition:", error);
          setPhase("completed");
          setLatestMessage("Session completed successfully");
          setContentKey((prev) => prev + 1); // Force content refresh even on error
          setSessionRestarted(false);
          setShowNewPrompt(false);
          setPrompt("");
        }
      })();
    } else if (
      phase === "todos" &&
      (currentWorktree.claude_activity_state === "running" ||
        currentWorktree.claude_activity_state === "inactive") &&
      !hasBeenActive
    ) {
      console.log(
        `Session not started yet (never been active, now ${currentWorktree.claude_activity_state}), staying in todos phase`,
      );
    }
  }, [
    currentWorktree?.claude_activity_state,
    phase,
    worktree.path,
    getWorktreeLatestAssistantMessage,
    getAllWorktreeSessionSummaries,
    hasBeenActive,
  ]);

  // Simplified auto-start logic - only for input phase without initial prompt
  useEffect(() => {
    // Only auto-start if we're in input phase, have an initial prompt, but no session was found
    // This is a fallback in case the initial load didn't start the session
    if (
      phase === "input" &&
      initialPromptRef.current?.trim() &&
      prompt.trim() &&
      !loading &&
      !sessionStarting &&
      !hasStartedFromInitialPrompt // Don't auto-start again if we already started from initial prompt
    ) {
      console.log("Fallback auto-start for input phase with initial prompt");
      // Short delay and then start
      const timer = setTimeout(() => {
        setPhase("todos");
        startSession(prompt); // Pass the prompt directly
      }, 500);

      return () => clearTimeout(timer);
    }
  }, [phase, prompt, loading, sessionStarting, hasStartedFromInitialPrompt]);

  // Clear restart flag when we get genuinely new content (different from what was showing at restart)
  useEffect(() => {
    if (sessionRestarted && currentWorktree?.todos) {
      const previousTodos = restartedContentRef.current.todos;
      const currentTodos = currentWorktree.todos;

      // Compare todos - check if they're different
      const todosChanged =
        !previousTodos ||
        previousTodos.length !== currentTodos.length ||
        JSON.stringify(previousTodos) !== JSON.stringify(currentTodos);

      if (todosChanged && currentTodos.length > 0) {
        console.log(
          "Got genuinely new todos after restart, clearing restart flag",
        );
        setSessionRestarted(false);
      }
    }
  }, [sessionRestarted, currentWorktree?.todos]);

  useEffect(() => {
    if (sessionRestarted && currentWorktree?.latest_claude_message) {
      const previousMessage = restartedContentRef.current.latestMessage;
      const currentMessage = currentWorktree.latest_claude_message;

      // Check if the message is different from what was showing at restart
      if (previousMessage !== currentMessage) {
        console.log(
          "Got genuinely new latest message after restart, clearing restart flag",
        );
        setSessionRestarted(false);
      }
    }
  }, [sessionRestarted, currentWorktree?.latest_claude_message]);

  useEffect(() => {
    return () => {
      wsRef.current?.close();
    };
  }, []);

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

  if (phase === "existing" && claudeSession) {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;
    const title = getWorkspaceTitle(worktree);

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
                <h1 className="text-lg font-semibold">{title}</h1>
                <p className="text-sm text-muted-foreground">
                  {repoName} · {cleanBranch}
                </p>
              </div>
            </div>
          </div>

          <div className="flex-1 flex flex-col">
            <div className="flex-1 p-4 overflow-y-auto">{existingContent}</div>

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
                    onClick={() => startSession()}
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
                    if (worktree.pull_request_url) {
                      window.open(worktree.pull_request_url, "_blank");
                    } else {
                      console.log("PR button clicked, opening dialog...");
                      setPrDialogOpen(true);
                    }
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

  if (phase === "todos") {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;
    const title = getWorkspaceTitle(worktree);

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
                <h1 className="text-lg font-semibold">{title}</h1>
                <p className="text-sm text-muted-foreground">
                  {repoName} · {cleanBranch}
                </p>
              </div>
            </div>
          </div>

          <div className="flex-1 p-4 space-y-4">
            <div className="flex items-center gap-3">
              {sessionStarting ? (
                <>
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary" />
                  <div className="text-sm text-muted-foreground">
                    Starting Claude session...
                  </div>
                </>
              ) : (
                <>
                  <div className={getStatusIndicatorClasses(currentWorktree)} />
                  <div className="text-sm text-muted-foreground">
                    Claude is working on your request...
                  </div>
                </>
              )}
            </div>

            {/* Dynamic session context - starts with prompt, evolves to show latest message */}
            <div
              className={`bg-primary/5 border border-primary/20 rounded-lg p-4 transition-opacity ${sessionRestarted ? "opacity-50" : "opacity-100"}`}
            >
              <div className="text-xs font-medium text-primary/80 mb-2">
                Session Context:
              </div>
              <div className="text-sm leading-relaxed">
                {currentWorktree?.latest_claude_message ? (
                  <div className="prose prose-sm max-w-none dark:prose-invert">
                    <TextContent
                      content={currentWorktree.latest_claude_message}
                    />
                  </div>
                ) : prompt || claudeSession?.header ? (
                  prompt || claudeSession?.header
                ) : (
                  "Starting session..."
                )}
              </div>
            </div>

            {/* Show todos if available, otherwise show generic thinking message */}
            {currentWorktree?.todos && currentWorktree.todos.length > 0 ? (
              <div
                className={`space-y-2 transition-opacity ${sessionRestarted ? "opacity-50" : "opacity-100"}`}
              >
                <div className="text-sm font-medium">Progress:</div>
                <TodoDisplay todos={currentWorktree.todos} />
              </div>
            ) : (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-muted-foreground/50" />
                <span>Claude is thinking...</span>
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
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;
    const title = getWorkspaceTitle(worktree);

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
                <h1 className="text-lg font-semibold">{title}</h1>
                <p className="text-sm text-muted-foreground">
                  {repoName} · {cleanBranch}
                </p>
              </div>
            </div>
          </div>

          <div className="flex-1 flex flex-col">
            <div className="flex-1 p-4 overflow-y-auto">{completedContent}</div>

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
                    onClick={() => startSession()}
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
                    if (worktree.pull_request_url) {
                      window.open(worktree.pull_request_url, "_blank");
                    } else {
                      console.log("PR button clicked, opening dialog...");
                      setPrDialogOpen(true);
                    }
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

  // Input phase - for new prompts or when no existing session
  if (phase === "input") {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;
    const title = getWorkspaceTitle(worktree);

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
                <h1 className="text-lg font-semibold">{title}</h1>
                <p className="text-sm text-muted-foreground">
                  {repoName} · {cleanBranch}
                </p>
              </div>
            </div>
          </div>

          <div className="flex-1 p-4 space-y-4">
            <div className="text-center space-y-2">
              <h2 className="text-xl font-semibold">Start Working</h2>
              <p className="text-sm text-muted-foreground">
                Describe what you'd like to work on
              </p>
            </div>
            <Textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="Describe your task..."
              className="min-h-[120px]"
              autoFocus={!!initialPrompt}
            />
          </div>

          <div className="fixed bottom-0 left-0 right-0 bg-background border-t p-4">
            <Button
              onClick={() => startSession()}
              disabled={!prompt.trim()}
              className="w-full"
            >
              Start Working
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
