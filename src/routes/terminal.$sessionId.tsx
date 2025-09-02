import { createFileRoute, useSearch, useParams } from "@tanstack/react-router";
import { useEffect, useRef, useCallback, useState } from "react";
import { useXTerm } from "react-xtermjs";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { useWebSocket as useWebSocketContext } from "@/lib/hooks";
import { FileDropAddon } from "@/lib/file-drop-addon";
import { ErrorDisplay } from "@/components/ErrorDisplay";

// TerminalPage component
function TerminalPage() {
  const search = useSearch({
    from: "/terminal/$sessionId",
  }) as any;
  const params = useParams({ from: "/terminal/$sessionId" });
  const { instance, ref } = useXTerm();
  const { setIsConnected } = useWebSocketContext();
  const wsRef = useRef<WebSocket | null>(null);
  const wsReady = useRef(false);
  const terminalReady = useRef(false);
  const bufferingRef = useRef(false);
  const isSetup = useRef(false);
  const setupSessionClosed = useRef(false);
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
      setTimeout(() => setShakeReadOnlyBadge(false), 600); // Animation duration
    }
  }, [isReadOnly]);

  // Handle read-only badge click to request promotion
  const handlePromoteRequest = useCallback(() => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "promote" }));
    }
  }, []);

  // Send focus state to backend
  const sendFocusState = useCallback((focused: boolean) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "focus", focused }));
    }
  }, []);

  // Helper to generate a unique key for session storage
  const getPromptStorageKey = useCallback(
    async (sessionId: string, prompt: string) => {
      // Use Web Crypto API to generate SHA-256 hash
      const encoder = new TextEncoder();
      const data = encoder.encode(prompt);
      const hashBuffer = await crypto.subtle.digest("SHA-256", data);
      const hashArray = Array.from(new Uint8Array(hashBuffer));
      const hashHex = hashArray
        .map((b) => b.toString(16).padStart(2, "0"))
        .join("");
      return `catnip_prompt_${sessionId}_${hashHex.slice(0, 16)}`;
    },
    [],
  );

  // Check if prompt has already been executed
  const hasPromptBeenExecuted = useCallback(
    async (sessionId: string, prompt: string) => {
      const key = await getPromptStorageKey(sessionId, prompt);
      return sessionStorage.getItem(key) === "executed";
    },
    [getPromptStorageKey],
  );

  // Mark prompt as executed
  const markPromptAsExecuted = useCallback(
    async (sessionId: string, prompt: string) => {
      const key = await getPromptStorageKey(sessionId, prompt);
      sessionStorage.setItem(key, "executed");
    },
    [getPromptStorageKey],
  );

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

  // Handle terminal click to send focus immediately
  const handleTerminalClick = useCallback(() => {
    sendFocusState(true);
    // Auto-promote on click when read-only
    if (isReadOnly) {
      handlePromoteRequest();
    }
  }, [sendFocusState, isReadOnly, handlePromoteRequest]);

  // Handle tab focus/blur events
  useEffect(() => {
    const handleFocus = () => {
      sendFocusState(true);
    };

    const handleBlur = () => {
      sendFocusState(false);
    };

    const handleVisibilityChange = () => {
      sendFocusState(!document.hidden);
    };

    window.addEventListener("focus", handleFocus);
    window.addEventListener("blur", handleBlur);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    // Send initial focus state
    sendFocusState(document.hasFocus() && !document.hidden);

    return () => {
      window.removeEventListener("focus", handleFocus);
      window.removeEventListener("blur", handleBlur);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [sendFocusState]);

  // Reset state when sessionId changes
  useEffect(() => {
    isSetup.current = false;
    wsReady.current = false;
    terminalReady.current = false;
    bufferingRef.current = false;
    setupSessionClosed.current = false;
    lastConnectionAttempt.current = 0;
    setError(null); // Reset error state

    // Close existing WebSocket if any
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
  }, [params.sessionId]);

  useEffect(() => {
    if (wsReady.current && dims) {
      wsRef.current?.send(JSON.stringify({ type: "resize", ...dims }));
    }
  }, [dims, wsReady.current]);

  // Update terminal cursor when read-only state changes
  useEffect(() => {
    if (instance && instance.options) {
      instance.options.cursorBlink = search.agent !== "claude" && !isReadOnly;
      instance.options.theme = {
        ...instance.options.theme,
        cursor: isReadOnly
          ? "transparent"
          : search.agent === "claude"
            ? "#0a0a0a"
            : "#00ff95",
        cursorAccent: isReadOnly
          ? "transparent"
          : search.agent === "claude"
            ? "#0a0a0a"
            : "#00ff95",
      };
    }
  }, [isReadOnly, search.agent, instance]);

  // Set up terminal when instance and ref become available
  useEffect(() => {
    if (!instance || !ref.current) {
      return;
    }

    // Only set up once per session
    if (isSetup.current) {
      return;
    }

    // Don't reconnect if this is a setup session that has already closed
    if (search.agent === "setup" && setupSessionClosed.current) {
      console.log(
        "[Terminal] Setup session already closed, skipping reconnection",
      );
      return;
    }

    // Rate limit reconnections to once per second maximum
    const now = Date.now();
    if (now - lastConnectionAttempt.current < 1000) {
      console.log("[Terminal] Rate limiting connection attempt, too soon");
      return;
    }
    lastConnectionAttempt.current = now;

    isSetup.current = true;
    instance.clear();

    // Check if we're running against mock server - skip WebSocket if so
    const isMockMode = import.meta.env.VITE_USE_MOCK === "true";
    if (isMockMode) {
      setError({
        title: "Terminal Not Available",
        message:
          "Terminal functionality is not available in mock mode. This is expected when running without the Catnip backend.",
      });
      return;
    }

    // Set up WebSocket connection
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const urlParams = new URLSearchParams();
    if (params.sessionId) {
      urlParams.set("session", params.sessionId);
    }
    if (search.agent) {
      urlParams.set("agent", String(search.agent));
    }
    if (search.reset) {
      urlParams.set("reset", String(search.reset));
    }
    const socketUrl = `${protocol}//${
      window.location.host
    }/v1/pty?${urlParams.toString()}`;

    const ws = new WebSocket(socketUrl);
    wsRef.current = ws;
    const buffer: Uint8Array[] = [];

    ws.onopen = () => {
      setIsConnected(true);
      wsReady.current = true;
      sendReadySignal();
      // Send initial focus state after connection
      sendFocusState(document.hasFocus() && !document.hidden);
    };

    ws.onclose = () => {
      setIsConnected(false);
      // For setup sessions, prevent reconnection since they naturally exit
      if (search.agent === "setup") {
        console.log("[Terminal] Setup session closed, preventing reconnection");
        setupSessionClosed.current = true;
        return;
      }
    };

    ws.onerror = (error) => {
      console.error("❌ WebSocket error:", error);
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
            // Resize terminal to match buffer dimensions
            if (instance.cols !== msg.cols || instance.rows !== msg.rows) {
              instance.resize(msg.cols, msg.rows);
            }
            //instance.refresh(0, msg.rows - 1);
            bufferingRef.current = true;
            return;
          } else if (msg.type === "buffer-complete") {
            // Now fit to actual window size and send resize
            terminalReady.current = true;

            // Ensure terminal is properly fitted after buffer is complete
            requestAnimationFrame(() => {
              if (fitAddon.current) {
                fitAddon.current.fit();
              }
            });

            // Check if we have a prompt to send
            if (search.prompt && search.agent === "claude") {
              // Check if this prompt has already been executed for this session
              void hasPromptBeenExecuted(params.sessionId, search.prompt).then(
                (promptExecuted) => {
                  if (!promptExecuted) {
                    // Mark as executed before sending to prevent race conditions
                    void markPromptAsExecuted(
                      params.sessionId,
                      search.prompt,
                    ).then(() => {
                      console.log(
                        `[Terminal] Marking prompt as executed and sending to Claude`,
                      );

                      // Wait for Claude UI to fully load before sending prompt
                      setTimeout(() => {
                        wsRef.current?.send(
                          JSON.stringify({
                            type: "prompt",
                            data: search.prompt,
                            submit: true,
                          }),
                        );
                      }, 1000); // Give Claude TUI time to initialize
                    });
                  } else {
                    console.log(`[Terminal] Prompt already executed, skipping`);
                  }
                },
              );
            }
            const dims = { cols: instance.cols, rows: instance.rows };
            wsRef.current?.send(JSON.stringify({ type: "resize", ...dims }));
            return;
          } else if (msg.type === "read-only") {
            // Handle read-only status from server
            setIsReadOnly(msg.data === true);
            return;
          } else if (msg.type === "error") {
            // Handle error messages from server
            setError({
              title: msg.error || "Error",
              message: msg.message || "An unexpected error occurred",
            });
            return;
          }
        } catch (_e) {
          // Not JSON, treat as regular text
        }
        // Check if this is the shell exit message
        if (event.data.includes("[Shell exited, starting new session...]")) {
          // Clear the terminal before writing the message
          instance.clear();
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
        cursor: isReadOnly
          ? "transparent"
          : search.agent === "claude"
            ? "#0a0a0a"
            : "#00ff95",
        cursorAccent: isReadOnly
          ? "transparent"
          : search.agent === "claude"
            ? "#0a0a0a"
            : "#00ff95",
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
      instance.options.cursorBlink = search.agent !== "claude" && !isReadOnly;
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
    // Use requestAnimationFrame to ensure DOM layout is complete
    const initialFitTimeout = setTimeout(() => {
      requestAnimationFrame(() => {
        if (fitAddon.current) {
          fitAddon.current.fit();
        }
      });
    }, 50);

    // Set up resize observer before sending ready signal
    const resizeObserver = new ResizeObserver((entries) => {
      if (resizeTimeout.current) {
        clearTimeout(resizeTimeout.current);
      }

      resizeTimeout.current = window.setTimeout(() => {
        if (fitAddon.current && instance) {
          // Update font size based on screen width
          const newFontSize = fontSize(entries[0].target);
          if (instance.options.fontSize !== newFontSize) {
            instance.options.fontSize = newFontSize;
          }
          // Send resize to WebSocket
          if (terminalReady.current) {
            fitAddon.current?.fit();
          }
        }
      }, 100);
    });

    const disposer = instance?.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        // Check if we're in read-only mode and trigger shake animation
        if (isReadOnly) {
          triggerReadOnlyShake();
          return; // Don't send data if read-only
        }
        ws.send(data);
      }
    });

    // Mark terminal as ready and try to send ready signal
    sendReadySignal();

    resizeObserver.observe(ref.current);
    observerRef.current = resizeObserver;

    // Cleanup function - dispose of addons and close WebSocket when component unmounts
    return () => {
      disposer?.dispose();
      // Close WebSocket connection on cleanup
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
    params.sessionId,
    search.agent,
    setDims,
    hasPromptBeenExecuted,
    markPromptAsExecuted,
    triggerReadOnlyShake,
    isReadOnly,
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
    <div className="h-full w-full bg-black p-4 relative">
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
      <div
        className="h-full w-full"
        onMouseDown={handleTerminalClick}
        onFocus={handleTerminalClick}
        tabIndex={-1}
      >
        <div ref={ref} className="h-full w-full" />
      </div>
    </div>
  );
}

export const Route = createFileRoute("/terminal/$sessionId")({
  component: TerminalPage,
});
