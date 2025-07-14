export interface GitStatus {
  repositories?: Record<string, any>;
  worktree_count?: number;
}

export interface Worktree {
  id: string;
  repo_id: string;
  name: string;
  branch: string;
  source_branch: string;
  path: string;
  commit_hash: string;
  commit_count: number;
  commits_behind: number;
  is_dirty: boolean;
}

export interface Repository {
  name: string;
  url: string;
  private: boolean;
  description?: string;
  fullName?: string;
}