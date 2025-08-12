import { create } from "zustand";
import { subscribeWithSelector } from "zustand/middleware";
import type { AppEvent, SSEMessage } from "../types/events";
import type {
  Worktree,
  GitStatus,
  Repository,
  LocalRepository,
} from "../lib/git-api";
import { gitApi } from "../lib/git-api";
import { useNotifications } from "../lib/useNotifications";

interface Port {
  port: number;
  service?: string;
  protocol?: "http" | "tcp";
  title?: string;
  workingDir?: string;
  timestamp: number;
  hostPort?: number; // mapped host port if forwarded via CLI
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
  repositories: Map<string, LocalRepository>;
  githubRepositories: Repository[];
  gitStatus: GitStatus;
  containerStatus: "running" | "stopped" | "error";
  containerMessage?: string;
  sshEnabled: boolean;

  // Notifications
  notifications: ReturnType<typeof useNotifications> | null;
  setNotifications: (
    notifications: ReturnType<typeof useNotifications>,
  ) => void;

  // Loading states
  initialLoading: boolean;
  worktreesLoading: boolean;
  repositoriesLoading: boolean;
  gitStatusLoading: boolean;
  loadError: string | null;

  // Actions
  connectSSE: () => void;
  disconnectSSE: () => void;
  handleEvent: (event: AppEvent) => void;
  loadInitialData: () => Promise<void>;
  refreshData: () => Promise<void>;

  // Worktree actions
  setWorktrees: (worktrees: Worktree[]) => void;
  updateWorktree: (worktreeId: string, updates: Partial<Worktree>) => void;
  addWorktree: (worktree: Worktree) => void;
  removeWorktree: (worktreeId: string) => void;

  // Repository actions
  setRepositories: (repositories: Record<string, LocalRepository>) => void;
  setGithubRepositories: (repositories: Repository[]) => void;
  setGitStatus: (status: GitStatus) => void;

  // Getters
  getActivePorts: () => Port[];
  getDirtyWorktrees: () => Worktree[];
  getRunningProcesses: () => Process[];
  getWorktreesList: () => Worktree[];
  getWorktreeById: (id: string) => Worktree | undefined;
  getRepositoriesList: () => LocalRepository[];
  getRepositoryById: (id: string) => LocalRepository | undefined;
  getGithubRepositories: () => Repository[];
  getWorktreesByRepo: (repoId: string) => Worktree[];
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
    repositories: new Map(),
    githubRepositories: [],
    gitStatus: {},
    containerStatus: "stopped",
    sshEnabled: false,

    // Notifications
    notifications: null,
    setNotifications: (notifications) => set({ notifications }),

    // Loading states
    initialLoading: false,
    worktreesLoading: false,
    repositoriesLoading: false,
    gitStatusLoading: false,
    loadError: null,

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
        try {
          console.error("SSE error:", error);
          console.log("SSE readyState:", eventSource?.readyState);

          // Handle different error scenarios gracefully
          const readyState = eventSource?.readyState;
          let errorMessage = "Connection lost. Attempting to reconnect...";

          if (readyState === EventSource.CONNECTING) {
            errorMessage = "Connecting to server...";
          } else if (readyState === EventSource.CLOSED) {
            errorMessage = "Connection closed. Will retry shortly.";
          }

          set({
            sseConnected: false,
            sseError: errorMessage,
          });

          // Auto-reconnect after 3 seconds, but only if not already connected
          // Wrap in try-catch to prevent any reconnection errors from crashing
          setTimeout(() => {
            try {
              const currentState = get();
              if (
                !currentState.sseConnected &&
                (!eventSource || eventSource.readyState === EventSource.CLOSED)
              ) {
                console.log("Attempting SSE reconnection...");
                currentState.connectSSE();
              }
            } catch (reconnectError) {
              console.error("SSE reconnection failed:", reconnectError);
              // Don't crash the app, just log the error
            }
          }, 3000);
        } catch (handleError) {
          console.error("Error in SSE error handler:", handleError);
          // Fallback: just set basic error state without crashing
          set({
            sseConnected: false,
            sseError: "Connection error occurred",
          });
        }
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
            workingDir: event.payload.working_dir,
            timestamp: Date.now(),
            hostPort: newPorts.get(event.payload.port)?.hostPort, // preserve mapping if any
          });
          set({ ports: newPorts });
          break;
        }
        case "port:mapped": {
          const newPorts = new Map(ports);
          const port = event.payload.port as number;
          const hostPort = event.payload.host_port as number;
          const existing = newPorts.get(port) || {
            port,
            timestamp: Date.now(),
          };
          if (hostPort && hostPort > 0) {
            newPorts.set(port, { ...existing, hostPort });
          } else {
            // cleared
            newPorts.set(port, { ...existing, hostPort: undefined });
          }
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
              dirty_files: event.payload.files?.map((file: any) =>
                typeof file === "string" ? { path: file, status: "M" } : file,
              ),
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
              dirty_files: event.payload.files?.map((file: any) =>
                typeof file === "string" ? { path: file, status: "M" } : file,
              ),
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

        case "worktree:updated": {
          const updatedWorktrees = new Map(worktrees);
          const existingWorktree = updatedWorktrees.get(
            event.payload.worktree_id,
          );
          if (existingWorktree) {
            const updatedWorktree = {
              ...existingWorktree,
              ...event.payload.updates,
            };
            updatedWorktrees.set(event.payload.worktree_id, updatedWorktree);
            set({ worktrees: updatedWorktrees });
          } else {
            console.warn(
              `Worktree not found for update: ${event.payload.worktree_id}`,
            );
          }
          break;
        }

        case "worktree:created": {
          const updatedWorktrees = new Map(worktrees);
          const newWorktree = event.payload.worktree;
          updatedWorktrees.set(newWorktree.id, {
            ...newWorktree,
            cache_status: {
              is_cached: true,
              is_loading: false,
              last_updated: Date.now(),
            },
          });
          set({ worktrees: updatedWorktrees });

          // After creating a new worktree, we need to refresh git status to get repository info
          // This is necessary when checking out a new GitHub repository
          const { repositories } = get();
          if (!repositories.has(newWorktree.repo_id)) {
            // Repository not in store, need to fetch git status
            gitApi
              .fetchGitStatus()
              .then((gitStatus) => {
                if (gitStatus.repositories) {
                  get().setGitStatus(gitStatus);
                }
              })
              .catch((error) => {
                console.error(
                  "Failed to fetch git status after worktree creation:",
                  error,
                );
              });
          }
          break;
        }

        case "worktree:deleted": {
          const updatedWorktrees = new Map(worktrees);
          updatedWorktrees.delete(event.payload.worktree_id);
          set({ worktrees: updatedWorktrees });
          break;
        }

        case "worktree:todos_updated": {
          const updatedWorktrees = new Map(worktrees);
          const existingWorktree = updatedWorktrees.get(
            event.payload.worktree_id,
          );
          if (existingWorktree) {
            const newTodos = event.payload.todos?.map((todo: any) => ({
              id: todo.id,
              content: todo.content || todo.text,
              status: todo.status || (todo.completed ? "completed" : "pending"),
              priority: todo.priority,
            }));

            updatedWorktrees.set(event.payload.worktree_id, {
              ...existingWorktree,
              todos: newTodos,
            });
            set({ worktrees: updatedWorktrees });
          } else {
            console.warn(
              `Worktree not found for todos update: ${event.payload.worktree_id}`,
            );
          }
          break;
        }

        case "session:title_updated": {
          const updatedWorktrees = new Map(worktrees);
          // Find worktree by workspace path since we don't have worktreeID
          const worktreeEntry = Array.from(worktrees.entries()).find(
            ([_, worktree]) => worktree.path === event.payload.workspace_dir,
          );
          if (worktreeEntry) {
            const [worktreeId, worktree] = worktreeEntry;
            updatedWorktrees.set(worktreeId, {
              ...worktree,
              session_title: event.payload.session_title,
              session_title_history: event.payload.session_title_history,
            });
            set({ worktrees: updatedWorktrees });
          }
          break;
        }

        case "session:stopped": {
          const { notifications } = get();
          console.log(
            "🔔 session:stopped - notifications object:",
            notifications,
          );
          console.log(
            "🔔 session:stopped - canShowNotifications:",
            notifications?.canShowNotifications,
          );
          if (notifications?.canShowNotifications) {
            // Find the worktree for this session
            const worktreeEntry = Array.from(worktrees.entries()).find(
              ([_, worktree]) => worktree.path === event.payload.workspace_dir,
            );

            if (worktreeEntry) {
              const [_, worktree] = worktreeEntry;
              console.log("🔔 Found worktree for notification:", worktree);

              const title =
                event.payload.session_title ||
                worktree.session_title?.title ||
                "Claude Session";
              const branchName =
                event.payload.branch_name || worktree.branch || "main";
              const lastTodo =
                event.payload.last_todo ||
                (worktree.todos && worktree.todos.length > 0
                  ? worktree.todos[worktree.todos.length - 1].content
                  : "No active todos");

              const notificationTitle = `${title} (${branchName})`;
              const notificationBody = `Session ended - Last todo: ${lastTodo}`;

              console.log("🔔 Attempting to show notification:", {
                title: notificationTitle,
                body: notificationBody,
              });

              try {
                void notifications.showNotification(notificationTitle, {
                  body: notificationBody,
                  icon: "/favicon.png",
                  tag: `session-stopped-${worktree.id}`,
                });
                console.log("🔔 Notification sent successfully!");
              } catch (error) {
                console.error("🔔 Failed to show notification:", error);
              }
            } else {
              console.log(
                "🔔 No worktree found for workspace_dir:",
                event.payload.workspace_dir,
              );
            }
          }
          break;
        }
      }
    },

    // Load initial data from APIs
    loadInitialData: async () => {
      set({ initialLoading: true, loadError: null });
      try {
        // Load data in parallel with proper error handling
        const [worktreesResult, gitStatusResult, githubReposResult] =
          await Promise.allSettled([
            gitApi.fetchWorktrees(),
            gitApi.fetchGitStatus(),
            gitApi.fetchRepositories(),
          ]);

        // Check if all critical requests failed
        if (
          worktreesResult.status === "rejected" &&
          gitStatusResult.status === "rejected"
        ) {
          const errorMessage =
            worktreesResult.reason?.message ||
            "Failed to connect to backend server";
          set({
            loadError: errorMessage,
            initialLoading: false,
          });
          return;
        }

        // Extract successful data with fallbacks
        const worktreesData =
          worktreesResult.status === "fulfilled" ? worktreesResult.value : [];
        const gitStatusData =
          gitStatusResult.status === "fulfilled" ? gitStatusResult.value : {};
        const githubReposData =
          githubReposResult.status === "fulfilled"
            ? githubReposResult.value
            : [];

        // Transform and set worktrees
        const worktreeMap = new Map<string, Worktree>();
        worktreesData.forEach((worktree) => {
          // Add cache status to indicate fresh data
          const enhancedWorktree = {
            ...worktree,
            cache_status: {
              is_cached: true,
              is_loading: false,
              last_updated: Date.now(),
            },
          };
          worktreeMap.set(worktree.id, enhancedWorktree);
        });

        // Transform repositories from git status
        const repositoryMap = new Map<string, LocalRepository>();
        if (
          gitStatusData &&
          typeof gitStatusData === "object" &&
          "repositories" in gitStatusData &&
          gitStatusData.repositories
        ) {
          Object.entries(gitStatusData.repositories).forEach(([id, repo]) => {
            repositoryMap.set(id, repo as LocalRepository);
          });
        }

        set({
          worktrees: worktreeMap,
          repositories: repositoryMap,
          gitStatus: gitStatusData,
          githubRepositories: githubReposData,
          initialLoading: false,
          loadError: null,
        });
      } catch (error: any) {
        console.error("Failed to load initial data:", error);
        set({
          loadError: error?.message || "Failed to load application data",
          initialLoading: false,
        });
      } finally {
        // initialLoading is already set to false above
      }
    },

    // Refresh all data
    refreshData: async () => {
      const state = get();
      await state.loadInitialData();
    },

    // Worktree actions
    setWorktrees: (worktrees: Worktree[]) => {
      const currentWorktrees = get().worktrees;

      // Check if worktrees have actually changed
      const hasChanges =
        currentWorktrees.size !== worktrees.length ||
        worktrees.some((worktree) => {
          const existing = currentWorktrees.get(worktree.id);
          // Compare the worktree data, excluding cache_status timestamps which always change
          const existingForComparison = existing
            ? {
                ...existing,
                cache_status: { ...existing.cache_status, last_updated: 0 },
              }
            : null;
          const newForComparison = {
            ...worktree,
            cache_status: { ...worktree.cache_status, last_updated: 0 },
          };
          return (
            !existing ||
            JSON.stringify(existingForComparison) !==
              JSON.stringify(newForComparison)
          );
        });

      // Only update if there are actual changes
      if (hasChanges) {
        const worktreeMap = new Map<string, Worktree>();
        worktrees.forEach((worktree) => {
          // Ensure cache status is present
          const enhancedWorktree = {
            ...worktree,
            cache_status: worktree.cache_status || {
              is_cached: true,
              is_loading: false,
              last_updated: Date.now(),
            },
          };
          worktreeMap.set(worktree.id, enhancedWorktree);
        });
        set({ worktrees: worktreeMap });
      }
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

    addWorktree: (worktree: Worktree) => {
      const { worktrees } = get();
      const updatedWorktrees = new Map(worktrees);
      // Ensure cache status is present
      const enhancedWorktree = {
        ...worktree,
        cache_status: worktree.cache_status || {
          is_cached: true,
          is_loading: false,
          last_updated: Date.now(),
        },
      };
      updatedWorktrees.set(worktree.id, enhancedWorktree);
      set({ worktrees: updatedWorktrees });
    },

    removeWorktree: (worktreeId: string) => {
      const { worktrees } = get();
      const updatedWorktrees = new Map(worktrees);
      updatedWorktrees.delete(worktreeId);
      set({ worktrees: updatedWorktrees });
    },

    // Repository actions
    setRepositories: (repositories: Record<string, LocalRepository>) => {
      const currentRepositories = get().repositories;
      const newRepositoryEntries = Object.entries(repositories);

      // Check if repositories have actually changed
      const hasChanges =
        currentRepositories.size !== newRepositoryEntries.length ||
        newRepositoryEntries.some(([id, repo]) => {
          const existing = currentRepositories.get(id);
          return !existing || JSON.stringify(existing) !== JSON.stringify(repo);
        });

      // Only update if there are actual changes
      if (hasChanges) {
        const repositoryMap = new Map<string, LocalRepository>();
        newRepositoryEntries.forEach(([id, repo]) => {
          repositoryMap.set(id, repo);
        });
        set({ repositories: repositoryMap });
      }
    },

    setGithubRepositories: (repositories: Repository[]) => {
      const currentRepos = get().githubRepositories;
      // Only update if repositories have actually changed
      if (JSON.stringify(currentRepos) !== JSON.stringify(repositories)) {
        set({ githubRepositories: repositories });
      }
    },

    setGitStatus: (status: GitStatus) => {
      set({ gitStatus: status });
      // Update repositories from git status if present
      if (status.repositories) {
        const currentRepositories = get().repositories;
        const newRepositoryEntries = Object.entries(status.repositories);

        // Check if repositories have actually changed
        const hasChanges =
          currentRepositories.size !== newRepositoryEntries.length ||
          newRepositoryEntries.some(([id, repo]) => {
            const existing = currentRepositories.get(id);
            return (
              !existing || JSON.stringify(existing) !== JSON.stringify(repo)
            );
          });

        // Only update if there are actual changes
        if (hasChanges) {
          const repositoryMap = new Map<string, LocalRepository>();
          newRepositoryEntries.forEach(([id, repo]) => {
            repositoryMap.set(id, repo as LocalRepository);
          });
          set({ repositories: repositoryMap });
        }
      }
    },

    // Getters
    getActivePorts: () => Array.from(get().ports.values()),
    getDirtyWorktrees: () =>
      Array.from(get().worktrees.values()).filter((w) => w.is_dirty),
    getRunningProcesses: () => Array.from(get().processes.values()),
    getWorktreesList: () => Array.from(get().worktrees.values()),
    getWorktreeById: (id: string) => get().worktrees.get(id),
    getRepositoriesList: () => Array.from(get().repositories.values()),
    getRepositoryById: (id: string) => get().repositories.get(id),
    getGithubRepositories: () => get().githubRepositories,
    getWorktreesByRepo: (repoId: string) =>
      Array.from(get().worktrees.values()).filter((w) => w.repo_id === repoId),
  })),
);

// Auto-connect SSE and load initial data on store creation
useAppStore.getState().connectSSE();

// Load initial data after store creation
void useAppStore.getState().loadInitialData();

// Cleanup on page unload
if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", () => {
    useAppStore.getState().disconnectSSE();
  });
}
