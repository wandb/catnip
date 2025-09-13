import { useEffect, useRef, useState, useCallback } from "react";

interface PtySSEConfig {
  sessionId: string;
  agent?: string;
  enabled?: boolean;
  prompt?: string;
}

interface PtySSEState {
  output: string;
  isConnected: boolean;
  isConnecting: boolean;
  error: string | null;
  claudeErrors: string[] | null;
  connect: () => void;
  disconnect: () => void;
  clearOutput: () => void;
  clearClaudeErrors: () => void;
}

export function usePtySSEConnection({
  sessionId,
  agent,
  enabled = true,
  prompt,
}: PtySSEConfig): PtySSEState {
  const [output, setOutput] = useState<string>("");
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [claudeErrors, setClaudeErrors] = useState<string[] | null>(null);

  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);
  const reconnectAttempts = useRef(0);
  const maxReconnectAttempts = 5;

  // Store current config in refs to avoid stale closures
  const configRef = useRef({ sessionId, agent, enabled, prompt });
  configRef.current = { sessionId, agent, enabled, prompt };

  // Clear output function
  const clearOutput = useCallback(() => {
    setOutput("");
  }, []);

  // Clear Claude errors function
  const clearClaudeErrors = useCallback(() => {
    setClaudeErrors(null);
  }, []);

  // Disconnect function
  const disconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    setIsConnected(false);
    setIsConnecting(false);
    reconnectAttempts.current = 0;
  }, []);

  // Connect function that uses refs to avoid stale closures
  const connect = useCallback(() => {
    const {
      sessionId: currentSessionId,
      agent: currentAgent,
      enabled: currentEnabled,
      prompt: currentPrompt,
    } = configRef.current;

    // Don't connect if already connecting or connected or disabled
    if (isConnecting || isConnected || !currentEnabled) {
      return;
    }

    // Check if we're running against mock server
    const isMockMode = import.meta.env.VITE_USE_MOCK === "true";
    if (isMockMode) {
      setError("PTY streaming is not available in mock mode");
      return;
    }

    setIsConnecting(true);
    setError(null);

    // Build SSE URL
    const urlParams = new URLSearchParams();
    urlParams.set("session", currentSessionId);
    if (currentAgent) {
      urlParams.set("agent", currentAgent);
    }
    if (currentPrompt) {
      urlParams.set("prompt", currentPrompt);
    }

    const sseUrl = `/v1/pty/sse?${urlParams.toString()}`;

    try {
      const eventSource = new EventSource(sseUrl);
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        console.log("PTY SSE connection opened for session:", currentSessionId);
        setIsConnected(true);
        setIsConnecting(false);
        setError(null);
        reconnectAttempts.current = 0;
      };

      eventSource.onmessage = (event) => {
        if (!event.data) return;

        try {
          // Try to parse as JSON first (for error messages)
          const parsed = JSON.parse(event.data);
          if (parsed.type === "claude-errors" && parsed.errors) {
            console.log("Received Claude errors:", parsed.errors);
            setClaudeErrors(parsed.errors);
          }
        } catch {
          // If not JSON, treat as raw terminal data (for backward compatibility)
          setOutput((prev) => prev + event.data);
        }
      };

      eventSource.onerror = (event) => {
        console.error("PTY SSE connection error:", event);
        setIsConnected(false);
        setIsConnecting(false);

        // Only set error state if we haven't connected yet or if we've exceeded max attempts
        const currentAttempts = reconnectAttempts.current;
        if (currentAttempts >= maxReconnectAttempts) {
          setError(`Failed to connect after ${maxReconnectAttempts} attempts`);
        } else {
          // Attempt to reconnect with exponential backoff
          reconnectAttempts.current++;
          const delay = Math.min(
            1000 * Math.pow(2, reconnectAttempts.current),
            10000,
          );

          reconnectTimeoutRef.current = window.setTimeout(() => {
            const { enabled: enabledAtRetry } = configRef.current;
            if (enabledAtRetry) {
              connect();
            }
          }, delay);
        }

        // Close the current connection
        if (eventSourceRef.current) {
          eventSourceRef.current.close();
          eventSourceRef.current = null;
        }
      };
    } catch (err) {
      console.error("Failed to create PTY SSE connection:", err);
      setError("Failed to create terminal stream connection");
      setIsConnecting(false);
    }
  }, []);

  // Auto-connect/disconnect when enabled changes
  useEffect(() => {
    if (enabled) {
      connect();
    } else {
      disconnect();
    }
  }, [enabled, connect, disconnect]);

  // Handle config changes (sessionId, agent, prompt)
  useEffect(() => {
    // If we have an active connection, disconnect and reconnect with new config
    if (eventSourceRef.current && enabled) {
      disconnect();
      clearOutput();
      // Small delay to ensure cleanup before reconnecting
      const timer = setTimeout(() => {
        connect();
      }, 100);
      return () => clearTimeout(timer);
    }
  }, [sessionId, agent, prompt]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      disconnect();
    };
  }, []);

  return {
    output,
    isConnected,
    isConnecting,
    error,
    claudeErrors,
    connect,
    disconnect,
    clearOutput,
    clearClaudeErrors,
  };
}
