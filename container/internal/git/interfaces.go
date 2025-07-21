package git

import (
	"github.com/vanpelt/catnip/internal/models"
)

// Repository represents operations on a Git repository
type Repository interface {
	// Core repository operations
	Clone(url, path string) error
	GetPath() string
	GetRemoteURL() (string, error)
	GetDefaultBranch() (string, error)

	// Branch operations
	ListBranches() ([]string, error)
	BranchExists(branch string) bool
	CreateBranch(branch, from string) error

	// Fetch operations
	Fetch() error
	FetchBranch(branch string) error
	FetchWithDepth(branch string, depth int) error

	// State checks
	IsBare() bool
	IsShallow() bool
	Unshallow() error
}

// Worktree represents operations on a Git worktree
type Worktree interface {
	// Core worktree operations
	GetPath() string
	GetBranch() string
	Checkout(branch string) error

	// Status operations
	Status() (*WorktreeStatus, error)
	IsDirty() bool
	HasConflicts() bool
	GetConflictedFiles() []string

	// Commit operations
	Add(paths ...string) error
	Commit(message string) error
	GetCommitHash() (string, error)

	// Sync operations
	Pull() error
	Push() error
	PushForce() error
	Merge(branch string) error
	Rebase(branch string) error

	// Diff operations
	Diff() (*WorktreeDiff, error)
	DiffWithBranch(branch string) (*WorktreeDiff, error)
}

// Manager is the main interface for Git operations
type Manager interface {
	// Repository management
	GetRepository(repoID string) (Repository, error)
	ListRepositories() ([]*models.Repository, error)
	CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error)

	// Worktree management
	CreateWorktree(repoID, branch, name string) (*models.Worktree, error)
	GetWorktree(worktreeID string) (Worktree, error)
	ListWorktrees() ([]*models.Worktree, error)
	DeleteWorktree(worktreeID string) error

	// GitHub operations
	ListGitHubRepositories() ([]map[string]interface{}, error)
	CreatePullRequest(worktreeID, title, body string) (*models.PullRequestResponse, error)
	UpdatePullRequest(worktreeID, title, body string) (*models.PullRequestResponse, error)
	GetPullRequestInfo(worktreeID string) (*models.PullRequestInfo, error)

	// Conflict checking
	CheckSyncConflicts(worktreeID string) (*models.MergeConflictError, error)
	CheckMergeConflicts(worktreeID string) (*models.MergeConflictError, error)

	// Sync operations
	SyncWorktree(worktreeID, strategy string) error
	MergeWorktreeToMain(worktreeID string, squash bool) error

	// State management
	SaveState() error
	LoadState() error
}

// CommandExecutor abstracts Git command execution
type CommandExecutor interface {
	Execute(dir string, args ...string) ([]byte, error)
	ExecuteWithEnv(dir string, env []string, args ...string) ([]byte, error)
	ExecuteGitWithWorkingDir(workingDir string, args ...string) ([]byte, error)
	ExecuteCommand(command string, args ...string) ([]byte, error)
}

// WorktreeStatus represents the status of a worktree
type WorktreeStatus struct {
	Branch         string
	IsDirty        bool
	HasConflicts   bool
	UnstagedFiles  []string
	StagedFiles    []string
	UntrackedFiles []string
}

// WorktreeDiff represents differences in a worktree
type WorktreeDiff struct {
	Files        []FileDiff
	Insertions   int
	Deletions    int
	FilesChanged int
}

// FileDiff represents changes to a single file
type FileDiff struct {
	Path       string
	Status     string
	Insertions int
	Deletions  int
}
