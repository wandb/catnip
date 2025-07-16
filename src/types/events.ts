export interface PortOpenedEvent {
  type: 'port:opened';
  payload: {
    port: number;
    service?: string;
    protocol?: 'http' | 'tcp';
    title?: string;
  };
}

export interface PortClosedEvent {
  type: 'port:closed';
  payload: {
    port: number;
  };
}

export interface GitDirtyEvent {
  type: 'git:dirty';
  payload: {
    workspace: string;
    files: string[];
  };
}

export interface GitCleanEvent {
  type: 'git:clean';
  payload: {
    workspace: string;
  };
}

export interface ProcessStartedEvent {
  type: 'process:started';
  payload: {
    pid: number;
    command: string;
    workspace?: string;
  };
}

export interface ProcessStoppedEvent {
  type: 'process:stopped';
  payload: {
    pid: number;
    exitCode: number;
  };
}

export interface ContainerStatusEvent {
  type: 'container:status';
  payload: {
    status: 'running' | 'stopped' | 'error';
    message?: string;
  };
}

export interface HeartbeatEvent {
  type: 'heartbeat';
  payload: {
    timestamp: number;
    uptime: number;
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
  | HeartbeatEvent;

export interface SSEMessage {
  event: AppEvent;
  timestamp: number;
  id: string;
}