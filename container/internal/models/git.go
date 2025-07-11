package models

import (
	"time"
)

// MergeConflictError represents a merge conflict that occurred during sync or merge operations
type MergeConflictError struct {
	Operation    string   `json:"operation"`     // "sync" or "merge"
	WorktreeName string   `json:"worktree_name"` // Name of the worktree
	WorktreePath string   `json:"worktree_path"` // Path to the worktree
	ConflictFiles []string `json:"conflict_files"` // List of files with conflicts
	Message      string   `json:"message"`       // Human-readable error message
}

func (e *MergeConflictError) Error() string {
	return e.Message
}

// Repository represents a Git repository
// @Description Git repository information and metadata
type Repository struct {
	// Repository identifier in owner/repo format
	ID          string    `json:"id" example:"anthropics/claude-code"`
	// Full GitHub repository URL
	URL         string    `json:"url" example:"https://github.com/anthropics/claude-code"`
	// Local path to the bare repository
	Path        string    `json:"path" example:"/workspace/repos/anthropics_claude-code.git"`
	// Default branch name for this repository
	DefaultBranch string  `json:"default_branch" example:"main"`
	// When this repository was first cloned
	CreatedAt   time.Time `json:"created_at" example:"2024-01-15T10:30:00Z"`
	// When this repository was last accessed
	LastAccessed time.Time `json:"last_accessed" example:"2024-01-15T16:45:30Z"`
}

// Worktree represents a Git worktree
// @Description Git worktree with branch and status information
type Worktree struct {
	// Unique identifier for this worktree
	ID           string    `json:"id" example:"abc123-def456-ghi789"`
	// Repository this worktree belongs to
	RepoID       string    `json:"repo_id" example:"anthropics/claude-code"`
	// User-friendly name for this worktree (e.g., 'vectorize-quasar')
	Name         string    `json:"name" example:"feature-api-docs"`
	// Absolute path to the worktree directory
	Path         string    `json:"path" example:"/workspace/worktrees/feature-api-docs"`
	// Current git branch name in this worktree
	Branch       string    `json:"branch" example:"feature/api-docs"`
	// Branch this worktree was originally created from
	SourceBranch string    `json:"source_branch" example:"main"`
	// Commit hash where this worktree diverged from source branch (updated after merges)
	CommitHash    string    `json:"commit_hash" example:"abc123def456"`
	// Number of commits ahead of the divergence point (CommitHash)
	CommitCount   int       `json:"commit_count" example:"3"`
	// Number of commits the source branch is ahead of our divergence point
	CommitsBehind int       `json:"commits_behind" example:"2"`
	// Whether there are uncommitted changes in the worktree
	IsDirty       bool      `json:"is_dirty" example:"true"`
	// When this worktree was created
	CreatedAt    time.Time `json:"created_at" example:"2024-01-15T14:00:00Z"`
	// When this worktree was last accessed
	LastAccessed time.Time `json:"last_accessed" example:"2024-01-15T16:30:00Z"`
}

// WorktreeCreateRequest represents a request to create a new worktree
type WorktreeCreateRequest struct {
	Source string `json:"source"` // Branch name or commit hash
	Name   string `json:"name"`   // User-friendly name
}

// CheckoutRequest represents a request to checkout a repository
type CheckoutRequest struct {
	Org    string `json:"org"`
	Repo   string `json:"repo"`
	Branch string `json:"branch,omitempty"`
}

// GitStatus represents the current Git status
// @Description Current git status including repository information
type GitStatus struct {
	// All loaded repositories mapped by repository ID
	Repositories    map[string]*Repository `json:"repositories"`
	// Total number of worktrees across all repositories
	WorktreeCount   int                    `json:"worktree_count" example:"3"`
}