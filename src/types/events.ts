export interface PortOpenedEvent {
  type: "port:opened";
  payload: {
    port: number;
    service?: string;
    protocol?: "http" | "tcp";
    title?: string;
    working_dir?: string;
  };
}

export interface PortClosedEvent {
  type: "port:closed";
  payload: {
    port: number;
  };
}

export interface GitDirtyEvent {
  type: "git:dirty";
  payload: {
    workspace: string;
    files: string[];
  };
}

export interface GitCleanEvent {
  type: "git:clean";
  payload: {
    workspace: string;
  };
}

export interface ProcessStartedEvent {
  type: "process:started";
  payload: {
    pid: number;
    command: string;
    workspace?: string;
  };
}

export interface ProcessStoppedEvent {
  type: "process:stopped";
  payload: {
    pid: number;
    exitCode: number;
  };
}

export interface ContainerStatusEvent {
  type: "container:status";
  payload: {
    status: "running" | "stopped" | "error";
    message?: string;
    sshEnabled: boolean;
  };
}

export interface HeartbeatEvent {
  type: "heartbeat";
  payload: {
    timestamp: number;
    uptime: number;
  };
}

export interface PortMappedEvent {
  type: "port:mapped";
  payload: {
    port: number;
    host_port: number; // 0 means cleared
  };
}

export interface WorktreeStatusUpdatedEvent {
  type: "worktree:status_updated";
  payload: {
    worktree_id: string;
    status: {
      is_dirty: boolean;
      commit_count: number;
      commits_behind: number;
      has_conflicts: boolean;
      is_cached: boolean;
      is_loading: boolean;
      last_updated: number;
    };
  };
}

export interface WorktreeBatchUpdatedEvent {
  type: "worktree:batch_updated";
  payload: {
    updates: Record<
      string,
      {
        is_dirty: boolean;
        commit_count: number;
        commits_behind: number;
        has_conflicts: boolean;
        is_cached: boolean;
        is_loading: boolean;
        last_updated: number;
      }
    >;
  };
}

export interface WorktreeDirtyEvent {
  type: "worktree:dirty";
  payload: {
    worktree_id: string;
    files: string[];
  };
}

export interface WorktreeCleanEvent {
  type: "worktree:clean";
  payload: {
    worktree_id: string;
  };
}

export interface WorktreeUpdatedEvent {
  type: "worktree:updated";
  payload: {
    worktree_id: string;
    updates: Record<string, any>;
  };
}

export interface WorktreeCreatedEvent {
  type: "worktree:created";
  payload: {
    worktree: any; // Will match the Worktree interface
  };
}

export interface WorktreeDeletedEvent {
  type: "worktree:deleted";
  payload: {
    worktree_id: string;
    worktree_name: string;
  };
}

export interface WorktreeTodosUpdatedEvent {
  type: "worktree:todos_updated";
  payload: {
    worktree_id: string;
    todos: {
      id: string;
      content: string;
      status: "pending" | "in_progress" | "completed";
      priority: "high" | "medium" | "low";
    }[];
  };
}

export interface SessionTitleUpdatedEvent {
  type: "session:title_updated";
  payload: {
    workspace_dir: string;
    worktree_id?: string;
    session_title: {
      title: string;
      timestamp: string;
      commit_hash: string;
    };
    session_title_history: {
      title: string;
      timestamp: string;
      commit_hash: string;
    }[];
  };
}

export interface SessionStoppedEvent {
  type: "session:stopped";
  payload: {
    workspace_dir: string;
    worktree_id?: string;
    session_title?: string;
    branch_name?: string;
    last_todo?: string;
  };
}

export interface NotificationEvent {
  type: "notification:show";
  payload: {
    title: string;
    body: string;
    subtitle?: string;
  };
}

export type AppEvent =
  | PortOpenedEvent
  | PortClosedEvent
  | GitDirtyEvent
  | GitCleanEvent
  | ProcessStartedEvent
  | ProcessStoppedEvent
  | ContainerStatusEvent
  | PortMappedEvent
  | HeartbeatEvent
  | WorktreeStatusUpdatedEvent
  | WorktreeBatchUpdatedEvent
  | WorktreeDirtyEvent
  | WorktreeCleanEvent
  | WorktreeUpdatedEvent
  | WorktreeCreatedEvent
  | WorktreeDeletedEvent
  | WorktreeTodosUpdatedEvent
  | SessionTitleUpdatedEvent
  | SessionStoppedEvent
  | NotificationEvent;

export interface SSEMessage {
  event: AppEvent;
  timestamp: number;
  id: string;
}
