import { useEffect, useRef, useCallback, useState } from "react";
import { useXTerm } from "react-xtermjs";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { useWebSocket as useWebSocketContext } from "@/lib/hooks";
import { FileDropAddon } from "@/lib/file-drop-addon";
import type { Worktree } from "@/lib/git-api";

interface XTerminalConfig {
  worktree: Worktree;
  terminalId?: string;
  isFocused?: boolean;
  agent?: string;
  enableAdvancedBuffering?: boolean;
}

interface XTerminalState {
  instance: ReturnType<typeof useXTerm>["instance"];
  ref: ReturnType<typeof useXTerm>["ref"];
  error: { title: string; message: string } | null;
  isReadOnly: boolean;
  shakeReadOnlyBadge: boolean;
  handlePromoteRequest: () => void;
}

export function useXTerminalConnection({
  worktree,
  terminalId = "default",
  isFocused = true,
  agent,
  enableAdvancedBuffering = false,
}: XTerminalConfig): XTerminalState {
  const { instance, ref } = useXTerm();
  const { setIsConnected } = useWebSocketContext();
  const wsRef = useRef<WebSocket | null>(null);
  const wsReady = useRef(false);
  const terminalReady = useRef(false);
  const bufferingRef = useRef(false);
  const isSetup = useRef(false);
  const lastConnectionAttempt = useRef(0);
  const readySignalSent = useRef(false);
  const fitAddon = useRef<FitAddon | null>(null);
  const webLinksAddon = useRef<WebLinksAddon | null>(null);
  const renderAddon = useRef<WebglAddon | null>(null);
  const resizeTimeout = useRef<number | null>(null);
  const observerRef = useRef<ResizeObserver | null>(null);

  // Advanced buffering state (for Claude terminal)
  const isFirstConnection = useRef(true);
  const lastWebSocketClose = useRef<number | null>(null);

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

  // Send focus state to backend when isFocused prop changes
  useEffect(() => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "focus", focused: isFocused }));
    }
  }, [isFocused]);

  const fontSize = useCallback((element: Element) => {
    if (element.clientWidth < 400) {
      return 6;
    } else if (element.clientWidth < 600 || element.clientHeight < 400) {
      return 10;
    } else {
      return 14;
    }
  }, []);

  // Track if user has scrolled up to view history
  const userScrolledUp = useRef(false);

  // Scroll terminal to bottom only if user hasn't scrolled up (for Claude terminal)
  const scrollToBottom = useCallback(() => {
    if (instance && !userScrolledUp.current) {
      instance.scrollToBottom();
    }
  }, [instance]);

  // Send ready signal when both WebSocket and terminal are ready (only once per connection)
  const sendReadySignal = useCallback(() => {
    if (
      !wsReady.current ||
      !wsRef.current ||
      !fitAddon.current ||
      readySignalSent.current
    ) {
      return;
    }
    readySignalSent.current = true;
    wsRef.current.send(JSON.stringify({ type: "ready" }));
    console.log("🎯 Ready signal sent to backend");
  }, []);

  // Reset state when worktree changes
  useEffect(() => {
    isSetup.current = false;
    wsReady.current = false;
    terminalReady.current = false;
    bufferingRef.current = false;
    lastConnectionAttempt.current = 0;
    readySignalSent.current = false;

    if (enableAdvancedBuffering) {
      isFirstConnection.current = true;
      lastWebSocketClose.current = null;
    }

    setError(null);

    // Reset terminal display to prevent prompt stacking between workspaces
    if (instance) {
      instance.clear();
      // Force a complete reset of terminal state
      instance.reset();
    }

    // Reset scroll tracking when switching workspaces
    if (agent === "claude") {
      userScrolledUp.current = false;
    }

    // Close existing WebSocket if any
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
  }, [worktree.id, enableAdvancedBuffering, instance, agent]);

  useEffect(() => {
    if (wsReady.current && dims) {
      wsRef.current?.send(JSON.stringify({ type: "resize", ...dims }));
    }
  }, [dims, wsReady.current]);

  // Claude terminal always has hidden cursor since it's a TUI
  useEffect(() => {
    if (instance && instance.options && agent === "claude") {
      instance.options.cursorBlink = false;
      instance.options.theme = {
        ...instance.options.theme,
        cursor: "#0a0a0a",
        cursorAccent: "#0a0a0a",
      };
    }
  }, [instance, agent]);

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
        `[${agent ? "Claude" : "Workspace"} Terminal] Rate limiting connection attempt, too soon`,
      );
      return;
    }
    lastConnectionAttempt.current = now;

    isSetup.current = true;

    // Clear terminal logic
    if (enableAdvancedBuffering) {
      // Advanced logic for Claude terminal
      const shouldClearTerminal =
        isFirstConnection.current ||
        (lastWebSocketClose.current &&
          now - lastWebSocketClose.current < 30000);

      if (shouldClearTerminal) {
        console.log(
          "[Claude Terminal] Clearing terminal - First connection:",
          isFirstConnection.current,
          "Recent close:",
          lastWebSocketClose.current
            ? now - lastWebSocketClose.current + "ms ago"
            : "none",
        );
        instance.clear();
        isFirstConnection.current = false;
        lastWebSocketClose.current = null;
      }
    } else {
      // Simple clear for workspace terminal
      instance.clear();
    }

    // Check if we're running against mock server - skip WebSocket if so
    const isMockMode = import.meta.env.VITE_USE_MOCK === "true";
    if (isMockMode) {
      if (agent) {
        console.log("📝 Skipping Claude terminal WebSocket in mock mode");
        return;
      } else {
        setError({
          title: "Terminal Not Available",
          message:
            "Terminal functionality is not available in mock mode. This is expected when running without the Catnip backend.",
        });
        return;
      }
    }

    // Set up WebSocket connection
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const urlParams = new URLSearchParams();

    // Create unique session name using terminalId
    const sessionName =
      terminalId === "default"
        ? worktree.name
        : `${worktree.name}:${terminalId}`;
    urlParams.set("session", sessionName);

    if (agent) {
      urlParams.set("agent", agent);
    }

    const socketUrl = `${protocol}//${window.location.host}/v1/pty?${urlParams.toString()}`;

    const ws = new WebSocket(socketUrl);
    wsRef.current = ws;
    const buffer: Uint8Array[] = [];

    ws.onopen = () => {
      setIsConnected(true);
      wsReady.current = true;
      readySignalSent.current = false; // Reset for new connection
      sendReadySignal();
    };

    ws.onclose = () => {
      console.log(
        `[${agent ? "Claude" : "Workspace"} Terminal] WebSocket closed`,
      );
      setIsConnected(false);
      if (enableAdvancedBuffering) {
        lastWebSocketClose.current = Date.now();
      }
    };

    ws.onerror = (error) => {
      console.error(
        `❌ ${agent ? "Claude" : "Workspace Terminal"} WebSocket error:`,
        error,
      );
      setIsConnected(false);

      // Handle WebSocket errors gracefully
      const isMockMode = import.meta.env.VITE_USE_MOCK === "true";
      if (isMockMode) {
        if (agent) {
          console.log(
            "📝 Claude terminal WebSocket failed in mock mode - this is expected",
          );
          return;
        } else {
          setError({
            title: "Terminal Not Available",
            message:
              "Terminal functionality is not available in mock mode. This is expected when running without the Catnip backend.",
          });
          return;
        }
      }

      // For real backend errors, set appropriate error state
      if (!agent) {
        setError({
          title: "Terminal Connection Failed",
          message:
            "Unable to connect to terminal. Please check your connection and try again.",
        });
      }
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
              if (agent === "claude") {
                // Force synchronous refresh after resize for Claude
                instance.refresh(0, msg.rows - 1);
              }
            }
            bufferingRef.current = true;
            return;
          } else if (msg.type === "buffer-complete") {
            terminalReady.current = true;

            if (enableAdvancedBuffering && buffer.length > 0) {
              // Advanced buffer handling for Claude terminal
              instance.clear();
              for (const chunk of buffer) {
                instance.write(chunk);
              }
              buffer.length = 0;
              // Reset scroll tracking when buffer content is written
              if (agent === "claude") {
                userScrolledUp.current = false;
              }
            }

            // Reset buffering flag
            bufferingRef.current = false;

            // Add a small delay before fitting to ensure content is rendered
            const delay = agent === "claude" ? 50 : 0;
            setTimeout(() => {
              requestAnimationFrame(() => {
                if (fitAddon.current) {
                  fitAddon.current.fit();
                  if (agent === "claude") {
                    scrollToBottom();
                    // Force a full refresh after fit for Claude
                    instance.refresh(0, instance.rows - 1);
                  }
                }
                // Send ready signal after initial fit for Claude
                if (agent === "claude") {
                  sendReadySignal();
                }
              });
            }, delay);

            // For workspace terminal, send dimensions immediately
            if (!agent) {
              const dims = { cols: instance.cols, rows: instance.rows };
              wsRef.current?.send(JSON.stringify({ type: "resize", ...dims }));
            }
            return;
          } else if (msg.type === "error") {
            setError({
              title: msg.error || "Error",
              message: msg.message || "An unexpected error occurred",
            });
            return;
          } else if (msg.type === "read-only") {
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
          if (!enableAdvancedBuffering) {
            bufferingRef.current = false;
          }
        } else {
          data = new Uint8Array(arrayBuffer);
        }
      } else {
        return;
      }

      // Handle buffered data for workspace terminal
      if (
        !bufferingRef.current &&
        buffer.length > 0 &&
        !enableAdvancedBuffering
      ) {
        rePaint();
        for (const chunk of buffer) {
          instance.write(chunk);
        }
        buffer.length = 0;
      }

      // Write data if not buffering
      if (data && !bufferingRef.current) {
        instance.write(data);
        // Reset scroll tracking when new content arrives (Claude terminal only)
        if (agent === "claude") {
          userScrolledUp.current = false;
        }
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
        cursor: agent === "claude" ? "#0a0a0a" : "#00ff95",
        cursorAccent: agent === "claude" ? "#0a0a0a" : "#00ff95",
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

      // Cursor configuration differs for Claude vs workspace
      if (agent === "claude") {
        // Hide cursor for Claude terminal since it's a TUI by matching background color
        instance.options.theme.cursor = "#0a0a0a";
        instance.options.theme.cursorAccent = "#0a0a0a";
      }

      instance.options.cursorBlink = agent !== "claude";
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

    // Add scroll listener to detect when user scrolls up (Claude terminal only)
    let scrollCleanup: (() => void) | null = null;
    if (agent === "claude") {
      const handleScroll = () => {
        if (instance) {
          const scrollTop = instance.buffer.active.viewportY;
          const scrollHeight = instance.buffer.active.length;
          const clientHeight = instance.rows;

          // Check if user has scrolled up from the bottom
          const isAtBottom = scrollTop >= scrollHeight - clientHeight;
          userScrolledUp.current = !isAtBottom;
        }
      };

      // Wait a bit for terminal to initialize, then add scroll listener
      setTimeout(() => {
        const terminalElement = ref.current?.querySelector(".xterm-viewport");
        if (terminalElement) {
          terminalElement.addEventListener("scroll", handleScroll);
          scrollCleanup = () => {
            terminalElement.removeEventListener("scroll", handleScroll);
          };
        }
      }, 200);
    }

    // Delay initial fit to allow layout to settle
    const initialFitTimeout = setTimeout(
      () => {
        requestAnimationFrame(() => {
          if (fitAddon.current && instance) {
            fitAddon.current.fit();
            if (agent === "claude") {
              scrollToBottom();
              // Ensure terminal is properly refreshed after initial fit
              instance.refresh(0, instance.rows - 1);
              // Send ready signal after initial fit is complete
              sendReadySignal();
            }
          }
        });
      },
      agent === "claude" ? 100 : 50,
    );

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
            if (agent === "claude") {
              scrollToBottom();
            }
          }
        }
      }, 100);
    });

    // Set up data handler for Claude terminal (inline)
    let disposer: any = null;
    if (agent === "claude") {
      disposer = instance?.onData((data: string) => {
        if (ws.readyState === WebSocket.OPEN) {
          if (isReadOnly) {
            triggerReadOnlyShake();
            return;
          }
          ws.send(data);
        }
      });
    }

    resizeObserver.observe(ref.current);
    observerRef.current = resizeObserver;

    // Cleanup function
    return () => {
      disposer?.dispose();
      scrollCleanup?.();
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
    worktree.name,
    terminalId,
    setDims,
    triggerReadOnlyShake,
    isReadOnly,
    sendReadySignal,
    agent,
    enableAdvancedBuffering,
    fontSize,
    scrollToBottom,
  ]);

  // Handle read-only data input separately for workspace terminal to avoid re-rendering
  useEffect(() => {
    if (!instance || agent === "claude") return; // Claude terminal handles this inline

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
  }, [isReadOnly, triggerReadOnlyShake, instance, agent]);

  return {
    instance,
    ref,
    error,
    isReadOnly,
    shakeReadOnlyBadge,
    handlePromoteRequest,
  };
}
