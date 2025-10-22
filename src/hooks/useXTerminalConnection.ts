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
  isConnected: boolean;
  isConnecting: boolean;
  handleRetryConnection: () => void;
  terminalContainerRef: React.RefObject<HTMLDivElement | null>;
  handleTerminalFocus: () => void;
  isTerminalFocused: boolean;
}

export function useXTerminalConnection({
  worktree,
  terminalId = "default",
  isFocused: windowFocused = true,
  agent,
  enableAdvancedBuffering = false,
}: XTerminalConfig): XTerminalState {
  const { instance, ref } = useXTerm();
  const { setIsConnected: setGlobalIsConnected } = useWebSocketContext();
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

  // Reconnection management
  const reconnectTimeoutRef = useRef<number | null>(null);
  const readySignalTimeoutRef = useRef<number | null>(null);
  const reconnectAttempts = useRef(0);
  const maxReconnectAttempts = 5;
  const isConnectingRef = useRef(false);
  const hasEverConnected = useRef(false);
  const isWorktreeChanging = useRef(false);
  const terminalContainerRef = useRef<HTMLDivElement>(null);

  // Advanced buffering state (for Claude terminal)
  const isFirstConnection = useRef(true);
  const lastWebSocketClose = useRef<number | null>(null);
  const isSessionRestarting = useRef(false);

  const [dims, setDims] = useState<{ cols: number; rows: number } | null>(null);
  const [error, setError] = useState<{ title: string; message: string } | null>(
    null,
  );
  const [isReadOnly, setIsReadOnly] = useState(false);
  const [isNonRetryableError, setIsNonRetryableError] = useState(false);

  const [shakeReadOnlyBadge, setShakeReadOnlyBadge] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(true); // Start in connecting state
  const [isTerminalFocused, setIsTerminalFocused] = useState(false);

  // Track last sent focus state to avoid duplicate messages
  const lastSentFocusState = useRef<boolean | null>(null);

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

  // Handle terminal focus management
  const handleTerminalFocus = useCallback(() => {
    setIsTerminalFocused(true);
    // The focus state will be sent via the useEffect hook for sendFocusState
    // Auto-promote on focus when read-only
    if (
      isReadOnly &&
      wsRef.current &&
      wsRef.current.readyState === WebSocket.OPEN
    ) {
      wsRef.current.send(JSON.stringify({ type: "promote" }));
    }
  }, [isReadOnly]);

  // Send focus state to backend when terminal focus changes (with deduplication)
  const sendFocusState = useCallback(() => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      const actualFocus = windowFocused && isTerminalFocused;

      // Only send if focus state actually changed
      if (lastSentFocusState.current !== actualFocus) {
        lastSentFocusState.current = actualFocus;
        wsRef.current.send(
          JSON.stringify({ type: "focus", focused: actualFocus }),
        );
      }
    }
  }, [windowFocused, isTerminalFocused]);

  useEffect(() => {
    sendFocusState();
  }, [sendFocusState]);

  // Add global click handler to detect focus changes
  useEffect(() => {
    const handleGlobalClick = (event: MouseEvent) => {
      const target = event.target as Node;
      const isClickInsideThisTerminal =
        terminalContainerRef.current?.contains(target);

      if (!isClickInsideThisTerminal) {
        // Click was outside this terminal, remove focus
        setIsTerminalFocused(false);
      }
    };

    document.addEventListener("mousedown", handleGlobalClick);
    return () => {
      document.removeEventListener("mousedown", handleGlobalClick);
    };
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

  // Track if user has scrolled up to view history (for all terminals)
  const userScrolledUp = useRef(false);

  // Smart scroll function that respects user scroll position
  const smartScrollToBottom = useCallback(() => {
    if (!instance) return;

    // Always scroll to bottom if user hasn't manually scrolled up
    if (!userScrolledUp.current) {
      instance.scrollToBottom();
      return;
    }

    // If user has scrolled up, check if they're close to bottom (within 3 lines)
    // If so, assume they want to follow along and scroll to bottom
    const scrollTop = instance.buffer.active.viewportY;
    const scrollHeight = instance.buffer.active.length;
    const clientHeight = instance.rows;
    const distanceFromBottom = scrollHeight - (scrollTop + clientHeight);

    if (distanceFromBottom <= 3) {
      userScrolledUp.current = false; // Reset scroll tracking
      instance.scrollToBottom();
    }
  }, [instance]);

  // Send ready signal when both WebSocket and terminal are ready (only once per connection)
  const sendReadySignal = useCallback(() => {
    if (
      !wsReady.current ||
      !wsRef.current ||
      readySignalSent.current ||
      wsRef.current.readyState !== WebSocket.OPEN
    ) {
      return;
    }
    readySignalSent.current = true;
    wsRef.current.send(JSON.stringify({ type: "ready" }));
  }, [agent, terminalId]);

  // Calculate reconnect delay with custom backoff sequence: 1s, 3s, 6s, 9s, 18s
  const getReconnectDelay = useCallback(() => {
    const delays = [1000, 3000, 6000, 9000, 18000]; // 1s, 3s, 6s, 9s, 18s
    const attemptIndex = Math.min(
      reconnectAttempts.current - 1,
      delays.length - 1,
    );
    return delays[attemptIndex] || 1000;
  }, []);

  // WebSocket connection setup function
  const connectWebSocket = useCallback(() => {
    if (isConnectingRef.current) {
      return;
    }

    // Rate limit reconnections (but be more lenient on first connection)
    const now = Date.now();
    const minDelay = reconnectAttempts.current === 0 ? 100 : 1000;
    if (now - lastConnectionAttempt.current < minDelay) {
      return;
    }
    lastConnectionAttempt.current = now;

    // Check if we're running against mock server - skip WebSocket if so
    const isMockMode = import.meta.env.VITE_USE_MOCK === "true";
    if (isMockMode) {
      if (agent) {
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

    isConnectingRef.current = true;
    setIsConnecting(true);

    // Set up WebSocket connection
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const urlParams = new URLSearchParams();

    // Create unique session name using terminalId
    const sessionName =
      terminalId === "default"
        ? worktree.name
        : `${worktree.name}:${terminalId}`;
    urlParams.set("session", sessionName);

    // Add worktree path for additional scoping security
    urlParams.set("path", worktree.path);

    if (agent) {
      urlParams.set("agent", agent);
    }

    const socketUrl = `${protocol}//${window.location.host}/v1/pty?${urlParams.toString()}`;
    const ws = new WebSocket(socketUrl);
    wsRef.current = ws;
    const buffer: Uint8Array[] = [];

    ws.onopen = () => {
      // Reset terminal state on reconnection to prevent duplicate content
      // If session was restarting, do a full reset and clear
      if (isSessionRestarting.current) {
        instance?.reset();
        instance?.clear();
        isSessionRestarting.current = false;
      } else if (reconnectAttempts.current > 0) {
        instance?.reset();
      }

      setIsConnected(true);
      setGlobalIsConnected(true);
      isConnectingRef.current = false;
      setIsConnecting(false);
      reconnectAttempts.current = 0; // Reset attempts on successful connection
      hasEverConnected.current = true; // Track that we've connected at least once
      // Clear worktree changing flag - new WebSocket is connected
      isWorktreeChanging.current = false;
      wsReady.current = true;
      readySignalSent.current = false; // Reset for new connection
      lastSentFocusState.current = null; // Reset focus tracking for new connection
      // Add small delay to ensure WebSocket is fully ready
      readySignalTimeoutRef.current = window.setTimeout(() => {
        sendReadySignal();
        // Focus state will be sent by the useEffect hook when connection is ready
      }, 10);
    };

    ws.onclose = (_event) => {
      setIsConnected(false);
      setGlobalIsConnected(false);
      wsReady.current = false;
      isConnectingRef.current = false; // Reset connecting flag immediately

      // Clear any pending ready signal timeout
      if (readySignalTimeoutRef.current) {
        clearTimeout(readySignalTimeoutRef.current);
        readySignalTimeoutRef.current = null;
      }

      if (enableAdvancedBuffering) {
        lastWebSocketClose.current = Date.now();
      }

      // Don't reconnect if worktree is changing (prevents stale reconnections)
      if (isWorktreeChanging.current) {
        setIsConnecting(false);
        return;
      }

      // Don't reconnect if we received a non-retryable error
      if (isNonRetryableError) {
        setIsConnecting(false);
        return;
      }

      // Simple logic: if we haven't exceeded max attempts, try to reconnect
      if (reconnectAttempts.current < maxReconnectAttempts) {
        reconnectAttempts.current += 1;
        const delay = getReconnectDelay();

        // Show connecting state while retrying
        setIsConnecting(true);

        reconnectTimeoutRef.current = window.setTimeout(() => {
          // Force a fresh connectWebSocket call to use current worktree values
          connectWebSocket();
        }, delay);
      } else {
        // Only now show disconnected state and stop connecting
        setIsConnecting(false);
        setError({
          title: "Connection Lost",
          message: "Unable to reconnect to terminal. Please refresh the page.",
        });
      }
    };

    ws.onerror = (_error) => {
      setIsConnected(false);
      setGlobalIsConnected(false);
      wsReady.current = false;
      isConnectingRef.current = false; // Reset connecting flag immediately

      // Clear any pending ready signal timeout
      if (readySignalTimeoutRef.current) {
        clearTimeout(readySignalTimeoutRef.current);
        readySignalTimeoutRef.current = null;
      }

      // Handle WebSocket errors gracefully
      const isMockMode = import.meta.env.VITE_USE_MOCK === "true";
      if (isMockMode) {
        if (agent) {
          return;
        } else {
          setError({
            title: "Terminal Not Available",
            message:
              "Terminal functionality is not available in mock mode. This is expected when running without the Catnip backend.",
          });
          setIsConnecting(false);
          return;
        }
      }

      // Don't reconnect if we received a non-retryable error
      if (isNonRetryableError) {
        setIsConnecting(false);
        return;
      }

      // For real backend errors, attempt reconnection like onclose does
      if (reconnectAttempts.current < maxReconnectAttempts) {
        reconnectAttempts.current += 1;
        const delay = getReconnectDelay();

        // Show connecting state while retrying
        setIsConnecting(true);

        reconnectTimeoutRef.current = window.setTimeout(() => {
          connectWebSocket();
        }, delay);
      } else {
        // Only now show disconnected state and stop connecting
        setIsConnecting(false);
        setError({
          title: "Terminal Connection Failed",
          message:
            "Unable to connect to terminal after multiple attempts. The backend may be unavailable.",
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
            if (
              instance &&
              (instance.cols !== msg.cols || instance.rows !== msg.rows)
            ) {
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
              instance?.clear();
              for (const chunk of buffer) {
                instance?.write(chunk);
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
                    smartScrollToBottom();
                    // Force a full refresh after fit for Claude
                    instance?.refresh(0, instance.rows - 1);
                  }
                }
                // Send ready signal after initial fit for Claude
                if (agent === "claude") {
                  setTimeout(() => sendReadySignal(), 10);
                }
              });
            }, delay);

            // For workspace terminal, send dimensions immediately
            if (!agent) {
              const dims = instance
                ? { cols: instance.cols, rows: instance.rows }
                : { cols: 80, rows: 24 };
              wsRef.current?.send(JSON.stringify({ type: "resize", ...dims }));
            }
            return;
          } else if (msg.type === "error") {
            setError({
              title: msg.error || "Error",
              message: msg.message || "An unexpected error occurred",
            });
            // Check if this is a non-retryable error
            if (msg.retryable === false) {
              setIsNonRetryableError(true);
            }
            return;
          } else if (msg.type === "read-only") {
            setIsReadOnly(msg.data === true);
            return;
          } else if (msg.type === "session-restarting") {
            // Backend is restarting the session - prepare for full reset
            isSessionRestarting.current = true;
            instance?.clear();
            // Close the WebSocket from our side to trigger reconnection
            // This is more reliable than waiting for backend to close it
            ws.close();
            return;
          } else {
            // Any other JSON message - don't display in terminal
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
        return; // Unknown data type, ignore
      }

      // Handle buffered data for workspace terminal
      if (
        !bufferingRef.current &&
        buffer.length > 0 &&
        !enableAdvancedBuffering
      ) {
        rePaint();
        for (const chunk of buffer) {
          instance?.write(chunk);
        }
        buffer.length = 0;
        // Smart scroll after writing buffered data
        setTimeout(() => smartScrollToBottom(), 0);
      }

      // Write data if not buffering
      if (data && !bufferingRef.current) {
        instance?.write(data);
        // Smart scroll after writing data to ensure we follow new content
        setTimeout(() => smartScrollToBottom(), 0);
      }
    };
  }, [
    worktree.name,
    worktree.path,
    terminalId,
    instance,
    agent,
    enableAdvancedBuffering,
    sendReadySignal,
    getReconnectDelay,
    smartScrollToBottom,
    isNonRetryableError,
  ]);

  // Manual retry function that doesn't require page reload
  const handleRetryConnection = useCallback(() => {
    setError(null);
    setIsNonRetryableError(false); // Reset non-retryable error state for manual retry
    reconnectAttempts.current = 0; // Reset attempts for manual retry
    isConnectingRef.current = false;
    setIsConnecting(true); // Start in connecting state for retry
    hasEverConnected.current = false; // Reset connection history

    // Clear any pending timeouts
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    if (readySignalTimeoutRef.current) {
      clearTimeout(readySignalTimeoutRef.current);
      readySignalTimeoutRef.current = null;
    }

    // Close existing connection if any
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    // Start fresh connection
    connectWebSocket();
  }, [connectWebSocket]);

  // Reset state when worktree changes
  useEffect(() => {
    // Set flag to indicate worktree is changing
    isWorktreeChanging.current = true;

    isSetup.current = false;
    wsReady.current = false;
    terminalReady.current = false;
    bufferingRef.current = false;
    lastConnectionAttempt.current = 0;
    readySignalSent.current = false;
    reconnectAttempts.current = 0;
    hasEverConnected.current = false;
    isSessionRestarting.current = false;

    if (enableAdvancedBuffering) {
      isFirstConnection.current = true;
      lastWebSocketClose.current = null;
    }

    setError(null);
    setIsNonRetryableError(false); // Reset non-retryable error state for new worktree
    setIsConnected(false);
    setIsConnecting(true); // Start in connecting state for new worktree
    setGlobalIsConnected(false);

    // Clear any pending timeouts
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    if (readySignalTimeoutRef.current) {
      clearTimeout(readySignalTimeoutRef.current);
      readySignalTimeoutRef.current = null;
    }

    // Reset terminal display to prevent prompt stacking between workspaces
    if (instance) {
      instance.clear();
      // Force a complete reset of terminal state
      instance.reset();
    }

    // Reset scroll tracking when switching workspaces (all terminals)
    userScrolledUp.current = false;

    // Close existing WebSocket if any (but only if it's not connected and working)
    if (wsRef.current) {
      // Don't close if WebSocket is connected and working properly
      if (wsRef.current.readyState !== WebSocket.OPEN || !isConnected) {
        // Temporarily prevent reconnection during cleanup
        const oldWs = wsRef.current;
        wsRef.current = null; // Clear reference first to prevent reconnection attempts
        oldWs.close();
      }
    }
  }, [worktree.id, worktree.path, enableAdvancedBuffering, instance, agent]);

  useEffect(() => {
    if (
      wsReady.current &&
      dims &&
      wsRef.current?.readyState === WebSocket.OPEN
    ) {
      wsRef.current.send(JSON.stringify({ type: "resize", ...dims }));
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

    isSetup.current = true;

    // Clear terminal logic
    if (enableAdvancedBuffering) {
      // Advanced logic for Claude terminal
      const now = Date.now();
      const shouldClearTerminal =
        isFirstConnection.current ||
        (lastWebSocketClose.current &&
          now - lastWebSocketClose.current < 30000);

      if (shouldClearTerminal) {
        instance.clear();
        isFirstConnection.current = false;
        lastWebSocketClose.current = null;
      }
    } else {
      // Simple clear for workspace terminal
      instance.clear();
    }

    // Start WebSocket connection
    connectWebSocket();

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
      if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
        wsRef.current.send(data);
      }
    };
    const fileDropAddon = new FileDropAddon(sendData);
    instance.loadAddon(fileDropAddon);

    instance.onResize((event) => {
      setDims({ cols: event.cols, rows: event.rows });
    });

    // Open terminal in DOM element
    instance.open(ref.current);

    // Add scroll listener to detect when user scrolls up (all terminals)
    let scrollCleanup: (() => void) | null = null;
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

    // Delay initial fit to allow layout to settle
    const initialFitTimeout = setTimeout(
      () => {
        requestAnimationFrame(() => {
          if (fitAddon.current && instance) {
            fitAddon.current.fit();

            // Smart scroll on initial load
            smartScrollToBottom();

            if (agent === "claude") {
              // Ensure terminal is properly refreshed after initial fit
              instance.refresh(0, instance.rows - 1);
              // Send ready signal after initial fit is complete
              setTimeout(() => sendReadySignal(), 10);
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
            // Smart scroll on resize
            smartScrollToBottom();
          }
        }
      }, 100);
    });

    // Data handler for Claude terminal is now handled by separate useEffect below
    // This ensures it updates when isReadOnly changes

    resizeObserver.observe(ref.current);
    observerRef.current = resizeObserver;

    // Cleanup function
    return () => {
      // Removed shouldReconnect flag completely - was causing race conditions
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
      if (readySignalTimeoutRef.current) {
        clearTimeout(readySignalTimeoutRef.current);
        readySignalTimeoutRef.current = null;
      }
      scrollCleanup?.();
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      setIsConnected(false);
      setGlobalIsConnected(false);
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
    worktree.path,
    terminalId,
    agent,
    enableAdvancedBuffering,
    // Removed all callback dependencies that change on every render
  ]);

  // Handle read-only data input for both terminal types
  useEffect(() => {
    if (!instance) return;

    const dataHandler = (data: string) => {
      // Ensure terminal is focused when user types
      if (!isTerminalFocused) {
        setIsTerminalFocused(true);
      }

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
  }, [isReadOnly, triggerReadOnlyShake, instance, agent, isTerminalFocused]);

  return {
    instance,
    ref,
    error,
    isReadOnly,
    shakeReadOnlyBadge,
    handlePromoteRequest,
    isConnected,
    isConnecting,
    handleRetryConnection,
    terminalContainerRef,
    handleTerminalFocus,
    isTerminalFocused,
  };
}
