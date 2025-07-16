import { createFileRoute, useSearch, useParams } from "@tanstack/react-router";
import { useEffect, useRef, useCallback, useState } from "react";
import { useXTerm } from "react-xtermjs";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { useWebSocket as useWebSocketContext } from "@/lib/hooks";
import { FileDropAddon } from "@/lib/file-drop-addon";

// TODO: What a cluster fuck.  It's working reasonable well now.  Please clean this awful shit up.
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
  const fitAddon = useRef<FitAddon | null>(null);
  const webLinksAddon = useRef<WebLinksAddon | null>(null);
  const renderAddon = useRef<WebglAddon | null>(null);
  const resizeTimeout = useRef<number | null>(null);
  const observerRef = useRef<ResizeObserver | null>(null);
  const [dims, setDims] = useState<{ cols: number; rows: number } | null>(null);

  // Send ready signal when both WebSocket and terminal are ready
  const sendReadySignal = useCallback(() => {
    if (!wsReady.current || !wsRef.current || !fitAddon.current) {
      console.log("🔍 Not sending ready signal");
      return;
    }
    console.log("🎯 Sending ready signal");
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

  // Reset state when sessionId changes
  useEffect(() => {
    isSetup.current = false;
    wsReady.current = false;
    terminalReady.current = false;
    bufferingRef.current = false;

    // Close existing WebSocket if any
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
  }, [params.sessionId]);

  useEffect(() => {
    if (wsReady.current && dims) {
      console.log("🔍 Sending resize to WebSocket", dims);
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
      console.log("🔍 Terminal already setup, skipping");
      return;
    }

    isSetup.current = true;
    instance.clear();

    // Set up WebSocket connection
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const urlParams = new URLSearchParams();
    if (params.sessionId) {
      urlParams.set("session", params.sessionId);
    }
    if (search.agent) {
      urlParams.set("agent", String(search.agent));
    }
    const socketUrl = `${protocol}//${
      window.location.host
    }/v1/pty?${urlParams.toString()}`;

    console.log("🔗 Connecting to WebSocket:", socketUrl);
    const ws = new WebSocket(socketUrl);
    wsRef.current = ws;
    const buffer: Uint8Array[] = [];

    ws.onopen = () => {
      console.log("✅ WebSocket connected");
      setIsConnected(true);
      wsReady.current = true;
      sendReadySignal();
    };

    ws.onclose = () => {
      console.log("🔌 WebSocket disconnected");
      setIsConnected(false);
    };

    ws.onerror = (error) => {
      console.error("❌ WebSocket error:", error);
      setIsConnected(false);
    };

    ws.onmessage = async (event) => {
      let data: string | Uint8Array | undefined;
      const rePaint = () => {
        fitAddon.current?.fit();
        console.log("✅ Buffer replay complete, fitting terminal");
        fitAddon.current?.fit();
      };
      // Handle both binary and text data
      if (event.data instanceof ArrayBuffer) {
        if (bufferingRef.current) {
          console.log("🔍 Buffering ArrayBuffer", event.data.byteLength);
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
            console.log(
              `📐 Server wants terminal at ${msg.cols}x${msg.rows} for buffer replay`,
            );
            // Resize terminal to match buffer dimensions
            if (instance.cols !== msg.cols || instance.rows !== msg.rows) {
              instance.resize(msg.cols, msg.rows);
            }
            //instance.refresh(0, msg.rows - 1);
            bufferingRef.current = true;
            return;
          } else if (msg.type === "buffer-complete") {
            // Now fit to actual window size and send resize
            console.log("🔍 Terminal ready");
            terminalReady.current = true;
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
          console.log(
            "🔍 Buffering blob, claude?",
            search.agent,
            arrayBuffer.byteLength,
          );
          buffer.push(new Uint8Array(arrayBuffer));
          // TODO: this assumes there's one buffer message :(
          bufferingRef.current = false;
        } else {
          data = new Uint8Array(arrayBuffer);
        }
      } else {
        return;
      }

      if (!bufferingRef.current && buffer.length > 0) {
        console.log(
          "🔍 Writing buffer and calling repaint",
          instance.cols,
          instance.rows,
        );
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
        cursor: search.agent === "claude" ? "#0a0a0a" : "#00ff95",
        cursorAccent: search.agent === "claude" ? "#0a0a0a" : "#00ff95",
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
      instance.options.cursorBlink = search.agent !== "claude";
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
          console.log("🔍 Resizing terminal?:", terminalReady.current);
          // Send resize to WebSocket
          if (terminalReady.current) {
            fitAddon.current?.fit();
          }
        }
      }, 250);
    });

    const disposer = instance?.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    // Mark terminal as ready and try to send ready signal
    sendReadySignal();

    resizeObserver.observe(ref.current);
    observerRef.current = resizeObserver;

    // Cleanup function - dispose of addons when component unmounts
    return () => {
      disposer?.dispose();
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
    };
  }, [instance, params.sessionId, search.agent, setDims]);

  return (
    <div className="h-full bg-black p-4">
      {/* Terminal */}
      <div className="h-full terminal-container">
        <div ref={ref} className="h-full w-full" />
      </div>
    </div>
  );
}

export const Route = createFileRoute("/terminal/$sessionId")({
  component: TerminalPage,
});
