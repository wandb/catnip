import { useEffect, useRef, useCallback, useState } from "react";
import { useXTerm } from "react-xtermjs";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { useWebSocket as useWebSocketContext } from "@/lib/hooks";
import { FileDropAddon } from "@/lib/file-drop-addon";
import { ErrorDisplay } from "@/components/ErrorDisplay";
import { WorkspaceTerminal } from "@/components/WorkspaceTerminal";
import { WorkspaceDiffViewer } from "@/components/WorkspaceDiffViewer";
import { PortPreview } from "@/components/PortPreview";
import { Button } from "@/components/ui/button";
import { SidebarTrigger } from "@/components/ui/sidebar";
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from "@/components/ui/resizable";
import { useSidebar } from "@/hooks/use-sidebar";
import { useWorktreeDiff } from "@/hooks/useWorktreeDiff";
import { Eye, ChevronDown, ChevronUp, Plus, X } from "lucide-react";
import type { Worktree, LocalRepository } from "@/lib/git-api";

interface TerminalTab {
  id: string;
  name: string;
}

interface WorkspaceMainContentProps {
  worktree: Worktree;
  repository: LocalRepository;
  showDiffView: boolean;
  setShowDiffView: (showDiff: boolean) => void;
  showPortPreview: number | null;
  setShowPortPreview: (port: number | null) => void;
  selectedFile?: string;
  setSelectedFile?: (file: string | undefined) => void;
}

function ClaudeTerminal({ worktree }: { worktree: Worktree }) {
  const { instance, ref } = useXTerm();
  const { setIsConnected } = useWebSocketContext();
  const wsRef = useRef<WebSocket | null>(null);
  const wsReady = useRef(false);
  const terminalReady = useRef(false);
  const bufferingRef = useRef(false);
  const isSetup = useRef(false);
  const lastConnectionAttempt = useRef(0);
  const fitAddon = useRef<FitAddon | null>(null);
  const webLinksAddon = useRef<WebLinksAddon | null>(null);
  const renderAddon = useRef<WebglAddon | null>(null);
  const resizeTimeout = useRef<number | null>(null);
  const observerRef = useRef<ResizeObserver | null>(null);
  const [dims, setDims] = useState<{ cols: number; rows: number } | null>(null);
  const [isReadOnly, setIsReadOnly] = useState(false);
  const [shakeReadOnlyBadge, setShakeReadOnlyBadge] = useState(false);
  const [error, setError] = useState<{ title: string; message: string } | null>(
    null,
  );

  // Trigger shake animation for read-only badge
  const triggerReadOnlyShake = useCallback(() => {
    if (isReadOnly) {
      setShakeReadOnlyBadge(true);
      setTimeout(() => setShakeReadOnlyBadge(false), 600);
    }
  }, [isReadOnly]);

  // Handle read-only badge click to request promotion
  const handlePromoteRequest = useCallback(() => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "promote" }));
    }
  }, []);

  // Scroll terminal to bottom
  const scrollToBottom = useCallback(() => {
    if (instance) {
      instance.scrollToBottom();
    }
  }, [instance]);

  // Send ready signal when both WebSocket and terminal are ready
  const sendReadySignal = useCallback(() => {
    if (!wsReady.current || !wsRef.current || !fitAddon.current) {
      return;
    }
    wsRef.current.send(JSON.stringify({ type: "ready" }));
  }, []);

  const fontSize = useCallback((element: Element) => {
    if (element.clientWidth < 400) {
      return 6;
    } else if (element.clientWidth < 600 || element.clientHeight < 400) {
      return 10;
    } else {
      return 14;
    }
  }, []);

  // Track if this is the very first connection to the worktree
  const isFirstConnection = useRef(true);

  // Reset state when worktree changes
  useEffect(() => {
    isSetup.current = false;
    wsReady.current = false;
    terminalReady.current = false;
    bufferingRef.current = false;
    lastConnectionAttempt.current = 0;
    isFirstConnection.current = true; // Reset first connection flag when worktree changes
    setError(null);

    // Close existing WebSocket if any
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
  }, [worktree.id]);

  useEffect(() => {
    if (wsReady.current && dims) {
      wsRef.current?.send(JSON.stringify({ type: "resize", ...dims }));
    }
  }, [dims, wsReady.current]);

  // Update terminal cursor when read-only state changes
  useEffect(() => {
    if (instance && instance.options) {
      instance.options.cursorBlink = !isReadOnly;
      instance.options.theme = {
        ...instance.options.theme,
        cursor: isReadOnly ? "transparent" : "#0a0a0a",
        cursorAccent: isReadOnly ? "transparent" : "#0a0a0a",
      };
    }
  }, [isReadOnly, instance]);

  // Set up terminal when instance and ref become available
  useEffect(() => {
    if (!instance || !ref.current) {
      return;
    }

    // Only set up once per session
    if (isSetup.current) {
      return;
    }

    // Rate limit reconnections to once per second maximum
    const now = Date.now();
    if (now - lastConnectionAttempt.current < 1000) {
      console.log(
        "[Claude Terminal] Rate limiting connection attempt, too soon",
      );
      return;
    }
    lastConnectionAttempt.current = now;

    isSetup.current = true;

    // For first connections, clear terminal to ensure clean state
    // For reconnections, don't clear - we want to preserve state until we get fresh data
    if (isFirstConnection.current) {
      instance.clear();
      isFirstConnection.current = false;
    }

    // Set up WebSocket connection for Claude agent in the workspace directory
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const urlParams = new URLSearchParams();
    urlParams.set("session", worktree.name);
    urlParams.set("agent", "claude");

    const socketUrl = `${protocol}//${window.location.host}/v1/pty?${urlParams.toString()}`;

    const ws = new WebSocket(socketUrl);
    wsRef.current = ws;
    const buffer: Uint8Array[] = [];

    ws.onopen = () => {
      setIsConnected(true);
      wsReady.current = true;
      sendReadySignal();
    };

    ws.onclose = () => {
      setIsConnected(false);
    };

    ws.onerror = (error) => {
      console.error("❌ Claude WebSocket error:", error);
      setIsConnected(false);
    };

    ws.onmessage = async (event) => {
      let data: string | Uint8Array | undefined;

      // Handle both binary and text data
      if (event.data instanceof ArrayBuffer) {
        if (bufferingRef.current) {
          buffer.push(new Uint8Array(event.data));
          return;
        } else {
          data = new Uint8Array(event.data);
        }
      } else if (typeof event.data === "string") {
        // Try to parse as JSON for control messages
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === "buffer-size") {
            if (instance.cols !== msg.cols || instance.rows !== msg.rows) {
              // Don't clear terminal on buffer-size - let the buffer replay handle it
              // Just resize to match the buffered dimensions
              instance.resize(msg.cols, msg.rows);
              // Force synchronous refresh after resize
              instance.refresh(0, msg.rows - 1);
            }
            bufferingRef.current = true;
            return;
          } else if (msg.type === "buffer-complete") {
            terminalReady.current = true;

            // Process any remaining buffered data first
            if (buffer.length > 0) {
              // Always clear terminal before replaying buffer to prevent corruption
              instance.clear();
              for (const chunk of buffer) {
                instance.write(chunk);
              }
              buffer.length = 0;
            } else {
              // For Claude sessions with no buffer data, don't clear - keep existing content
            }

            // Reset buffering flag so new data can be written
            bufferingRef.current = false;

            // Add a small delay before fitting to ensure content is rendered
            setTimeout(() => {
              requestAnimationFrame(() => {
                if (fitAddon.current) {
                  fitAddon.current.fit();
                  scrollToBottom();
                  // Force a full refresh after fit to fix any rendering issues
                  instance.refresh(0, instance.rows - 1);
                }
                // Dimensions will be sent by the resize listener when fit() completes
              });
            }, 50);
            return;
          } else if (msg.type === "read-only") {
            setIsReadOnly(msg.data === true);
            return;
          } else if (msg.type === "error") {
            setError({
              title: msg.error || "Error",
              message: msg.message || "An unexpected error occurred",
            });
            return;
          }
        } catch (_e) {
          // Not JSON, treat as regular text
        }
        data = event.data;
      } else if (event.data instanceof Blob) {
        const arrayBuffer = await event.data.arrayBuffer();
        if (bufferingRef.current) {
          buffer.push(new Uint8Array(arrayBuffer));
          bufferingRef.current = false;
        } else {
          data = new Uint8Array(arrayBuffer);
        }
      } else {
        return;
      }

      if (!bufferingRef.current && buffer.length > 0) {
        // Buffer replay already handled in buffer-complete
        buffer.length = 0;
      }
      if (data && !bufferingRef.current) {
        instance.write(data);
      }
    };

    // Configure terminal options
    if (instance.options) {
      instance.options.fontFamily =
        '"FiraCode Nerd Font Mono", "JetBrains Mono", "Monaco", "monospace"';
      instance.options.fontSize = fontSize(ref.current);
      instance.options.theme = {
        background: "#0a0a0a",
        foreground: "#e2e8f0",
        cursor: isReadOnly ? "transparent" : "#0a0a0a",
        cursorAccent: isReadOnly ? "transparent" : "#0a0a0a",
        selectionBackground: "#333333",
        black: "#0a0a0a",
        red: "#fc8181",
        green: "#68d391",
        yellow: "#f6e05e",
        blue: "#63b3ed",
        magenta: "#d6bcfa",
        cyan: "#4fd1c7",
        white: "#e2e8f0",
        brightBlack: "#4a5568",
        brightRed: "#fc8181",
        brightGreen: "#68d391",
        brightYellow: "#f6e05e",
        brightBlue: "#63b3ed",
        brightMagenta: "#d6bcfa",
        brightCyan: "#4fd1c7",
        brightWhite: "#f7fafc",
      };
      instance.options.cursorBlink = !isReadOnly;
      instance.options.scrollback = 10000;
      instance.options.allowProposedApi = true;
      instance.options.drawBoldTextInBrightColors = false;
      instance.options.fontWeight = "normal";
      instance.options.fontWeightBold = "bold";
      instance.options.letterSpacing = 0;
      instance.options.lineHeight = 1.0;
    }

    // Create addons
    fitAddon.current = new FitAddon();
    webLinksAddon.current = new WebLinksAddon();

    // Load addons
    instance.loadAddon(fitAddon.current);
    instance.loadAddon(webLinksAddon.current);

    try {
      renderAddon.current = new WebglAddon();
      instance.loadAddon(renderAddon.current);
    } catch (error) {
      console.warn("Render addon failed to load:", error);
    }

    // Set up FileDropAddon
    const sendData = (data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    };
    const fileDropAddon = new FileDropAddon(sendData);
    instance.loadAddon(fileDropAddon);

    instance.onResize((event) => {
      setDims({ cols: event.cols, rows: event.rows });
    });

    // Open terminal in DOM element
    instance.open(ref.current);

    // Delay initial fit to allow layout to settle
    const initialFitTimeout = setTimeout(() => {
      requestAnimationFrame(() => {
        if (fitAddon.current && instance) {
          fitAddon.current.fit();
          scrollToBottom();
          // Ensure terminal is properly refreshed after initial fit
          instance.refresh(0, instance.rows - 1);
          // Send ready signal after initial fit is complete
          sendReadySignal();
        }
      });
    }, 100);

    // Set up resize observer
    const resizeObserver = new ResizeObserver((entries) => {
      if (resizeTimeout.current) {
        clearTimeout(resizeTimeout.current);
      }

      resizeTimeout.current = window.setTimeout(() => {
        if (fitAddon.current && instance) {
          const newFontSize = fontSize(entries[0].target);
          if (instance.options.fontSize !== newFontSize) {
            instance.options.fontSize = newFontSize;
          }
          if (terminalReady.current) {
            fitAddon.current?.fit();
            scrollToBottom();
          }
        }
      }, 100);
    });

    const disposer = instance?.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        if (isReadOnly) {
          triggerReadOnlyShake();
          return;
        }
        ws.send(data);
      }
    });

    resizeObserver.observe(ref.current);
    observerRef.current = resizeObserver;

    // Cleanup function
    return () => {
      disposer?.dispose();
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      setIsConnected(false);
      fitAddon.current = null;
      webLinksAddon.current = null;
      renderAddon.current = null;
      if (observerRef.current) {
        observerRef.current.disconnect();
        observerRef.current = null;
      }
      if (resizeTimeout.current) {
        clearTimeout(resizeTimeout.current);
        resizeTimeout.current = null;
      }
      clearTimeout(initialFitTimeout);
    };
  }, [
    instance,
    worktree.id,
    worktree.path,
    setDims,
    triggerReadOnlyShake,
    isReadOnly,
    sendReadySignal,
  ]);

  // Show error display if there's an error
  if (error) {
    return (
      <div className="h-full w-full bg-background flex items-center justify-center">
        <ErrorDisplay
          title={error.title}
          message={error.message}
          onRetry={() => {
            setError(null);
            window.location.reload();
          }}
        />
      </div>
    );
  }

  return (
    <div className="h-full w-full bg-black relative">
      {/* Read-only indicator */}
      {isReadOnly && (
        <div
          className={`absolute top-4 right-4 z-10 bg-yellow-600/20 border border-yellow-500/50 text-yellow-300 px-3 py-1 rounded-md text-sm font-medium backdrop-blur-sm cursor-pointer hover:bg-yellow-600/30 hover:border-yellow-500/70 transition-all duration-200 ${
            shakeReadOnlyBadge ? "animate-pulse animate-bounce" : ""
          }`}
          onClick={handlePromoteRequest}
          title="Click to request write access"
        >
          👁️ Read Only
        </div>
      )}
      {/* Terminal */}
      <div className="h-full w-full p-4">
        <div ref={ref} className="h-full w-full" />
      </div>
    </div>
  );
}

export function WorkspaceMainContent({
  worktree,
  repository: _repository,
  showDiffView,
  setShowDiffView,
  showPortPreview,
  setShowPortPreview,
  selectedFile,
  setSelectedFile,
}: WorkspaceMainContentProps) {
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";
  const [isTerminalMinimized, setIsTerminalMinimized] = useState(false);
  const [previousTerminalSize, setPreviousTerminalSize] = useState(30);
  const [terminalSize, setTerminalSize] = useState(30);

  // Terminal tabs state
  const [terminals, setTerminals] = useState<TerminalTab[]>([
    { id: "default", name: "Terminal 1" },
  ]);
  const [activeTerminal, setActiveTerminal] = useState("default");

  // Terminal management functions
  const addTerminal = useCallback(() => {
    if (terminals.length >= 3) return;

    const newId = `terminal-${terminals.length + 1}`;
    const newTerminal = {
      id: newId,
      name: `Terminal ${terminals.length + 1}`,
    };

    setTerminals((prev) => [...prev, newTerminal]);
    setActiveTerminal(newId);
  }, [terminals.length]);

  const removeTerminal = useCallback(
    (terminalId: string) => {
      if (terminals.length <= 1) return; // Always keep at least one terminal

      setTerminals((prev) => prev.filter((t) => t.id !== terminalId));

      // If removing active terminal, switch to first remaining terminal
      if (activeTerminal === terminalId) {
        const remaining = terminals.filter((t) => t.id !== terminalId);
        setActiveTerminal(remaining[0]?.id || "default");
      }
    },
    [terminals, activeTerminal],
  );

  // Get diff stats to check if we have changes
  const { diffStats } = useWorktreeDiff(
    worktree.id,
    worktree.commit_hash,
    worktree.is_dirty,
  );

  const hasChanges =
    diffStats && diffStats.file_diffs && diffStats.file_diffs.length > 0;

  // Track panel sizes
  const handlePanelLayout = useCallback(
    (sizes: number[]) => {
      // The terminal panel is the second panel (index 1)
      if (sizes[1] && !isTerminalMinimized) {
        setTerminalSize(sizes[1]);
      }
    },
    [isTerminalMinimized],
  );

  // Handle minimize/expand toggle
  const toggleTerminalMinimized = useCallback(() => {
    if (!isTerminalMinimized) {
      // Save current size before minimizing
      setPreviousTerminalSize(terminalSize);
    }
    setIsTerminalMinimized(!isTerminalMinimized);
  }, [isTerminalMinimized, terminalSize]);

  return (
    <div className="flex flex-1 flex-col h-screen overflow-hidden">
      <ResizablePanelGroup
        direction="vertical"
        className="h-full"
        onLayout={handlePanelLayout}
        key={isTerminalMinimized ? "minimized" : "expanded"}
      >
        {/* Main Content Area - Claude Session or Diff View */}
        <ResizablePanel
          defaultSize={isTerminalMinimized ? 95 : 100 - previousTerminalSize}
          minSize={30}
        >
          <div className="h-full bg-muted/50 overflow-hidden">
            {showPortPreview ? (
              <div className="h-full">
                <PortPreview
                  port={showPortPreview}
                  onClose={() => setShowPortPreview(null)}
                />
              </div>
            ) : showDiffView ? (
              <div className="h-full">
                <WorkspaceDiffViewer
                  worktree={worktree}
                  selectedFile={selectedFile}
                  onClose={() => {
                    setShowDiffView(false);
                    setSelectedFile?.(undefined);
                  }}
                />
              </div>
            ) : (
              <ResizablePanelGroup direction="vertical" className="h-full">
                <ResizablePanel defaultSize={100} minSize={100}>
                  <div className="h-full flex flex-col">
                    <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          {isCollapsed && (
                            <SidebarTrigger className="h-4 w-4" />
                          )}
                          <img
                            src="/anthropic.png"
                            alt="Claude"
                            className="w-4 h-4"
                          />
                          <span className="text-sm font-medium">Claude</span>
                          {worktree.session_title && (
                            <span className="text-xs text-muted-foreground">
                              - {worktree.session_title.title}
                            </span>
                          )}
                        </div>
                        {/* Eyeball toggle - only show if we have changes */}
                        {hasChanges && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setShowDiffView(true)}
                            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
                            title="View diff"
                          >
                            <Eye className="w-3 h-3" />
                          </Button>
                        )}
                      </div>
                    </div>
                    <div className="flex-1 overflow-hidden">
                      <ClaudeTerminal worktree={worktree} />
                    </div>
                  </div>
                </ResizablePanel>
              </ResizablePanelGroup>
            )}
          </div>
        </ResizablePanel>

        {/* Resizable Handle */}
        {!isTerminalMinimized && (
          <ResizableHandle className="bg-border hover:bg-accent transition-colors cursor-row-resize" />
        )}

        {/* Terminal */}
        <ResizablePanel
          defaultSize={isTerminalMinimized ? 5 : previousTerminalSize}
          minSize={isTerminalMinimized ? 0 : 15}
          maxSize={isTerminalMinimized ? 5 : 70}
          collapsible={true}
        >
          <div className="flex flex-col bg-muted/50 overflow-hidden h-full">
            <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm flex-shrink-0">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 flex-1 min-w-0">
                  {isCollapsed && <SidebarTrigger className="h-4 w-4" />}

                  {/* Terminal Tabs */}
                  <div className="flex items-center gap-1 flex-1 min-w-0">
                    {terminals.map((terminal) => (
                      <div
                        key={terminal.id}
                        className={`flex items-center gap-1 px-2 py-1 rounded-md text-xs cursor-pointer transition-colors ${
                          activeTerminal === terminal.id
                            ? "bg-background text-foreground border border-border"
                            : "text-muted-foreground hover:text-foreground hover:bg-background/50"
                        }`}
                        onClick={() => setActiveTerminal(terminal.id)}
                      >
                        <span className="truncate max-w-[80px]">
                          {terminal.name}
                        </span>
                        {terminals.length > 1 && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              removeTerminal(terminal.id);
                            }}
                            className="ml-1 p-0.5 rounded hover:bg-destructive/20 hover:text-destructive transition-colors"
                            title={`Close ${terminal.name}`}
                          >
                            <X className="w-2.5 h-2.5" />
                          </button>
                        )}
                      </div>
                    ))}

                    {/* Add Terminal Button */}
                    {terminals.length < 3 && (
                      <button
                        onClick={addTerminal}
                        className="flex items-center justify-center w-6 h-6 rounded-md text-muted-foreground hover:text-foreground hover:bg-background/50 transition-colors"
                        title="Add new terminal (max 3)"
                      >
                        <Plus className="w-3 h-3" />
                      </button>
                    )}
                  </div>

                  <span className="text-xs text-muted-foreground ml-2">
                    /workspace/{worktree.name}
                  </span>
                </div>

                <Button
                  variant="ghost"
                  size="sm"
                  onClick={toggleTerminalMinimized}
                  className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground ml-2"
                  title={
                    isTerminalMinimized
                      ? "Expand terminal"
                      : "Minimize terminal"
                  }
                >
                  {isTerminalMinimized ? (
                    <ChevronUp className="w-3 h-3" />
                  ) : (
                    <ChevronDown className="w-3 h-3" />
                  )}
                </Button>
              </div>
            </div>
            {!isTerminalMinimized && (
              <div className="flex-1 min-h-0 pb-2 bg-black relative">
                {terminals.map((terminal) => (
                  <div
                    key={terminal.id}
                    className={`absolute inset-0 ${
                      activeTerminal === terminal.id ? "block" : "hidden"
                    }`}
                  >
                    <WorkspaceTerminal
                      worktree={worktree}
                      terminalId={terminal.id}
                    />
                  </div>
                ))}
              </div>
            )}
          </div>
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
}
