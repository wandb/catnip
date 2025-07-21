package git

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vanpelt/catnip/internal/models"
)

const (
	workspaceDir = "/workspace"
	reposDir     = "/workspace/repos"
	stateFile    = "/workspace/.git-state"
)

// ManagerImpl implements the Manager interface
type ManagerImpl struct {
	mu           sync.RWMutex
	repositories map[string]*models.Repository
	worktrees    map[string]*models.Worktree
	executor     CommandExecutor
	stateManager *StateManager
}

// NewManager creates a new Git manager
func NewManager() Manager {
	manager := &ManagerImpl{
		repositories: make(map[string]*models.Repository),
		worktrees:    make(map[string]*models.Worktree),
		executor:     NewGitCommandExecutor(),
		stateManager: NewStateManager(stateFile),
	}

	// Load state on initialization
	if err := manager.LoadState(); err != nil {
		log.Printf("⚠️ Failed to load state: %v", err)
	}

	return manager
}

// GetRepository returns a repository by ID
func (m *ManagerImpl) GetRepository(repoID string) (Repository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	repo, exists := m.repositories[repoID]
	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	return NewRepository(repo.Path, m.executor), nil
}

// ListRepositories returns all repositories
func (m *ManagerImpl) ListRepositories() ([]*models.Repository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	repos := make([]*models.Repository, 0, len(m.repositories))
	for _, repo := range m.repositories {
		repos = append(repos, repo)
	}

	return repos, nil
}

// CheckoutRepository checks out a repository
func (m *ManagerImpl) CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error) {
	repoID := fmt.Sprintf("%s/%s", org, repo)
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", org, repo)

	// Check if repository already exists
	m.mu.RLock()
	existingRepo, exists := m.repositories[repoID]
	m.mu.RUnlock()

	if exists {
		// Create a new worktree for existing repository
		worktree, err := m.CreateWorktree(repoID, branch, GenerateSessionName())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create worktree: %w", err)
		}
		return existingRepo, worktree, nil
	}

	// Clone new repository
	barePath := filepath.Join(reposDir, fmt.Sprintf("%s_%s.git", strings.ReplaceAll(org, "/", "_"), repo))

	// Create repository object
	repository := &models.Repository{
		ID:           repoID,
		URL:          repoURL,
		Path:         barePath,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}

	// Clone the repository
	gitRepo := NewRepository(barePath, m.executor)
	if err := gitRepo.Clone(repoURL, barePath); err != nil {
		return nil, nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Get default branch
	defaultBranch, err := gitRepo.GetDefaultBranch()
	if err != nil {
		defaultBranch = "main"
	}
	repository.DefaultBranch = defaultBranch

	// Store repository
	m.mu.Lock()
	m.repositories[repoID] = repository
	m.mu.Unlock()

	// Create initial worktree
	if branch == "" {
		branch = defaultBranch
	}
	worktree, err := m.CreateWorktree(repoID, branch, GenerateSessionName())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create initial worktree: %w", err)
	}

	// Save state
	_ = m.SaveState()

	return repository, worktree, nil
}

// CreateWorktree creates a new worktree
func (m *ManagerImpl) CreateWorktree(repoID, branch, name string) (*models.Worktree, error) {
	m.mu.RLock()
	repo, exists := m.repositories[repoID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	// Generate unique ID
	worktreeID := uuid.New().String()

	// Extract repo name
	repoParts := strings.Split(repoID, "/")
	repoName := repoParts[len(repoParts)-1]

	// Create worktree path (extract clean name for filesystem)
	workspaceName := ExtractWorkspaceName(name)
	worktreePath := filepath.Join(workspaceDir, repoName, workspaceName)

	// Create worktree using git command
	args := []string{"-C", repo.Path, "worktree", "add", "-b", name, worktreePath}
	if branch != "" {
		args = append(args, branch)
	}

	output, err := m.executor.Execute("", args...)
	if err != nil {
		// Check if branch already exists
		if strings.Contains(string(output), "already exists") {
			// Try with a new name
			newName := m.generateUniqueSessionName(repo.Path)
			return m.CreateWorktree(repoID, branch, newName)
		}
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	// Create worktree object (use already extracted workspaceName from line 156)
	worktree := &models.Worktree{
		ID:           worktreeID,
		RepoID:       repoID,
		Name:         workspaceName, // Clean display name
		Path:         worktreePath,
		Branch:       name, // Full git branch name
		SourceBranch: branch,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}

	// Get initial commit hash
	gitWorktree := NewWorktree(worktreePath, name, m.executor)
	if hash, err := gitWorktree.GetCommitHash(); err == nil {
		worktree.CommitHash = hash
	}

	// Store worktree
	m.mu.Lock()
	m.worktrees[worktreeID] = worktree
	m.mu.Unlock()

	// Save state
	_ = m.SaveState()

	return worktree, nil
}

// GetWorktree returns a worktree by ID
func (m *ManagerImpl) GetWorktree(worktreeID string) (Worktree, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	worktree, exists := m.worktrees[worktreeID]
	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	return NewWorktree(worktree.Path, worktree.Branch, m.executor), nil
}

// ListWorktrees returns all worktrees
func (m *ManagerImpl) ListWorktrees() ([]*models.Worktree, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	worktrees := make([]*models.Worktree, 0, len(m.worktrees))
	for _, wt := range m.worktrees {
		worktrees = append(worktrees, wt)
	}

	return worktrees, nil
}

// DeleteWorktree deletes a worktree
func (m *ManagerImpl) DeleteWorktree(worktreeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	worktree, exists := m.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	repo, exists := m.repositories[worktree.RepoID]
	if !exists {
		return fmt.Errorf("repository %s not found", worktree.RepoID)
	}

	// Remove worktree using git
	_, err := m.executor.Execute("", "-C", repo.Path, "worktree", "remove", "--force", worktree.Path)
	if err != nil {
		// Try to remove directory anyway
		_ = os.RemoveAll(worktree.Path)
	}

	// Delete branch
	_, _ = m.executor.Execute("", "-C", repo.Path, "branch", "-D", worktree.Branch)

	// Remove from map
	delete(m.worktrees, worktreeID)

	// Save state
	_ = m.SaveState()

	return nil
}

// SaveState saves the current state
func (m *ManagerImpl) SaveState() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.stateManager.SaveState(m.repositories, m.worktrees)
}

// LoadState loads the saved state
func (m *ManagerImpl) LoadState() error {
	repositories, worktrees, err := m.stateManager.LoadState()
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.repositories = repositories
	m.worktrees = worktrees
	m.mu.Unlock()

	return nil
}

// generateUniqueSessionName generates a unique session name
func (m *ManagerImpl) generateUniqueSessionName(repoPath string) string {
	gitRepo := NewRepository(repoPath, m.executor)
	return GenerateUniqueSessionName(func(name string) bool {
		return gitRepo.BranchExists(name)
	})
}

// ListGitHubRepositories returns a list of GitHub repositories (not implemented)
func (m *ManagerImpl) ListGitHubRepositories() ([]map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

// CreatePullRequest creates a new pull request (not implemented)
func (m *ManagerImpl) CreatePullRequest(worktreeID, title, body string) (*models.PullRequestResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// UpdatePullRequest updates an existing pull request (not implemented)
func (m *ManagerImpl) UpdatePullRequest(worktreeID, title, body string) (*models.PullRequestResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetPullRequestInfo retrieves pull request information (not implemented)
func (m *ManagerImpl) GetPullRequestInfo(worktreeID string) (*models.PullRequestInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// CheckSyncConflicts checks for potential sync conflicts (not implemented)
func (m *ManagerImpl) CheckSyncConflicts(worktreeID string) (*models.MergeConflictError, error) {
	return nil, fmt.Errorf("not implemented")
}

// CheckMergeConflicts checks for merge conflicts (not implemented)
func (m *ManagerImpl) CheckMergeConflicts(worktreeID string) (*models.MergeConflictError, error) {
	return nil, fmt.Errorf("not implemented")
}

// SyncWorktree synchronizes a worktree with its base branch (not implemented)
func (m *ManagerImpl) SyncWorktree(worktreeID, strategy string) error {
	return fmt.Errorf("not implemented")
}

// MergeWorktreeToMain merges a worktree back to the main branch (not implemented)
func (m *ManagerImpl) MergeWorktreeToMain(worktreeID string, squash bool) error {
	return fmt.Errorf("not implemented")
}
