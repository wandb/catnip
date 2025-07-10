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
	ID          string    `json:"id" example:"anthropics/claude-code" description:"Repository identifier in owner/repo format"`
	URL         string    `json:"url" example:"https://github.com/anthropics/claude-code" description:"Full GitHub repository URL"`
	Path        string    `json:"path" example:"/workspace/repos/anthropics_claude-code.git" description:"Local path to the bare repository"`
	DefaultBranch string  `json:"default_branch" example:"main" description:"Default branch name for this repository"`
	CreatedAt   time.Time `json:"created_at" example:"2024-01-15T10:30:00Z" description:"When this repository was first cloned"`
	LastAccessed time.Time `json:"last_accessed" example:"2024-01-15T16:45:30Z" description:"When this repository was last accessed"`
}

// Worktree represents a Git worktree
// @Description Git worktree with branch and status information
type Worktree struct {
	ID           string    `json:"id" example:"abc123-def456-ghi789" description:"Unique identifier for this worktree"`
	RepoID       string    `json:"repo_id" example:"anthropics/claude-code" description:"Repository this worktree belongs to"`
	Name         string    `json:"name" example:"feature-api-docs" description:"User-friendly name for this worktree"`
	Path         string    `json:"path" example:"/workspace/worktrees/feature-api-docs" description:"Absolute path to the worktree directory"`
	Branch       string    `json:"branch" example:"feature/api-docs" description:"Current git branch name"`
	SourceBranch string    `json:"source_branch" example:"main" description:"Branch this worktree was created from"`
	CommitHash    string    `json:"commit_hash" example:"abc123def456" description:"Current commit hash"`
	CommitCount   int       `json:"commit_count" example:"3" description:"Number of commits made since worktree creation"`
	CommitsBehind int       `json:"commits_behind" example:"2" description:"Number of commits this branch is behind the source branch"`
	IsDirty       bool      `json:"is_dirty" example:"true" description:"Whether there are uncommitted changes"`
	CreatedAt    time.Time `json:"created_at" example:"2024-01-15T14:00:00Z" description:"When this worktree was created"`
	LastAccessed time.Time `json:"last_accessed" example:"2024-01-15T16:30:00Z" description:"When this worktree was last accessed"`
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
	Repositories    map[string]*Repository `json:"repositories" description:"All loaded repositories mapped by repository ID"`
	WorktreeCount   int                    `json:"worktree_count" example:"3" description:"Total number of worktrees across all repositories"`
}