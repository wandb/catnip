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
import { gitApi } from "@/lib/git-api";
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
    } else if (
      jsonContent.startsWith("```json") &&
      jsonContent.endsWith("```")
    ) {
      // Handle case without newlines
      jsonContent = jsonContent.slice(7, -3).trim(); // Remove ```json from start and ``` from end
    }

    const parsed = JSON.parse(jsonContent);

    // Check if it has the required fields
    if (
      parsed &&
      typeof parsed.title === "string" &&
      typeof parsed.description === "string"
    ) {
      return {
        title: parsed.title,
        description: parsed.description,
      };
    }
  } catch (_error) {
    // Only log if it looked like it might be JSON
    if (trimmed.includes("{")) {
      // Silent fail for non-JSON content
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

function ClaudeErrorDisplay({ errors }: { errors: string[] }) {
  return (
    <div className="mb-6 p-4 border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-950/20 rounded-lg">
      <div className="flex items-start gap-3">
        <div className="flex-shrink-0 w-5 h-5 mt-0.5">
          <svg
            className="w-5 h-5 text-red-500"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fillRule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
              clipRule="evenodd"
            />
          </svg>
        </div>
        <div className="flex-1">
          <h3 className="text-sm font-medium text-red-800 dark:text-red-200 mb-2">
            Claude encountered{" "}
            {errors.length === 1 ? "an error" : "some errors"}
          </h3>
          <div className="space-y-2">
            {errors.map((error, index) => (
              <div
                key={index}
                className="text-sm text-red-700 dark:text-red-300 font-mono bg-red-100 dark:bg-red-900/20 p-2 rounded border"
              >
                {error}
              </div>
            ))}
          </div>
        </div>
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
    "input" | "todos" | "completed" | "existing" | "error"
  >("input");
  const [showNewPrompt, setShowNewPrompt] = useState(false);
  const [claudeSession, setClaudeSession] =
    useState<ClaudeSessionSummary | null>(null);
  const [latestMessage, setLatestMessage] = useState<string>("");
  const [errorContent, setErrorContent] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [sessionStarting, setSessionStarting] = useState(false);
  const [hasBeenActive, setHasBeenActive] = useState(false);
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [contentKey, setContentKey] = useState(0); // Force content refresh
  const [hasStartedFromInitialPrompt, setHasStartedFromInitialPrompt] =
    useState(false);
  const [sessionRestarted, setSessionRestarted] = useState(false);
  const [claudeErrors, setClaudeErrors] = useState<string[]>([]);
  const restartedContentRef = useRef<{
    latestMessage?: string;
    todos?: any[];
  }>({});
  const initialPromptRef = useRef<string | undefined>(initialPrompt);

  const { getAllWorktreeSessionSummaries, getWorktreeLatestMessageOrError } =
    useClaudeApi();
  const currentWorktree = useAppStore((state) =>
    state.worktrees.get(worktree.id),
  );

  const startSession = async (promptToSendArg?: string) => {
    // Use the provided prompt or fall back to the state prompt
    const actualPrompt = promptToSendArg || prompt;
    const wasFromInitialPrompt = Boolean(promptToSendArg && initialPrompt);

    // Track if we're starting from an initial prompt
    if (wasFromInitialPrompt) {
      setHasStartedFromInitialPrompt(true);
    }

    setSessionStarting(true);
    setClaudeErrors([]); // Clear any previous errors

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

    setPhase("todos");

    // Send prompt to PTY using new API
    try {
      await gitApi.sendPromptToPTY(worktree.name, actualPrompt, "claude");
    } catch (error) {
      console.error("Failed to send prompt to PTY:", error);
      if (error instanceof Error && error.message === "PTY_TIMEOUT") {
        setClaudeErrors([
          "PTY is not ready yet. Please wait a moment and try again.",
        ]);
        setPhase("error");
      } else {
        setClaudeErrors(["Failed to send prompt to Claude. Please try again."]);
        setPhase("error");
      }
    } finally {
      setSessionStarting(false);
    }
  };

  // Pre-compute content for all phases to avoid conditional hooks
  const existingContent = useMemo(() => {
    const content =
      latestMessage || claudeSession?.header || "No session content available.";

    // Don't try to parse error messages as PR summaries
    const isErrorContent =
      content.includes("API Error:") ||
      content.includes("401") ||
      content.includes("Unauthorized");
    const prSummary = isErrorContent ? null : parsePRSummary(content);

    if (prSummary) {
      return <PRSummaryDisplay summary={prSummary} />;
    }

    return <TextContent content={content} />;
  }, [latestMessage, claudeSession, contentKey]);

  const completedContent = useMemo(() => {
    const content = latestMessage || "Session completed successfully";

    // Don't try to parse error messages as PR summaries
    const isErrorContent =
      content.includes("API Error:") ||
      content.includes("401") ||
      content.includes("Unauthorized");
    const prSummary = isErrorContent ? null : parsePRSummary(content);

    if (prSummary) {
      return <PRSummaryDisplay summary={prSummary} />;
    }

    return <TextContent content={content} />;
  }, [latestMessage, contentKey]);

  // Start PTY proactively when component mounts
  useEffect(() => {
    const startPTY = async () => {
      try {
        await gitApi.startPTY(worktree.name, "claude");
        console.log("✅ Started PTY for workspace:", worktree.name);
      } catch (error) {
        console.warn("⚠️ Failed to start PTY:", error);
        // Non-fatal - PTY will be created on-demand if needed
      }
    };

    void startPTY();
  }, [worktree.name]);

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
            const messageResult = await getWorktreeLatestMessageOrError(
              worktree.path,
            );
            if (messageResult.isError) {
              setLatestMessage(messageResult.content);
              setErrorContent(messageResult.content);
              setPhase("error");
            } else {
              setLatestMessage(messageResult.content);
              setPhase("existing");
            }
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
              void startSession(trimmedPrompt);
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
            void startSession(trimmedPrompt);
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

    // Track when Claude becomes active
    if (currentWorktree.claude_activity_state === "active") {
      if (!hasBeenActive) {
        setHasBeenActive(true);
      }
    }

    // Handle transitions based on Claude activity state
    // Don't transition away from error phase or into completed phase if there are Claude errors
    const hasClaudeErrors = claudeErrors && claudeErrors.length > 0;

    if (
      phase === "existing" &&
      currentWorktree.claude_activity_state === "active" &&
      !hasClaudeErrors
    ) {
      setPhase("todos");
    } else if (
      phase === "todos" &&
      currentWorktree.claude_activity_state === "active" &&
      !hasClaudeErrors
    ) {
      // Already in todos and Claude is active - stay in todos
    } else if (
      (phase === "todos" || phase === "existing") &&
      (currentWorktree.claude_activity_state === "running" ||
        currentWorktree.claude_activity_state === "inactive") &&
      hasBeenActive &&
      !hasClaudeErrors
    ) {
      // Only transition to completed if Claude has actually been active before AND there are no errors

      // Check for errors BEFORE setting phase to completed
      void (async () => {
        try {
          const messageResult = await getWorktreeLatestMessageOrError(
            worktree.path,
          );

          if (messageResult.isError) {
            // Store error content
            setErrorContent(messageResult.content);
            setPhase("error");
          } else {
            setLatestMessage(messageResult.content);
            setPhase("completed");
            // Clear restart flag and prompt when completing
            setSessionRestarted(false);
            setShowNewPrompt(false);
            setPrompt("");
          }
          setContentKey((prev) => prev + 1);
        } catch (error) {
          console.warn(
            "Failed to get latest message, defaulting to completed phase:",
            error,
          );
          // Set a fallback message when we can't retrieve the actual message
          setLatestMessage("Session completed successfully");
          setPhase("completed");
          setSessionRestarted(false);
          setShowNewPrompt(false);
          setPrompt("");
          setContentKey((prev) => prev + 1);
        }
      })();
    } else if (hasClaudeErrors) {
      console.log(
        "Claude errors present, staying in or switching to error phase",
      );
      // The error useEffect will handle the transition to error phase
    } else if (
      phase === "todos" &&
      currentWorktree.claude_activity_state === "running" &&
      hasBeenActive &&
      !hasClaudeErrors
    ) {
      // Transition from active to running - refresh content and switch to existing/completed
      void (async () => {
        try {
          const messageResult = await getWorktreeLatestMessageOrError(
            worktree.path,
          );
          if (messageResult.isError) {
            setLatestMessage(messageResult.content);
            setPhase("error");
          } else {
            setLatestMessage(messageResult.content);
          }
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
      // Phase transition logic placeholder
    }
  }, [
    currentWorktree?.claude_activity_state,
    phase,
    worktree.path,
    getWorktreeLatestMessageOrError,
    getAllWorktreeSessionSummaries,
    hasBeenActive,
    claudeErrors,
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
      // Short delay and then start
      const timer = setTimeout(() => {
        setPhase("todos");
        void startSession(prompt); // Pass the prompt directly
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
        setSessionRestarted(false);
      }
    }
  }, [sessionRestarted, currentWorktree?.latest_claude_message]);

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

  if (phase === "error") {
    // Extract workspace name and use fallback for repo name
    const parts = worktree.name.split("/");
    const repoName = repository.name || parts[0] || "Unknown";
    const cleanBranch = worktree.branch.startsWith("/")
      ? worktree.branch.slice(1)
      : worktree.branch;
    const title = getWorkspaceTitle(worktree);

    const handleDismissErrors = () => {
      setErrorContent("");
      setPhase("existing");
    };

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
            <div className="flex-1 p-4 overflow-y-auto">
              {/* Always show error message if we have it */}
              {errorContent && (
                <div className="bg-destructive/10 border border-destructive/20 rounded-lg p-4">
                  <div className="flex items-center gap-2 text-destructive mb-2">
                    <span className="text-sm font-medium">Error</span>
                  </div>
                  <pre className="text-sm whitespace-pre-wrap text-foreground/90">
                    {errorContent}
                  </pre>
                </div>
              )}

              {/* Display Claude errors */}
              {claudeErrors && claudeErrors.length > 0 && (
                <div className="mt-4">
                  <ClaudeErrorDisplay errors={claudeErrors} />
                </div>
              )}

              {/* Fallback only if we have nothing */}
              {!errorContent &&
                (!claudeErrors || claudeErrors.length === 0) && (
                  <div className="bg-destructive/10 border border-destructive/20 rounded-lg p-4">
                    <div className="flex items-center gap-2 text-destructive mb-2">
                      <span className="text-sm font-medium">Error</span>
                    </div>
                    <p className="text-sm text-foreground/90">
                      Claude encountered an error. Please try starting a new
                      session.
                    </p>
                  </div>
                )}
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
                    onClick={() => {
                      // Clear errors and restart session
                      setClaudeErrors([]);
                      void startSession();
                    }}
                    disabled={!prompt.trim()}
                    className="flex-1"
                  >
                    Try Again
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
                  Try Again
                </Button>
                <Button
                  onClick={handleDismissErrors}
                  variant="outline"
                  className="flex-1"
                >
                  Dismiss Errors
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
          onRefreshPrStatuses={async () => {}}
        />
      </>
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
          onRefreshPrStatuses={async () => {}}
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
          onRefreshPrStatuses={async () => {}}
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
          onRefreshPrStatuses={async () => {}}
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
          onRefreshPrStatuses={async () => {}}
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
