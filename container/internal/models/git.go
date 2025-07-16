package models

import (
	"time"
)

// TitleEntry represents a title with its timestamp
type TitleEntry struct {
	Title     string    `json:"title"`
	Timestamp time.Time `json:"timestamp"`
}

// MergeConflictError represents a merge conflict that occurred during sync or merge operations
type MergeConflictError struct {
	Operation     string   `json:"operation"`      // "sync" or "merge"
	WorktreeName  string   `json:"worktree_name"`  // Name of the worktree
	WorktreePath  string   `json:"worktree_path"`  // Path to the worktree
	ConflictFiles []string `json:"conflict_files"` // List of files with conflicts
	Message       string   `json:"message"`        // Human-readable error message
}

func (e *MergeConflictError) Error() string {
	return e.Message
}

// Repository represents a Git repository
// @Description Git repository information and metadata
type Repository struct {
	// Repository identifier in owner/repo format
	ID string `json:"id" example:"anthropics/claude-code"`
	// Full GitHub repository URL
	URL string `json:"url" example:"https://github.com/anthropics/claude-code"`
	// Local path to the bare repository
	Path string `json:"path" example:"/workspace/repos/anthropics_claude-code.git"`
	// Default branch name for this repository
	DefaultBranch string `json:"default_branch" example:"main"`
	// When this repository was first cloned
	CreatedAt time.Time `json:"created_at" example:"2024-01-15T10:30:00Z"`
	// When this repository was last accessed
	LastAccessed time.Time `json:"last_accessed" example:"2024-01-15T16:45:30Z"`
	// Repository description
	Description string `json:"description" example:"AI coding assistant"`
}

// Worktree represents a Git worktree
// @Description Git worktree with branch and status information
type Worktree struct {
	// Unique identifier for this worktree
	ID string `json:"id" example:"abc123-def456-ghi789"`
	// Repository this worktree belongs to
	RepoID string `json:"repo_id" example:"anthropics/claude-code"`
	// User-friendly name for this worktree (e.g., 'vectorize-quasar')
	Name string `json:"name" example:"feature-api-docs"`
	// Absolute path to the worktree directory
	Path string `json:"path" example:"/workspace/worktrees/feature-api-docs"`
	// Current git branch name in this worktree
	Branch string `json:"branch" example:"feature/api-docs"`
	// Branch this worktree was originally created from
	SourceBranch string `json:"source_branch" example:"main"`
	// Commit hash where this worktree diverged from source branch (updated after merges)
	CommitHash string `json:"commit_hash" example:"abc123def456"`
	// Number of commits ahead of the divergence point (CommitHash)
	CommitCount int `json:"commit_count" example:"3"`
	// Number of commits the source branch is ahead of our divergence point
	CommitsBehind int `json:"commits_behind" example:"2"`
	// Whether there are uncommitted changes in the worktree
	IsDirty bool `json:"is_dirty" example:"true"`
	// When this worktree was created
	CreatedAt time.Time `json:"created_at" example:"2024-01-15T14:00:00Z"`
	// When this worktree was last accessed
	LastAccessed time.Time `json:"last_accessed" example:"2024-01-15T16:30:00Z"`
	// Current session title (from terminal title escape sequences)
	SessionTitle *TitleEntry `json:"session_title,omitempty"`
	// History of session titles
	SessionTitleHistory []TitleEntry `json:"session_title_history,omitempty"`
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
	Repositories map[string]*Repository `json:"repositories"`
	// Total number of worktrees across all repositories
	WorktreeCount int `json:"worktree_count" example:"3"`
}

// PullRequestResponse represents the response from creating a pull request
// @Description Response containing pull request information after creation
type PullRequestResponse struct {
	// Pull request number
	Number int `json:"number" example:"123"`
	// URL to the pull request
	URL string `json:"url" example:"https://github.com/owner/repo/pull/123"`
	// Title of the pull request
	Title string `json:"title" example:"Feature: Add new functionality"`
	// Body/description of the pull request
	Body string `json:"body" example:"This PR adds new functionality to the system"`
	// Head branch (source branch of the PR)
	HeadBranch string `json:"head_branch" example:"feature/new-feature"`
	// Base branch (target branch of the PR)
	BaseBranch string `json:"base_branch" example:"main"`
	// Repository in owner/repo format
	Repository string `json:"repository" example:"owner/repo"`
}

// PullRequestInfo represents information about an existing pull request
// @Description Information about an existing pull request for a worktree
type PullRequestInfo struct {
	// Whether the branch has commits ahead of the base branch
	HasCommitsAhead bool `json:"has_commits_ahead" example:"true"`
	// Whether a pull request exists for this branch
	Exists bool `json:"exists" example:"true"`
	// Title of the existing pull request (if exists)
	Title string `json:"title,omitempty" example:"Feature: Add new functionality"`
	// Body/description of the existing pull request (if exists)
	Body string `json:"body,omitempty" example:"This PR adds new functionality"`
	// Pull request number (if exists)
	Number int `json:"number,omitempty" example:"123"`
	// URL to the pull request (if exists)
	URL string `json:"url,omitempty" example:"https://github.com/owner/repo/pull/123"`
}
