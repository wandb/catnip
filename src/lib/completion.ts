import { useState, useCallback } from "react";

// TypeScript interfaces matching the Go models
export interface CompletionMessage {
  role: string;
  content: string;
}

export interface CompletionRequest {
  prompt: string;
  max_tokens?: number;
  model?: string;
  system?: string;
  messages?: CompletionMessage[];
  stream?: boolean;
  working_directory?: string;
  suppress_events?: boolean;
}

export interface CompletionUsage {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
}

export interface CompletionResponse {
  response: string;
  usage: CompletionUsage;
  model: string;
  truncated: boolean;
}

export interface CompletionError {
  error: string;
}

// Configuration for completion requests
export interface CompletionConfig {
  request: CompletionRequest;
  ignoreCache?: boolean;
  cacheKey?: string;
}

// Hook return type
export interface UseCompletionResult {
  data: CompletionResponse | null;
  loading: boolean;
  error: string | null;
  execute: (config: CompletionConfig) => Promise<void>;
  clearCache: () => void;
}

// Cache utility functions
const CACHE_PREFIX = "catnip_completion_";
const CACHE_EXPIRY = 1 * 60 * 60 * 1000; // 1 hour

interface CacheEntry {
  data: CompletionResponse;
  timestamp: number;
}

function generateCacheKey(
  request: CompletionRequest,
  customKey?: string,
): string {
  if (customKey) {
    return `${CACHE_PREFIX}${customKey}`;
  }

  // Generate a key based on request content
  const keyData = {
    prompt: request.prompt,
    max_tokens: request.max_tokens,
    model: request.model,
    system: request.system,
    messages: request.messages,
    working_directory: request.working_directory,
  };

  return `${CACHE_PREFIX}${btoa(JSON.stringify(keyData))}`;
}

function getCachedResponse(cacheKey: string): CompletionResponse | null {
  try {
    const cached = localStorage.getItem(cacheKey);
    if (!cached) return null;

    const entry: CacheEntry = JSON.parse(cached) as CacheEntry;
    const now = Date.now();

    // Check if cache is expired
    if (now - entry.timestamp > CACHE_EXPIRY) {
      localStorage.removeItem(cacheKey);
      return null;
    }

    return entry.data;
  } catch (error) {
    console.error("Error reading from cache:", error);
    return null;
  }
}

function setCachedResponse(cacheKey: string, data: CompletionResponse): void {
  try {
    const entry: CacheEntry = {
      data,
      timestamp: Date.now(),
    };
    localStorage.setItem(cacheKey, JSON.stringify(entry));
  } catch (error) {
    console.error("Error writing to cache:", error);
  }
}

function clearCompletionCache(): void {
  try {
    const keys = Object.keys(localStorage);
    keys.forEach((key) => {
      if (key.startsWith(CACHE_PREFIX)) {
        localStorage.removeItem(key);
      }
    });
  } catch (error) {
    console.error("Error clearing cache:", error);
  }
}

// Direct usage function
export async function getCompletion(
  config: CompletionConfig,
): Promise<CompletionResponse> {
  const { request, ignoreCache = false, cacheKey } = config;

  // Check cache first (unless ignored)
  if (!ignoreCache) {
    const key = generateCacheKey(request, cacheKey);
    const cached = getCachedResponse(key);
    if (cached) {
      return cached;
    }
  }

  // Create abort controller for request timeout
  const controller = new AbortController();
  const timeoutId = setTimeout(() => {
    controller.abort();
  }, 10000); // 10 seconds

  try {
    // Make API request with timeout
    const response = await fetch("/v1/claude/messages", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    if (!response.ok) {
      let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
      try {
        const errorData: CompletionError =
          (await response.json()) as CompletionError;
        errorMessage = errorData.error || errorMessage;
      } catch (parseError) {
        // If we can't parse the error response, use the status message
        console.warn("Failed to parse error response:", parseError);
      }
      throw new Error(errorMessage);
    }

    const data: CompletionResponse =
      (await response.json()) as CompletionResponse;

    // Cache the response (unless cache is ignored)
    if (!ignoreCache) {
      const key = generateCacheKey(request, cacheKey);
      setCachedResponse(key, data);
    }

    return data;
  } catch (error) {
    clearTimeout(timeoutId);

    if (error instanceof Error) {
      if (error.name === "AbortError") {
        throw new Error(
          "Request timeout: The server did not respond within 10 seconds",
        );
      }
      throw error;
    }

    throw new Error("Unknown error occurred during completion request");
  }
}

// React hook for completion
export function useCompletion(): UseCompletionResult {
  const [data, setData] = useState<CompletionResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const execute = useCallback(async (config: CompletionConfig) => {
    setLoading(true);
    setError(null);

    try {
      const result = await getCompletion(config);
      setData(result);
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : "Unknown error occurred";
      setError(errorMessage);
      setData(null);
    } finally {
      setLoading(false);
    }
  }, []);

  const clearCache = useCallback(() => {
    clearCompletionCache();
  }, []);

  return {
    data,
    loading,
    error,
    execute,
    clearCache,
  };
}

// Utility function to create completion requests
export function createCompletionRequest(config: {
  prompt: string;
  maxTokens?: number;
  model?: string;
  system?: string;
  messages?: CompletionMessage[];
  stream?: boolean;
  workingDirectory?: string;
  suppressEvents?: boolean;
}): CompletionRequest {
  return {
    prompt: config.prompt,
    max_tokens: config.maxTokens,
    model: config.model, // No default - let Claude choose
    system: config.system,
    messages: config.messages,
    stream: config.stream,
    working_directory: config.workingDirectory,
    suppress_events: config.suppressEvents ?? false, // Default to false for user requests
  };
}

// SSE streaming interfaces
export interface StreamingEvent {
  type: string;
  data: any;
}

export interface StreamingConfig {
  request: CompletionRequest;
  onEvent: (event: StreamingEvent) => void;
  onError: (error: string) => void;
  onComplete: () => void;
}

// SSE streaming function
export function streamCompletion(config: StreamingConfig): () => void {
  const { request, onEvent, onError, onComplete } = config;

  // Create the streaming request
  const streamingRequest: CompletionRequest = {
    ...request,
    stream: true,
  };

  // Create abort controller for cleanup
  const controller = new AbortController();

  // Start the streaming fetch
  const startStream = async () => {
    try {
      console.log("Starting SSE stream with request:", streamingRequest);

      const response = await fetch("/v1/claude/messages", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "text/event-stream",
          "Cache-Control": "no-cache",
        },
        body: JSON.stringify(streamingRequest),
        signal: controller.signal,
      });

      if (!response.ok) {
        const errorText = await response.text();
        console.error("Stream request failed:", response.status, errorText);
        onError(
          `Stream request failed: ${response.status} ${response.statusText}`,
        );
        return;
      }

      if (!response.body) {
        onError("No response body received");
        return;
      }

      console.log("SSE connection opened");
      onEvent({ type: "connection_opened", data: null });

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      try {
        while (true) {
          const { done, value } = await reader.read();

          if (done) {
            console.log("SSE stream complete");
            onComplete();
            break;
          }

          const chunk = decoder.decode(value, { stream: true });
          console.log("SSE chunk received:", chunk);

          // Parse Server-Sent Events format
          const lines = chunk.split("\n");
          for (const line of lines) {
            if (line.startsWith("data: ")) {
              const dataStr = line.slice(6); // Remove "data: " prefix

              if (dataStr === "[DONE]") {
                console.log("SSE stream complete (DONE signal)");
                onComplete();
                break;
              }

              try {
                const data = JSON.parse(dataStr);
                console.log("SSE event parsed:", data);
                onEvent({ type: "message", data });
              } catch (_parseError) {
                // If it's not JSON, treat as plain text
                console.log("SSE text event:", dataStr);
                onEvent({ type: "message", data: dataStr });
              }
            }
          }
        }
      } finally {
        reader.releaseLock();
      }
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        console.log("Stream aborted");
        return;
      }

      console.error("SSE error:", error);
      onError(
        error instanceof Error ? error.message : "Stream connection error",
      );
    }
  };

  // Start the stream
  void startStream();

  // Return cleanup function
  return () => {
    console.log("Aborting SSE stream");
    controller.abort();
  };
}

// React hook for streaming completion
export interface UseStreamingCompletionResult {
  loading: boolean;
  error: string | null;
  events: StreamingEvent[];
  startStream: (
    config: Omit<StreamingConfig, "onEvent" | "onError" | "onComplete">,
  ) => () => void;
  clearEvents: () => void;
}

export function useStreamingCompletion(): UseStreamingCompletionResult {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [events, setEvents] = useState<StreamingEvent[]>([]);

  const startStream = useCallback(
    (config: Omit<StreamingConfig, "onEvent" | "onError" | "onComplete">) => {
      setLoading(true);
      setError(null);
      setEvents([]);

      const cleanup = streamCompletion({
        ...config,
        onEvent: (event) => {
          setEvents((prev) => [...prev, event]);
        },
        onError: (errorMessage) => {
          setError(errorMessage);
          setLoading(false);
        },
        onComplete: () => {
          setLoading(false);
        },
      });

      return cleanup;
    },
    [],
  );

  const clearEvents = useCallback(() => {
    setEvents([]);
    setError(null);
  }, []);

  return {
    loading,
    error,
    events,
    startStream,
    clearEvents,
  };
}

// Utility function to check cache status
export function getCacheStats(): { count: number; totalSize: number } {
  try {
    const keys = Object.keys(localStorage);
    const completionKeys = keys.filter((key) => key.startsWith(CACHE_PREFIX));

    let totalSize = 0;
    completionKeys.forEach((key) => {
      const value = localStorage.getItem(key);
      if (value) {
        totalSize += value.length;
      }
    });

    return {
      count: completionKeys.length,
      totalSize,
    };
  } catch (error) {
    console.error("Error getting cache stats:", error);
    return { count: 0, totalSize: 0 };
  }
}
