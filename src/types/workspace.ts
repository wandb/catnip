export interface Worktree {
  id: string;
  name: string;
  repo_id: string;
  last_accessed: string;
}

export interface Repository {
  id: string;
  name?: string;
  available?: boolean;
}

export interface RepositoryWithWorktrees extends Repository {
  worktrees: Worktree[];
  projectName: string;
  kittyCount: number;
  lastActivity: string;
}

export interface WorkspaceParams {
  project: string;
  workspace: string;
}

export interface RouteParams {
  project?: string;
  workspace?: string;
}

// Error boundary types
export interface ErrorFallbackProps {
  error: Error;
  resetErrorBoundary: () => void;
}

// Dialog state types
export interface UnavailableRepoAlert {
  open: boolean;
  repoName: string;
  repoId: string;
  worktrees: Worktree[];
}

export interface DeleteConfirmDialog {
  open: boolean;
  worktrees: Worktree[];
  repoName: string;
}

export interface SingleWorkspaceDeleteDialog {
  open: boolean;
  worktreeId: string;
  worktreeName: string;
  hasChanges: boolean;
  commitCount: number;
}