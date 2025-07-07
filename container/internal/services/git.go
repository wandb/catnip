package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vanpelt/catnip/internal/models"
)

const (
	workspaceDir = "/workspace"
	gitStateDir  = "/workspace/.git-state"
)

// GitService manages Git repositories and worktrees
type GitService struct {
	currentRepo    *models.Repository
	worktrees      map[string]*models.Worktree
	activeWorktree string
	mu             sync.RWMutex
}

// NewGitService creates a new Git service instance
func NewGitService() *GitService {
	s := &GitService{
		worktrees: make(map[string]*models.Worktree),
	}
	
	// Ensure workspace directory exists
	os.MkdirAll(workspaceDir, 0755)
	os.MkdirAll(gitStateDir, 0755)
	
	// Configure Git to use gh as credential helper if available
	s.configureGitCredentials()
	
	// Load existing state if available
	s.loadState()
	
	return s
}

// CheckoutRepository clones a GitHub repository as a bare repo and creates initial worktree
func (s *GitService) CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	repoID := fmt.Sprintf("%s/%s", org, repo)
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", org, repo)
	repoName := strings.ReplaceAll(repo, "/", "-")
	barePath := filepath.Join(workspaceDir, fmt.Sprintf("%s.git", repoName))
	
	// Check if a directory is already mounted at the repo location
	potentialMountPath := filepath.Join(workspaceDir, repoName)
	if info, err := os.Stat(potentialMountPath); err == nil && info.IsDir() {
		// Check if it's a Git repository
		if _, err := os.Stat(filepath.Join(potentialMountPath, ".git")); err == nil {
			log.Printf("⚠️  Found existing Git repository at %s, skipping checkout", potentialMountPath)
			return nil, nil, fmt.Errorf("a repository already exists at %s (possibly mounted)", potentialMountPath)
		}
	}
	
	log.Printf("🔄 Checking out repository: %s", repoID)
	
	// Clone as bare repository with shallow depth
	cloneCmd := exec.Command("git", "clone", "--bare", "--depth", "1", "--single-branch")
	if branch != "" {
		cloneCmd.Args = append(cloneCmd.Args, "--branch", branch)
	}
	cloneCmd.Args = append(cloneCmd.Args, repoURL, barePath)
	
	// Set environment for the clone command
	cloneCmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	output, err := cloneCmd.CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to clone repository: %v\n%s", err, output)
	}
	
	// Get default branch if not specified
	if branch == "" {
		// First try to get the symbolic ref
		cmd := exec.Command("git", "-C", barePath, "symbolic-ref", "refs/remotes/origin/HEAD")
		cmd.Env = cloneCmd.Env
		output, err := cmd.Output()
		if err == nil {
			branch = strings.TrimSpace(strings.TrimPrefix(string(output), "refs/remotes/origin/"))
		} else {
			// If symbolic ref doesn't work, try to find the default branch from remote refs
			cmd = exec.Command("git", "-C", barePath, "branch", "-r")
			cmd.Env = cloneCmd.Env
			output, err = cmd.Output()
			if err == nil {
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.Contains(line, "origin/main") {
						branch = "main"
						break
					} else if strings.Contains(line, "origin/master") {
						branch = "master"
						break
					}
				}
			}
			
			// Final fallback
			if branch == "" {
				branch = "main"
				log.Printf("⚠️ Could not detect default branch, using fallback: %s", branch)
			}
		}
	}
	
	// Create repository object
	repository := &models.Repository{
		ID:            repoID,
		URL:           repoURL,
		Path:          barePath,
		DefaultBranch: branch,
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
	}
	
	s.currentRepo = repository
	
	// Create initial worktree
	worktree, err := s.createWorktreeInternal(branch, branch, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create initial worktree: %v", err)
	}
	
	// Save state
	s.saveState()
	
	log.Printf("✅ Repository checked out successfully: %s", repoID)
	return repository, worktree, nil
}

// CreateWorktree creates a new worktree from source (branch or commit)
func (s *GitService) CreateWorktree(source, name string) (*models.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.currentRepo == nil {
		return nil, fmt.Errorf("no repository checked out")
	}
	
	return s.createWorktreeInternal(source, name, false)
}

// createWorktreeInternal creates a worktree (internal helper)
func (s *GitService) createWorktreeInternal(source, name string, isInitial bool) (*models.Worktree, error) {
	id := uuid.New().String()
	
	// Extract repo name from repo ID (e.g., "owner/repo" -> "repo")
	repoParts := strings.Split(s.currentRepo.ID, "/")
	repoName := repoParts[len(repoParts)-1]
	
	// All worktrees use repo/branch pattern for consistency
	// This makes it easier to manage multiple branches and matches our session ID format
	worktreePath := filepath.Join(workspaceDir, repoName, name)
	
	// Create worktree using git
	cmd := exec.Command("git", "-C", s.currentRepo.Path, "worktree", "add", worktreePath, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v\n%s", err, output)
	}
	
	// Get current commit hash
	cmd = exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD")
	commitOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}
	
	worktree := &models.Worktree{
		ID:           id,
		RepoID:       s.currentRepo.ID,
		Name:         name,
		Path:         worktreePath,
		Branch:       source,
		CommitHash:   strings.TrimSpace(string(commitOutput)),
		IsActive:     isInitial,
		IsDirty:      false,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}
	
	s.worktrees[id] = worktree
	
	if isInitial || len(s.worktrees) == 1 {
		s.activeWorktree = id
		s.updateCurrentSymlink(worktreePath)
	}
	
	return worktree, nil
}

// ListWorktrees returns all worktrees
func (s *GitService) ListWorktrees() []*models.Worktree {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	worktrees := make([]*models.Worktree, 0, len(s.worktrees))
	for _, wt := range s.worktrees {
		// Update dirty status
		wt.IsDirty = s.checkIfDirty(wt.Path)
		worktrees = append(worktrees, wt)
	}
	
	return worktrees
}

// ActivateWorktree switches to a different worktree
func (s *GitService) ActivateWorktree(worktreeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	worktree, exists := s.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree not found: %s", worktreeID)
	}
	
	// Update active status
	for _, wt := range s.worktrees {
		wt.IsActive = (wt.ID == worktreeID)
	}
	
	s.activeWorktree = worktreeID
	worktree.LastAccessed = time.Now()
	
	// Update current symlink
	s.updateCurrentSymlink(worktree.Path)
	
	// Save state
	s.saveState()
	
	log.Printf("✅ Activated worktree: %s (%s)", worktree.Name, worktree.Path)
	return nil
}

// GetStatus returns the current Git status
func (s *GitService) GetStatus() *models.GitStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var activeWorktree *models.Worktree
	if s.activeWorktree != "" {
		activeWorktree = s.worktrees[s.activeWorktree]
	}
	
	return &models.GitStatus{
		Repository:     s.currentRepo,
		ActiveWorktree: activeWorktree,
		WorktreeCount:  len(s.worktrees),
	}
}

// checkIfDirty checks if a worktree has uncommitted changes
func (s *GitService) checkIfDirty(worktreePath string) bool {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// updateCurrentSymlink updates the /workspace/current symlink
func (s *GitService) updateCurrentSymlink(targetPath string) error {
	currentPath := filepath.Join(workspaceDir, "current")
	
	// Remove existing symlink if it exists
	os.Remove(currentPath)
	
	// Create new symlink
	return os.Symlink(targetPath, currentPath)
}

// State persistence

func (s *GitService) saveState() error {
	state := map[string]interface{}{
		"repository":      s.currentRepo,
		"worktrees":       s.worktrees,
		"activeWorktree":  s.activeWorktree,
	}
	
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filepath.Join(gitStateDir, "state.json"), data, 0644)
}

func (s *GitService) loadState() error {
	data, err := os.ReadFile(filepath.Join(gitStateDir, "state.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No state to load
		}
		return err
	}
	
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	
	// Load repository
	if repoData, exists := state["repository"]; exists {
		var repo models.Repository
		if err := json.Unmarshal(repoData, &repo); err == nil {
			s.currentRepo = &repo
		}
	}
	
	// Load worktrees
	if worktreesData, exists := state["worktrees"]; exists {
		var worktrees map[string]*models.Worktree
		if err := json.Unmarshal(worktreesData, &worktrees); err == nil {
			s.worktrees = worktrees
		}
	}
	
	// Load active worktree
	if activeData, exists := state["activeWorktree"]; exists {
		var active string
		if err := json.Unmarshal(activeData, &active); err == nil {
			s.activeWorktree = active
		}
	}
	
	return nil
}

// GetActiveWorktreePath returns the path to the active worktree
func (s *GitService) GetActiveWorktreePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.activeWorktree != "" {
		if wt, exists := s.worktrees[s.activeWorktree]; exists {
			return wt.Path
		}
	}
	
	return workspaceDir // fallback
}

// configureGitCredentials sets up Git to use gh CLI for GitHub authentication
func (s *GitService) configureGitCredentials() {
	// Check if gh CLI is authenticated
	cmd := exec.Command("gh", "auth", "status")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	if err := cmd.Run(); err != nil {
		log.Printf("ℹ️ GitHub CLI not authenticated, Git operations will only work with public repositories")
		return
	}
	
	log.Printf("🔐 Configuring Git to use GitHub CLI for authentication")
	
	// Configure Git to use gh as credential helper for GitHub
	configCmd := exec.Command("git", "config", "--global", "credential.https://github.com.helper", "!gh auth git-credential")
	configCmd.Env = append(os.Environ(),
		"HOME=/home/catnip", 
		"USER=catnip",
	)
	
	if err := configCmd.Run(); err != nil {
		log.Printf("❌ Failed to configure Git credential helper: %v", err)
	} else {
		log.Printf("✅ Git credential helper configured successfully")
	}
}