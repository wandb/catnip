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
        }, 120000);

        eventSource.addEventListener("open", () => {
          console.log("🐱 SSE connection opened");
        });

        // Handle specific event types that your server sends
        eventSource.addEventListener("status", (event) => {
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

        eventSource.addEventListener("success", async (event) => {
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

        eventSource.addEventListener("error", (event) => {
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

        eventSource.addEventListener("setup", (event) => {
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

        eventSource.addEventListener("multiple", (event) => {
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
        eventSource.addEventListener("message", (event) => {
          if (isResolved) return;
          console.log("🐱 Generic message received:", event.data);

          try {
            const data = JSON.parse(event.data);
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
        eventSource.onerror = (error) => {
          if (isResolved) return;
          console.error("🐱 SSE connection error:", error);

          const errorMessage = error.message || "SSE connection failed";
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

  async getWorkspaces(): Promise<WorkspaceInfo[]> {
    try {
      const headers = await this.getHeaders(true); // Include codespace header
      const response = await fetch(`${BASE_URL}/v1/git/worktrees`, { headers });

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
        return [];
      }

      try {
        const parsed = JSON.parse(responseText);
        console.log(
          "🐱 Loaded",
          Array.isArray(parsed) ? parsed.length : 0,
          "workspaces",
        );
        return Array.isArray(parsed) ? parsed : [];
      } catch (parseError) {
        console.error("🐱 Error parsing workspaces JSON:", parseError);
        throw new Error("Invalid JSON response from workspaces endpoint");
      }
    } catch (error) {
      console.error("🐱 Error in getWorkspaces:", error);
      throw error;
    }
  }

  async getWorkspace(id: string): Promise<WorkspaceInfo> {
    const headers = await this.getHeaders(true); // Include codespace header
    const encodedId = encodeURIComponent(id);
    const url = `${BASE_URL}/v1/git/worktrees/${encodedId}`;

    console.log("🐱 Fetching workspace:", { id, encodedId, url });
    console.log("🐱 Request headers:", headers);

    const response = await fetch(url, { headers });

    console.log("🐱 Workspace response:", {
      status: response.status,
      statusText: response.statusText,
      headers: Object.fromEntries(response.headers.entries()),
    });

    if (!response.ok) {
      const responseText = await response.text();
      console.error(
        "🐱 Failed to fetch workspace:",
        response.status,
        responseText,
      );
      throw new Error(
        `Failed to fetch workspace (${response.status}): ${responseText}`,
      );
    }

    const responseText = await response.text();
    console.log("🐱 Raw workspace response:", responseText.substring(0, 200));

    try {
      return JSON.parse(responseText);
    } catch (parseError) {
      console.error("🐱 Failed to parse workspace JSON:", parseError);
      console.error("🐱 Response text:", responseText);
      throw new Error(
        `Invalid JSON response: ${responseText.substring(0, 100)}`,
      );
    }
  }

  async sendPrompt(workspacePath: string, prompt: string): Promise<void> {
    const headers = await this.getHeaders(true); // Include codespace header
    const response = await fetch(`${BASE_URL}/v1/claude/messages`, {
      method: "POST",
      headers,
      body: JSON.stringify({
        prompt: prompt,
        working_directory: workspacePath,
      }),
    });

    if (!response.ok) {
      throw new Error("Failed to send prompt");
    }
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
