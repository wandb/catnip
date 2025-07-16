import { create } from "zustand";
import { subscribeWithSelector } from "zustand/middleware";
import type { AppEvent, SSEMessage } from "../types/events";

interface Port {
  port: number;
  service?: string;
  protocol?: "http" | "tcp";
  title?: string;
  timestamp: number;
}

interface GitWorkspace {
  workspace: string;
  isDirty: boolean;
  dirtyFiles: string[];
  lastUpdated: number;
}

interface Process {
  pid: number;
  command: string;
  workspace?: string;
  startTime: number;
}

interface AppState {
  // Connection state
  sseConnected: boolean;
  sseError: string | null;
  lastEventId: string | null;

  // Application state
  ports: Map<number, Port>;
  gitWorkspaces: Map<string, GitWorkspace>;
  processes: Map<number, Process>;
  containerStatus: "running" | "stopped" | "error";
  containerMessage?: string;

  // Actions
  connectSSE: () => void;
  disconnectSSE: () => void;
  handleEvent: (event: AppEvent) => void;

  // Getters
  getActivePorts: () => Port[];
  getDirtyWorkspaces: () => GitWorkspace[];
  getRunningProcesses: () => Process[];
}

let eventSource: EventSource | null = null;

export const useAppStore = create<AppState>()(
  subscribeWithSelector((set, get) => ({
    // Initial state
    sseConnected: false,
    sseError: null,
    lastEventId: null,
    ports: new Map(),
    gitWorkspaces: new Map(),
    processes: new Map(),
    containerStatus: "stopped",

    connectSSE: () => {
      if (eventSource) {
        eventSource.close();
      }

      const url = "/v1/events";
      console.log("Connecting to SSE:", url);
      eventSource = new EventSource(url);

      eventSource.onopen = () => {
        set({ sseConnected: true, sseError: null });
        console.log("SSE connected successfully");
      };

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);

          // Handle different message formats
          if (data.type === "connection") {
            // Connection message from server
            console.log("SSE connection confirmed:", data.payload?.clientId);
            return;
          }

          const message: SSEMessage = data;

          // Skip empty or invalid events
          if (!message.event || !message.event.type) {
            console.warn("Received invalid SSE message:", message);
            return;
          }

          set({ lastEventId: message.id });
          get().handleEvent(message.event);
          console.log("SSE message received:", message.event.type);
        } catch (error) {
          console.error(
            "Failed to parse SSE message:",
            error,
            "Raw data:",
            event.data,
          );
        }
      };

      eventSource.onerror = (error) => {
        console.error("SSE error:", error);
        console.log("SSE readyState:", eventSource?.readyState);
        set({
          sseConnected: false,
          sseError: "Connection lost. Attempting to reconnect...",
        });

        // Auto-reconnect after 3 seconds
        setTimeout(() => {
          const currentState = get();
          if (!currentState.sseConnected) {
            console.log("Attempting to reconnect SSE...");
            currentState.connectSSE();
          }
        }, 3000);
      };
    },

    disconnectSSE: () => {
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      set({ sseConnected: false, sseError: null });
    },

    handleEvent: (event: AppEvent) => {
      const { ports, gitWorkspaces, processes } = get();

      switch (event.type) {
        case "port:opened": {
          const newPorts = new Map(ports);
          newPorts.set(event.payload.port, {
            port: event.payload.port,
            service: event.payload.service,
            protocol: event.payload.protocol,
            title: event.payload.title,
            timestamp: Date.now(),
          });
          set({ ports: newPorts });
          break;
        }

        case "port:closed": {
          const updatedPorts = new Map(ports);
          updatedPorts.delete(event.payload.port);
          set({ ports: updatedPorts });
          break;
        }

        case "git:dirty": {
          const newGitWorkspaces = new Map(gitWorkspaces);
          newGitWorkspaces.set(event.payload.workspace, {
            workspace: event.payload.workspace,
            isDirty: true,
            dirtyFiles: event.payload.files,
            lastUpdated: Date.now(),
          });
          set({ gitWorkspaces: newGitWorkspaces });
          break;
        }

        case "git:clean": {
          const cleanGitWorkspaces = new Map(gitWorkspaces);
          const workspace = cleanGitWorkspaces.get(event.payload.workspace);
          if (workspace) {
            cleanGitWorkspaces.set(event.payload.workspace, {
              ...workspace,
              isDirty: false,
              dirtyFiles: [],
              lastUpdated: Date.now(),
            });
          }
          set({ gitWorkspaces: cleanGitWorkspaces });
          break;
        }

        case "process:started": {
          const newProcesses = new Map(processes);
          newProcesses.set(event.payload.pid, {
            pid: event.payload.pid,
            command: event.payload.command,
            workspace: event.payload.workspace,
            startTime: Date.now(),
          });
          set({ processes: newProcesses });
          break;
        }

        case "process:stopped": {
          const updatedProcesses = new Map(processes);
          updatedProcesses.delete(event.payload.pid);
          set({ processes: updatedProcesses });
          break;
        }

        case "container:status":
          set({
            containerStatus: event.payload.status,
            containerMessage: event.payload.message,
          });
          break;

        case "heartbeat":
          // Heartbeat keeps connection alive, no state update needed
          break;
      }
    },

    // Getters
    getActivePorts: () => Array.from(get().ports.values()),
    getDirtyWorkspaces: () =>
      Array.from(get().gitWorkspaces.values()).filter((w) => w.isDirty),
    getRunningProcesses: () => Array.from(get().processes.values()),
  })),
);

// Auto-connect on store creation
useAppStore.getState().connectSSE();

// Cleanup on page unload
if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", () => {
    useAppStore.getState().disconnectSSE();
  });
}
