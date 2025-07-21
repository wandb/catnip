package git

// ServiceHelper provides a convenient wrapper around all Git operations
// This makes it easy for the GitService to use the extracted functionality
type ServiceHelper struct {
	Executor      CommandExecutor
	BranchOps     *BranchOperations
	FetchExecutor *FetchExecutor
	PushExecutor  *PushExecutor
	StatusChecker *StatusChecker
	URLManager    *URLManager
}

// NewServiceHelper creates a new service helper with all components
func NewServiceHelper() *ServiceHelper {
	executor := NewGoGitCommandExecutor() // Use go-git by default

	return &ServiceHelper{
		Executor:      executor,
		BranchOps:     NewBranchOperations(executor),
		FetchExecutor: NewFetchExecutor(executor),
		PushExecutor:  NewPushExecutor(executor),
		StatusChecker: NewStatusChecker(executor),
		URLManager:    NewURLManager(executor),
	}
}

// NewShellServiceHelper creates a new service helper using shell git commands
func NewShellServiceHelper() *ServiceHelper {
	executor := NewGitCommandExecutor() // Use shell git

	return &ServiceHelper{
		Executor:      executor,
		BranchOps:     NewBranchOperations(executor),
		FetchExecutor: NewFetchExecutor(executor),
		PushExecutor:  NewPushExecutor(executor),
		StatusChecker: NewStatusChecker(executor),
		URLManager:    NewURLManager(executor),
	}
}

// NewInMemoryServiceHelper creates a new service helper with in-memory git operations for testing
func NewInMemoryServiceHelper() *ServiceHelper {
	executor := NewInMemoryExecutor()

	return &ServiceHelper{
		Executor:      executor,
		BranchOps:     NewBranchOperations(executor),
		FetchExecutor: NewFetchExecutor(executor),
		PushExecutor:  NewPushExecutor(executor),
		StatusChecker: NewStatusChecker(executor),
		URLManager:    NewURLManager(executor),
	}
}

// GetInMemoryExecutor returns the underlying InMemoryExecutor if this helper uses one, nil otherwise
func (h *ServiceHelper) GetInMemoryExecutor() *InMemoryExecutor {
	if executor, ok := h.Executor.(*InMemoryExecutor); ok {
		return executor
	}
	return nil
}

// Convenience methods for common operations

// ExecuteGit runs a git command with working directory
func (h *ServiceHelper) ExecuteGit(workingDir string, args ...string) ([]byte, error) {
	return h.Executor.ExecuteGitWithWorkingDir(workingDir, args...)
}

// ExecuteCommand runs any command with standard environment
func (h *ServiceHelper) ExecuteCommand(command string, args ...string) ([]byte, error) {
	return h.Executor.ExecuteCommand(command, args...)
}

// BranchExists checks if a branch exists (local by default)
func (h *ServiceHelper) BranchExists(repoPath, branch string, isRemote bool) bool {
	return h.BranchOps.BranchExists(repoPath, branch, BranchExistsOptions{IsRemote: isRemote})
}

// GetCommitCount gets commit count between refs
func (h *ServiceHelper) GetCommitCount(repoPath, fromRef, toRef string) (int, error) {
	return h.BranchOps.GetCommitCount(repoPath, fromRef, toRef)
}

// GetRemoteURL gets the remote URL
func (h *ServiceHelper) GetRemoteURL(repoPath string) (string, error) {
	return h.BranchOps.GetRemoteURL(repoPath)
}

// GetDefaultBranch gets the default branch
func (h *ServiceHelper) GetDefaultBranch(repoPath string) (string, error) {
	return h.BranchOps.GetDefaultBranch(repoPath)
}

// FetchBranchFast performs optimized fetch for status updates
func (h *ServiceHelper) FetchBranchFast(repoPath, branch string) error {
	return h.FetchExecutor.FetchBranchFast(repoPath, branch)
}

// FetchBranchFull performs full fetch for operations needing history
func (h *ServiceHelper) FetchBranchFull(repoPath, branch string) error {
	return h.FetchExecutor.FetchBranchFull(repoPath, branch)
}

// FetchWithStrategy performs fetch using a strategy
func (h *ServiceHelper) FetchWithStrategy(repoPath string, strategy FetchStrategy) error {
	return h.FetchExecutor.FetchBranch(repoPath, strategy)
}

// PushWithStrategy performs push using a strategy
func (h *ServiceHelper) PushWithStrategy(worktreePath string, strategy PushStrategy) error {
	return h.PushExecutor.PushBranch(worktreePath, strategy)
}

// IsDirty checks if worktree has uncommitted changes
func (h *ServiceHelper) IsDirty(worktreePath string) bool {
	return h.StatusChecker.IsDirty(worktreePath)
}

// HasConflicts checks if worktree has conflicts
func (h *ServiceHelper) HasConflicts(worktreePath string) bool {
	return h.StatusChecker.HasConflicts(worktreePath)
}

// GetWorktreeStatus gets comprehensive status
func (h *ServiceHelper) GetWorktreeStatus(worktreePath string) (*WorktreeStatus, error) {
	return h.StatusChecker.GetWorktreeStatus(worktreePath)
}

// SetupRemoteURL sets up remote URL with optional conversion
func (h *ServiceHelper) SetupRemoteURL(worktreePath, remoteName, targetURL string) error {
	return h.URLManager.SetupRemoteURL(worktreePath, remoteName, targetURL)
}
