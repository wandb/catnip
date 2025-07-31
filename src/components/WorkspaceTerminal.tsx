import { useEffect, useRef, useCallback, useState } from "react";
import { useXTerm } from "react-xtermjs";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { useWebSocket as useWebSocketContext } from "@/lib/hooks";
import { FileDropAddon } from "@/lib/file-drop-addon";
import { ErrorDisplay } from "@/components/ErrorDisplay";
import type { Worktree } from "@/lib/git-api";

interface WorkspaceTerminalProps {
  worktree: Worktree;
}

export function WorkspaceTerminal({ worktree }: WorkspaceTerminalProps) {
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
  const [error, setError] = useState<{ title: string; message: string } | null>(
    null,
  );
  const [isReadOnly, setIsReadOnly] = useState(false);
  const [shakeReadOnlyBadge, setShakeReadOnlyBadge] = useState(false);

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

  const fontSize = useCallback((element: Element) => {
    if (element.clientWidth < 400) {
      return 6;
    } else if (element.clientWidth < 600 || element.clientHeight < 400) {
      return 10;
    } else {
      return 14;
    }
  }, []);

  // Send ready signal when both WebSocket and terminal are ready
  const sendReadySignal = useCallback(() => {
    if (!wsReady.current || !wsRef.current || !fitAddon.current) {
      return;
    }
    wsRef.current.send(JSON.stringify({ type: "ready" }));
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
        "[Workspace Terminal] Rate limiting connection attempt, too soon",
      );
      return;
    }
    lastConnectionAttempt.current = now;

    isSetup.current = true;
    instance.clear();

    // Set up WebSocket connection for bash terminal in the workspace directory
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const urlParams = new URLSearchParams();
    urlParams.set("session", worktree.name);
    // Don't set agent parameter - this should be a regular bash terminal

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
      console.error("❌ Workspace Terminal WebSocket error:", error);
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
        // Try to parse as JSON for control messages ONLY
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
          } else if (msg.type === "error") {
            setError({
              title: msg.error || "Error",
              message: msg.message || "An unexpected error occurred",
            });
            return;
          } else if (msg.type === "read-only") {
            // Handle read-only messages for workspace terminal
            setIsReadOnly(msg.data === true);
            return;
          }
        } catch (_e) {
          // Not JSON, treat as regular text output
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
        cursor: "#00ff95",
        cursorAccent: "#00ff95",
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
      instance.options.cursorBlink = true;
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

    resizeObserver.observe(ref.current);
    observerRef.current = resizeObserver;

    // Cleanup function
    return () => {
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
  }, [instance, worktree.id, worktree.name, setDims, sendReadySignal]);

  // Handle read-only data input separately to avoid re-rendering the entire terminal
  useEffect(() => {
    if (!instance) return;

    const dataHandler = (data: string) => {
      if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
        if (isReadOnly) {
          triggerReadOnlyShake();
          return;
        }
        wsRef.current.send(data);
      }
    };

    // Remove existing data handler and add new one
    const disposer = instance.onData(dataHandler);
    return () => disposer?.dispose();
  }, [isReadOnly, triggerReadOnlyShake, instance]);

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
          className={`absolute top-2 right-2 z-10 bg-yellow-600/20 border border-yellow-500/50 text-yellow-300 px-2 py-1 rounded-md text-xs font-medium backdrop-blur-sm cursor-pointer hover:bg-yellow-600/30 hover:border-yellow-500/70 transition-all duration-200 ${
            shakeReadOnlyBadge ? "animate-pulse animate-bounce" : ""
          }`}
          onClick={handlePromoteRequest}
          title="Click to request write access"
        >
          👁️ Read Only
        </div>
      )}
      {/* Terminal with minimal padding */}
      <div ref={ref} className="h-full w-full p-2" />
    </div>
  );
}
