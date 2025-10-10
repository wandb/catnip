import * as SecureStore from "expo-secure-store";
import EventSource from "react-native-sse";

const BASE_URL = "https://catnip.run";

export interface CodespaceInfo {
  name: string;
  lastUsed: number;
  repository?: string;
}

export interface WorkspaceInfo {
  id: string;
  name: string;
  branch: string;
  repo_id: string; // This is the repository field from EnhancedWorktree
  claude_activity_state?: "active" | "running" | "inactive";
  commit_count?: number;
  is_dirty?: boolean;
  last_accessed?: string;
  created_at?: string;
  todos?: Todo[];
  latest_session_title?: string;
  pull_request_url?: string;
  path: string;
}

export interface Todo {
  content: string;
  status: "pending" | "in_progress" | "completed";
  activeForm?: string;
}

class CatnipAPI {
  private async getHeaders(includeCodespace = false): Promise<HeadersInit> {
    const sessionToken = await SecureStore.getItemAsync("session_token");

    if (!sessionToken) {
      throw new Error("No session token available");
    }

    const headers: HeadersInit = {
      "Content-Type": "application/json",
      Authorization: `Bearer ${sessionToken}`,
    };

    if (includeCodespace) {
      const codespaceName = await SecureStore.getItemAsync("codespace_name");
      if (codespaceName) {
        (headers as Record<string, string>)["X-Codespace-Name"] = codespaceName;
        console.log("🐱 Adding codespace header:", codespaceName);
      } else {
        console.warn("🐱 No codespace name found in storage");
      }
    }

    return headers;
  }

  connectCodespace(
    codespaceName?: string,
    org?: string,
    onEvent?: (event: any) => void,
  ): { promise: Promise<void>; cleanup: () => void } {
    console.log(
      "🐱 Connecting to codespace:",
      codespaceName ? `${codespaceName}` : "auto-select",
    );

    let eventSource: EventSource | null = null;
    let timeoutId: NodeJS.Timeout | null = null;
    let isResolved = false;

    const cleanup = () => {
      if (timeoutId) {
        clearTimeout(timeoutId);
        timeoutId = null;
      }
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      isResolved = true;
    };

    const promise = new Promise<void>(async (resolve, reject) => {
      try {
        const headers = await this.getHeaders();
        const baseUrl = org ? `https://${org}.catnip.run` : BASE_URL;
        const url = codespaceName
          ? `${baseUrl}/v1/codespace?codespace=${encodeURIComponent(codespaceName)}`
          : `${baseUrl}/v1/codespace`;

        console.log("🐱 Creating EventSource with react-native-sse...");

        eventSource = new EventSource(url, {
          headers: headers as Record<string, string>,
          withCredentials: false,
          pollingInterval: 0, // No polling, pure SSE
        });
        console.log("🐱 EventSource created successfully");

        // Single 2-minute timeout for the entire connection
        timeoutId = setTimeout(() => {
          if (!isResolved) {
            console.log("🐱 Connection timeout after 2 minutes");
            cleanup();
            reject(new Error("Connection timeout: Server not responding"));
          }
        }, 120000) as any;

        eventSource.addEventListener("open", () => {
          console.log("🐱 SSE connection opened");
        });

        // Handle specific event types that your server sends
        eventSource.addEventListener("status" as any, (event: any) => {
          if (isResolved) return;

          try {
            const data = JSON.parse(event.data);
            console.log("🐱 Status:", data.message);

            if (onEvent) {
              onEvent({ type: "status", ...data });
            }
          } catch (parseError) {
            console.error("🐱 Error parsing status event:", parseError);
          }
        });

        eventSource.addEventListener("success" as any, async (event: any) => {
          if (isResolved) return;
          console.log("🐱 Codespace ready!");

          try {
            const data = JSON.parse(event.data);

            if (onEvent) {
              onEvent({ type: "success", ...data });
            }

            // Store the codespace name for future API calls
            if (codespaceName) {
              await SecureStore.setItemAsync("codespace_name", codespaceName);
            }

            cleanup();
            resolve();
          } catch (parseError) {
            console.error("🐱 Error parsing success event:", parseError);
            cleanup();
            resolve();
          }
        });

        eventSource.addEventListener("error", (event: any) => {
          if (isResolved) return;
          console.log("🐱 Error event received:", event.data);

          try {
            const data = JSON.parse(event.data);
            console.log("🐱 Parsed error data:", data);

            if (onEvent) {
              onEvent({ type: "error", ...data });
            }

            // Only reject on final errors, not transient ones
            if (data.message && data.message.includes("permanently")) {
              cleanup();
              reject(new Error(data.message || "Server reported error"));
            }
            // Otherwise, let the connection continue - might get success later
          } catch (parseError) {
            console.error("🐱 Error parsing error event:", parseError);
            cleanup();
            reject(new Error("Server error"));
          }
        });

        eventSource.addEventListener("setup" as any, (event: any) => {
          if (isResolved) return;
          console.log("🐱 Setup event received:", event.data);

          try {
            const data = JSON.parse(event.data);
            console.log("🐱 Parsed setup data:", data);

            if (onEvent) {
              onEvent({ type: "setup", ...data });
            }
          } catch (parseError) {
            console.error("🐱 Error parsing setup event:", parseError);
          }
        });

        eventSource.addEventListener("multiple" as any, (event: any) => {
          if (isResolved) return;
          console.log("🐱 Multiple event received:", event.data);

          try {
            const data = JSON.parse(event.data);
            console.log("🐱 Parsed multiple data:", data);

            if (onEvent) {
              onEvent({ type: "multiple", ...data });
            }
          } catch (parseError) {
            console.error("🐱 Error parsing multiple event:", parseError);
          }
        });

        // Fallback for generic message events
        eventSource.addEventListener("message", (event: any) => {
          if (isResolved) return;
          console.log("🐱 Generic message received:", event.data);

          try {
            const data = event.data ? JSON.parse(event.data) : null;
            console.log("🐱 Parsed generic message:", data);

            if (onEvent) {
              onEvent(data);
            }

            if (data.type === "success") {
              cleanup();
              resolve();
            }
          } catch (parseError) {
            console.error("🐱 Error parsing generic message:", parseError);
          }
        });

        // Handle connection errors (not event data errors)
        (eventSource as any).onerror = (error: any) => {
          if (isResolved) return;
          console.error("🐱 SSE connection error:", error);

          const errorMessage = error?.message || "SSE connection failed";
          if (onEvent) {
            onEvent({
              type: "error",
              message: errorMessage,
            });
          }
          cleanup();
          reject(new Error(errorMessage));
        };
      } catch (createError) {
        console.error("🐱 Failed to create EventSource:", createError);
        cleanup();
        reject(createError);
      }
    });

    return { promise, cleanup };
  }

  async getWorkspaces(
    ifNoneMatch?: string,
  ): Promise<{ workspaces: WorkspaceInfo[]; etag?: string } | null> {
    try {
      const headers = await this.getHeaders(true); // Include codespace header

      // Add If-None-Match header for conditional request
      if (ifNoneMatch) {
        (headers as Record<string, string>)["If-None-Match"] = ifNoneMatch;
      }

      const response = await fetch(`${BASE_URL}/v1/git/worktrees`, { headers });

      // Handle 304 Not Modified - content unchanged
      if (response.status === 304) {
        console.log("🐱 Workspaces not modified (304)");
        return null;
      }

      if (!response.ok) {
        const responseText = await response.text();
        console.error(
          "🐱 Failed to fetch workspaces:",
          response.status,
          responseText,
        );
        throw new Error(
          `Failed to fetch workspaces (${response.status}): ${responseText}`,
        );
      }

      const responseText = await response.text();

      if (!responseText || responseText.trim() === "") {
        console.log("🐱 Empty response from workspaces endpoint");
        return { workspaces: [] };
      }

      try {
        const parsed = JSON.parse(responseText);
        const workspaces = Array.isArray(parsed) ? parsed : [];
        console.log("🐱 Loaded", workspaces.length, "workspaces");

        // Extract ETag from response headers
        const etag = response.headers.get("ETag") || undefined;

        return { workspaces, etag };
      } catch (parseError) {
        console.error("🐱 Error parsing workspaces JSON:", parseError);
        throw new Error("Invalid JSON response from workspaces endpoint");
      }
    } catch (error) {
      console.error("🐱 Error in getWorkspaces:", error);
      throw error;
    }
  }

  async getWorkspace(
    id: string,
    ifNoneMatch?: string,
  ): Promise<{ workspace: WorkspaceInfo; etag?: string } | null> {
    try {
      // Get all workspaces with ETag support
      const result = await this.getWorkspaces(ifNoneMatch);

      // If 304 Not Modified, return null
      if (result === null) {
        return null;
      }

      const workspace = result.workspaces.find((w) => w.id === id);

      if (!workspace) {
        throw new Error(`Workspace with ID ${id} not found`);
      }

      console.log("🐱 Found workspace:", workspace);
      return { workspace, etag: result.etag };
    } catch (error) {
      console.error("🐱 Error getting workspace:", error);
      throw error;
    }
  }

  async getClaudeSessions(): Promise<Record<string, any>> {
    try {
      const headers = await this.getHeaders(true);
      const response = await fetch(`${BASE_URL}/v1/claude/sessions`, {
        headers,
      });

      if (!response.ok) {
        const responseText = await response.text();
        console.error(
          "🐱 Failed to fetch Claude sessions:",
          response.status,
          responseText,
        );
        return {};
      }

      const result = await response.json();
      return result || {};
    } catch (error) {
      console.error("🐱 Error fetching Claude sessions:", error);
      return {};
    }
  }

  async getWorktreeLatestMessage(
    worktreePath: string,
  ): Promise<{ content: string; isError: boolean }> {
    try {
      const headers = await this.getHeaders(true);
      const response = await fetch(
        `${BASE_URL}/v1/claude/latest-message?worktree_path=${encodeURIComponent(worktreePath)}`,
        { headers },
      );

      if (!response.ok) {
        const responseText = await response.text();
        console.error(
          "🐱 Failed to fetch latest message:",
          response.status,
          responseText,
        );
        return { content: "Failed to fetch message", isError: true };
      }

      const result = await response.json();
      return {
        content: result.content || "",
        isError: result.isError || false,
      };
    } catch (error) {
      console.error("🐱 Error fetching latest message:", error);
      return { content: "Error fetching message", isError: true };
    }
  }

  async sendPrompt(workspacePath: string, prompt: string): Promise<void> {
    const headers = await this.getHeaders(true); // Include codespace header

    // Use SSE endpoint for PTY session (gives us auto-commits and session tracking)
    const params = new URLSearchParams({
      session: workspacePath,
      agent: "claude",
      prompt: prompt,
    });

    const url = `${BASE_URL}/v1/pty/sse?${params.toString()}`;

    // Fire off the SSE request asynchronously - we don't need to process the stream
    // The backend will handle prompt injection into the PTY session
    fetch(url, { headers })
      .then((response) => {
        if (!response.ok) {
          console.error("Failed to send prompt via SSE:", response.status);
        } else {
          console.log("Prompt sent successfully via SSE");
          // Close the stream immediately since we don't need to read it
          response.body?.cancel();
        }
      })
      .catch((error) => {
        console.error("Error sending prompt via SSE:", error);
      });

    // Return immediately - prompt injection happens asynchronously in the PTY session
  }

  async createWorkspace(
    orgRepo: string,
    branch?: string,
  ): Promise<WorkspaceInfo> {
    const headers = await this.getHeaders(true); // Include codespace header
    const [org, repo] = orgRepo.split("/");

    if (!org || !repo) {
      throw new Error("Repository must be in format 'org/repo'");
    }

    const url = `${BASE_URL}/v1/git/checkout/${org}/${repo}${branch ? `?branch=${encodeURIComponent(branch)}` : ""}`;
    const response = await fetch(url, {
      method: "POST",
      headers,
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to create workspace: ${errorText}`);
    }

    const result = await response.json();
    return result.worktree; // CheckoutResponse has worktree field
  }

  async getAuthStatus(): Promise<{ authenticated: boolean; user?: string }> {
    try {
      const headers = await this.getHeaders();
      const response = await fetch(`${BASE_URL}/v1/auth/status`, { headers });

      if (!response.ok) {
        return { authenticated: false };
      }

      return response.json();
    } catch {
      return { authenticated: false };
    }
  }
}

export const api = new CatnipAPI();
