package git

import "time"

// WorktreeStatus represents the status of a worktree
type WorktreeStatus struct {
	Branch         string
	IsDirty        bool
	HasConflicts   bool
	UnstagedFiles  []string
	StagedFiles    []string
	UntrackedFiles []string
}

// Operations provides a comprehensive interface for all Git operations
// This consolidates all git functionality into a single, testable interface
type Operations interface {
	// Core command execution
	ExecuteGit(workingDir string, args ...string) ([]byte, error)
	ExecuteGitWithTimeout(workingDir string, timeout time.Duration, args ...string) ([]byte, error)
	ExecuteCommand(command string, args ...string) ([]byte, error)

	// Branch operations
	BranchExists(repoPath, branch string, isRemote bool) bool
	GetCommitCount(repoPath, fromRef, toRef string) (int, error)
	GetRemoteURL(repoPath string) (string, error)
	GetDefaultBranch(repoPath string) (string, error)
	GetRemoteDefaultBranch(repoPath string) (string, error)
	GetLocalBranches(repoPath string) ([]string, error)
	GetRemoteBranches(repoPath string, defaultBranch string) ([]string, error)
	GetRemoteBranchesFromURL(remoteURL string) ([]string, error)
	CreateBranch(repoPath, branch, fromRef string) error
	DeleteBranch(repoPath, branch string, force bool) error
	ListBranches(repoPath string, options ListBranchesOptions) ([]string, error)
	RenameBranch(repoPath, oldBranch, newBranch string) error

	// Worktree operations
	CreateWorktree(repoPath, worktreePath, branch, fromRef string) error
	RemoveWorktree(repoPath, worktreePath string, force bool) error
	ListWorktrees(repoPath string) ([]WorktreeInfo, error)
	PruneWorktrees(repoPath string) error

	// Status operations
	IsDirty(worktreePath string) bool
	HasConflicts(worktreePath string) bool
	HasUncommittedChanges(worktreePath string) (bool, error)
	GetConflictedFiles(worktreePath string) ([]string, error)
	GetStatus(worktreePath string) (*WorktreeStatus, error)

	// Fetch operations
	FetchBranch(repoPath string, strategy FetchStrategy) error
	FetchBranchFast(repoPath, branch string) error
	FetchBranchFull(repoPath, branch string) error

	// Push operations
	PushBranch(worktreePath string, strategy PushStrategy) error

	// Remote operations
	AddRemote(repoPath, name, url string) error
	RemoveRemote(repoPath, name string) error
	SetRemoteURL(repoPath, name, url string) error
	GetRemotes(repoPath string) (map[string]string, error)

	// Clone operations
	Clone(url, path string, options CloneOptions) error

	// Commit operations
	Add(worktreePath string, paths ...string) error
	Commit(worktreePath, message string, options CommitOptions) error
	GetCommitHash(worktreePath, ref string) (string, error)
	ResetMixed(worktreePath, ref string) error

	// Merge/Rebase operations
	Merge(worktreePath, ref string) error
	Rebase(worktreePath, ref string) error
	CherryPick(worktreePath, commit string) error
	AbortRebase(worktreePath string) error
	ContinueRebase(worktreePath string) error

	// Diff operations
	DiffNameOnly(worktreePath, filter string) ([]string, error)
	MergeTree(worktreePath, base, head string) (string, error)

	// Stash operations
	Stash(worktreePath string) error
	StashPop(worktreePath string) error

	// Tag operations
	CreateTag(repoPath, tag, ref string) error
	DeleteTag(repoPath, tag string) error
	ListTags(repoPath string) ([]string, error)

	// Config operations
	GetConfig(repoPath, key string) (string, error)
	SetConfig(repoPath, key, value string) error
	UnsetConfig(repoPath, key string) error
	SetGlobalConfig(key, value string) error

	// Branch display name (checks for nice name mapping first)
	GetDisplayBranch(worktreePath string) (string, error)

	// Rev operations
	RevParse(repoPath, ref string) (string, error)
	RevList(repoPath string, options RevListOptions) ([]string, error)
	ShowRef(repoPath, ref string, options ShowRefOptions) error

	// Garbage collection
	GarbageCollect(repoPath string) error

	// Utility operations
	IsGitRepository(path string) bool
	GetGitRoot(path string) (string, error)
}

// ListBranchesOptions configures branch listing
type ListBranchesOptions struct {
	All    bool   // Include remote branches
	Remote bool   // Only remote branches
	Local  bool   // Only local branches (default)
	Merged string // Only branches merged into specified branch
}

// CloneOptions configures clone operation
type CloneOptions struct {
	Bare         bool
	Depth        int
	SingleBranch bool
	Branch       string
}

// CommitOptions configures commit operation
type CommitOptions struct {
	NoVerify bool // Skip pre-commit hooks
	Amend    bool
	Author   string
}

// RevListOptions configures rev-list operation
type RevListOptions struct {
	Count    bool
	MaxCount int
	Since    string
	Until    string
}

// ShowRefOptions configures show-ref operation
type ShowRefOptions struct {
	Verify bool
	Quiet  bool
}

// WorktreeInfo represents information about a worktree
type WorktreeInfo struct {
	Path   string
	Branch string
	Commit string
	Bare   bool
}
