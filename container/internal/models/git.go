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
type Repository struct {
	ID          string    `json:"id"`          // e.g., "owner/repo"
	URL         string    `json:"url"`         // Full GitHub URL
	Path        string    `json:"path"`        // Path to bare repo
	DefaultBranch string  `json:"default_branch"`
	CreatedAt   time.Time `json:"created_at"`
	LastAccessed time.Time `json:"last_accessed"`
}

// Worktree represents a Git worktree
type Worktree struct {
	ID           string    `json:"id"`            // UUID
	RepoID       string    `json:"repo_id"`       // Reference to Repository.ID
	Name         string    `json:"name"`          // User-friendly name
	Path         string    `json:"path"`          // Absolute path to worktree
	Branch       string    `json:"branch"`        // Git branch name
	SourceBranch string    `json:"source_branch"` // Branch this worktree was created from
	CommitHash    string    `json:"commit_hash"`     // Current commit
	CommitCount   int       `json:"commit_count"`   // Commits made since creation
	CommitsBehind int       `json:"commits_behind"` // Commits behind source branch
	IsDirty       bool      `json:"is_dirty"`       // Has uncommitted changes
	CreatedAt    time.Time `json:"created_at"`
	LastAccessed time.Time `json:"last_accessed"`
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
type GitStatus struct {
	Repository      *Repository            `json:"repository"`       // Repository of active worktree (for backward compatibility)
	Repositories    map[string]*Repository `json:"repositories"`     // All loaded repositories
	ActiveWorktree  *Worktree              `json:"active_worktree"`
	WorktreeCount   int                    `json:"worktree_count"`
}