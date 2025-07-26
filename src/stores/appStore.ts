import { create } from "zustand";
import { subscribeWithSelector } from "zustand/middleware";
import type { AppEvent, SSEMessage } from "../types/events";
import type { Worktree } from "../lib/git-api";

interface Port {
  port: number;
  service?: string;
  protocol?: "http" | "tcp";
  title?: string;
  timestamp: number;
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
  worktrees: Map<string, Worktree>;
  processes: Map<number, Process>;
  containerStatus: "running" | "stopped" | "error";
  containerMessage?: string;
  sshEnabled: boolean;

  // Actions
  connectSSE: () => void;
  disconnectSSE: () => void;
  handleEvent: (event: AppEvent) => void;
  setWorktrees: (worktrees: Worktree[]) => void;
  updateWorktree: (worktreeId: string, updates: Partial<Worktree>) => void;

  // Getters
  getActivePorts: () => Port[];
  getDirtyWorktrees: () => Worktree[];
  getRunningProcesses: () => Process[];
  getWorktreesList: () => Worktree[];
  getWorktreeById: (id: string) => Worktree | undefined;
}

let eventSource: EventSource | null = null;

export const useAppStore = create<AppState>()(
  subscribeWithSelector((set, get) => ({
    // Initial state
    sseConnected: false,
    sseError: null,
    lastEventId: null,
    ports: new Map(),
    worktrees: new Map(),
    processes: new Map(),
    containerStatus: "stopped",
    sshEnabled: false,

    connectSSE: () => {
      // Prevent multiple simultaneous connections
      if (eventSource && eventSource.readyState === EventSource.CONNECTING) {
        return;
      }

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

        // Auto-reconnect after 3 seconds, but only if not already connected
        setTimeout(() => {
          const currentState = get();
          if (
            !currentState.sseConnected &&
            (!eventSource || eventSource.readyState === EventSource.CLOSED)
          ) {
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
      const { ports, worktrees, processes } = get();

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
          const updatedWorktrees = new Map(worktrees);
          // Find worktree by path/workspace name
          const worktreeEntry = Array.from(worktrees.entries()).find(
            ([_, worktree]) =>
              worktree.path === event.payload.workspace ||
              worktree.name === event.payload.workspace,
          );
          if (worktreeEntry) {
            const [worktreeId, worktree] = worktreeEntry;
            updatedWorktrees.set(worktreeId, {
              ...worktree,
              is_dirty: true,
              dirty_files: event.payload.files,
            });
            set({ worktrees: updatedWorktrees });
          }
          break;
        }

        case "git:clean": {
          const updatedWorktrees = new Map(worktrees);
          // Find worktree by path/workspace name
          const worktreeEntry = Array.from(worktrees.entries()).find(
            ([_, worktree]) =>
              worktree.path === event.payload.workspace ||
              worktree.name === event.payload.workspace,
          );
          if (worktreeEntry) {
            const [worktreeId, worktree] = worktreeEntry;
            updatedWorktrees.set(worktreeId, {
              ...worktree,
              is_dirty: false,
              dirty_files: [],
            });
            set({ worktrees: updatedWorktrees });
          }
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
            sshEnabled: event.payload.sshEnabled || false,
          });
          break;

        case "heartbeat":
          // Heartbeat keeps connection alive, no state update needed
          break;

        case "worktree:status_updated": {
          const updatedWorktrees = new Map(worktrees);
          const existingWorktree = updatedWorktrees.get(
            event.payload.worktree_id,
          );
          if (existingWorktree) {
            // Merge cache status with existing worktree data
            updatedWorktrees.set(event.payload.worktree_id, {
              ...existingWorktree,
              is_dirty: event.payload.status.is_dirty,
              commit_count: event.payload.status.commit_count,
              commits_behind: event.payload.status.commits_behind,
              has_conflicts: event.payload.status.has_conflicts,
              cache_status: {
                is_cached: event.payload.status.is_cached,
                is_loading: event.payload.status.is_loading,
                last_updated: event.payload.status.last_updated,
              },
            });
            set({ worktrees: updatedWorktrees });
          }
          break;
        }

        case "worktree:batch_updated": {
          const updatedWorktrees = new Map(worktrees);
          for (const [worktreeId, status] of Object.entries(
            event.payload.updates,
          )) {
            const existingWorktree = updatedWorktrees.get(worktreeId);
            if (existingWorktree) {
              // Apply cached status updates
              updatedWorktrees.set(worktreeId, {
                ...existingWorktree,
                is_dirty: status.is_dirty,
                commit_count: status.commit_count,
                commits_behind: status.commits_behind,
                has_conflicts: status.has_conflicts,
                cache_status: {
                  is_cached: status.is_cached,
                  is_loading: status.is_loading,
                  last_updated: status.last_updated,
                },
              });
            }
          }
          set({ worktrees: updatedWorktrees });
          break;
        }

        case "worktree:dirty": {
          const updatedWorktrees = new Map(worktrees);
          const existingWorktree = updatedWorktrees.get(
            event.payload.worktree_id,
          );
          if (existingWorktree) {
            updatedWorktrees.set(event.payload.worktree_id, {
              ...existingWorktree,
              is_dirty: true,
              dirty_files: event.payload.files,
            });
            set({ worktrees: updatedWorktrees });
          }
          break;
        }

        case "worktree:clean": {
          const updatedWorktrees = new Map(worktrees);
          const existingWorktree = updatedWorktrees.get(
            event.payload.worktree_id,
          );
          if (existingWorktree) {
            updatedWorktrees.set(event.payload.worktree_id, {
              ...existingWorktree,
              is_dirty: false,
              dirty_files: [],
            });
            set({ worktrees: updatedWorktrees });
          }
          break;
        }
      }
    },

    setWorktrees: (worktrees: Worktree[]) => {
      const worktreeMap = new Map<string, Worktree>();
      worktrees.forEach((worktree) => {
        worktreeMap.set(worktree.id, worktree);
      });
      set({ worktrees: worktreeMap });
    },

    updateWorktree: (worktreeId: string, updates: Partial<Worktree>) => {
      const { worktrees } = get();
      const existingWorktree = worktrees.get(worktreeId);
      if (existingWorktree) {
        const updatedWorktrees = new Map(worktrees);
        updatedWorktrees.set(worktreeId, { ...existingWorktree, ...updates });
        set({ worktrees: updatedWorktrees });
      }
    },

    // Getters
    getActivePorts: () => Array.from(get().ports.values()),
    getDirtyWorktrees: () =>
      Array.from(get().worktrees.values()).filter((w) => w.is_dirty),
    getRunningProcesses: () => Array.from(get().processes.values()),
    getWorktreesList: () => Array.from(get().worktrees.values()),
    getWorktreeById: (id: string) => get().worktrees.get(id),
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
