import { useEffect, useRef, useCallback, useState } from "react";
import { useXTerm } from "react-xtermjs";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { useWebSocket as useWebSocketContext } from "@/lib/hooks";
import { FileDropAddon } from "@/lib/file-drop-addon";
import { ErrorDisplay } from "@/components/ErrorDisplay";
import { WorkspaceTerminal } from "@/components/WorkspaceTerminal";
import { SidebarTrigger, useSidebar } from "@/components/ui/sidebar";
import type { Worktree, LocalRepository } from "@/lib/git-api";

interface WorkspaceMainContentProps {
  worktree: Worktree;
  repository: LocalRepository;
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

  // Reset state when worktree changes
  useEffect(() => {
    isSetup.current = false;
    wsReady.current = false;
    terminalReady.current = false;
    bufferingRef.current = false;
    lastConnectionAttempt.current = 0;
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
    instance.clear();

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
      const rePaint = () => {
        fitAddon.current?.fit();
      };

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
              instance.resize(msg.cols, msg.rows);
            }
            bufferingRef.current = true;
            return;
          } else if (msg.type === "buffer-complete") {
            terminalReady.current = true;
            requestAnimationFrame(() => {
              if (fitAddon.current) {
                fitAddon.current.fit();
              }
            });
            const dims = { cols: instance.cols, rows: instance.rows };
            wsRef.current?.send(JSON.stringify({ type: "resize", ...dims }));
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
        rePaint();
        for (const chunk of buffer) {
          instance.write(chunk);
        }
        buffer.length = 0;
      }
      if (data) {
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
        if (fitAddon.current) {
          fitAddon.current.fit();
        }
      });
    }, 50);

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

    sendReadySignal();
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

export function WorkspaceMainContent({ worktree }: WorkspaceMainContentProps) {
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";

  return (
    <div className="flex flex-1 flex-col h-screen overflow-hidden">
      {/* Claude Session */}
      <div className="flex-1 bg-muted/50 overflow-hidden">
        <div className="h-full flex flex-col">
          <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
            <div className="flex items-center gap-2">
              {isCollapsed && <SidebarTrigger className="h-4 w-4" />}
              <div className="w-2 h-2 bg-blue-500 rounded-full animate-pulse"></div>
              <span className="text-sm font-medium">Claude</span>
              {worktree.session_title && (
                <span className="text-xs text-muted-foreground">
                  - {worktree.session_title.title}
                </span>
              )}
            </div>
          </div>
          <div className="flex-1">
            <ClaudeTerminal worktree={worktree} />
          </div>
        </div>
      </div>

      {/* Separator */}
      <div className="h-px bg-border"></div>

      {/* Terminal */}
      <div
        className="flex flex-col bg-muted/50 overflow-hidden"
        style={{ height: "300px" }}
      >
        <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm flex-shrink-0">
          <div className="flex items-center gap-2">
            <div className="w-2 h-2 bg-green-500 rounded-full"></div>
            <span className="text-sm font-medium">Terminal</span>
            <span className="text-xs text-muted-foreground">
              {worktree.path}
            </span>
          </div>
        </div>
        <div className="flex-1 min-h-0">
          <WorkspaceTerminal worktree={worktree} />
        </div>
      </div>
    </div>
  );
}
