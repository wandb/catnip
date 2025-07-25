export interface PortOpenedEvent {
  type: "port:opened";
  payload: {
    port: number;
    service?: string;
    protocol?: "http" | "tcp";
    title?: string;
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
  };
}

export interface HeartbeatEvent {
  type: "heartbeat";
  payload: {
    timestamp: number;
    uptime: number;
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

export type AppEvent =
  | PortOpenedEvent
  | PortClosedEvent
  | GitDirtyEvent
  | GitCleanEvent
  | ProcessStartedEvent
  | ProcessStoppedEvent
  | ContainerStatusEvent
  | HeartbeatEvent
  | WorktreeStatusUpdatedEvent
  | WorktreeBatchUpdatedEvent
  | WorktreeDirtyEvent
  | WorktreeCleanEvent;

export interface SSEMessage {
  event: AppEvent;
  timestamp: number;
  id: string;
}
