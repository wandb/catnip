package services

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vanpelt/catnip/internal/models"
)

const (
	workspaceDir = "/workspace"
	gitStateDir  = "/workspace/.git-state"
	devRepoPath  = "/workspace/catnip"
)

// Fun session name generation (matches frontend and worker)
var verbs = []string{"warp", "pixelate", "compile", "encrypt", "vectorize", "hydrate", "fork",
	"spawn", "dockerize", "cache", "teleport", "refactor", "quantize", "stream", "debug"}

var nouns = []string{"otter", "kraken", "wombat", "quokka", "nebula", "photon", "quasar",
	"badger", "pangolin", "goblin", "cyborg", "ninja", "gizmo", "raptor", "penguin"}

func generateSessionName() string {
	verb := verbs[rand.Intn(len(verbs))]
	noun := nouns[rand.Intn(len(nouns))]
	return fmt.Sprintf("%s-%s", verb, noun)
}

// GitService manages multiple Git repositories and their worktrees
type GitService struct {
	repositories map[string]*models.Repository // key: repoID (e.g., "owner/repo")
	worktrees    map[string]*models.Worktree   // key: worktree ID
	mu           sync.RWMutex
}

// NewGitService creates a new Git service instance
func NewGitService() *GitService {
	s := &GitService{
		repositories: make(map[string]*models.Repository),
		worktrees:    make(map[string]*models.Worktree),
	}
	
	// Ensure workspace directory exists
	os.MkdirAll(workspaceDir, 0755)
	os.MkdirAll(gitStateDir, 0755)
	
	// Configure Git to use gh as credential helper if available
	s.configureGitCredentials()
	
	// Load existing state if available
	s.loadState()
	
	// Detect and load the dev repo if it exists
	s.detectDevRepo()
	
	// Commit sync service removed - git worktrees handle synchronization automatically
	
	return s
}

// CheckoutRepository clones a GitHub repository as a bare repo and creates initial worktree
func (s *GitService) CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	repoID := fmt.Sprintf("%s/%s", org, repo)
	
	// Handle dev repo specially
	if repoID == "catnip-dev/dev" {
		return s.handleDevRepoWorktree(branch)
	}
	
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
	
	// Check if repository already exists in our map
	if existingRepo, exists := s.repositories[repoID]; exists {
		log.Printf("🔄 Repository already loaded, creating new worktree: %s", repoID)
		return s.createWorktreeForExistingRepo(existingRepo, branch)
	}
	
	// Check if bare repository already exists on disk
	if _, err := os.Stat(barePath); err == nil {
		log.Printf("🔄 Found existing bare repository, loading and creating new worktree: %s", repoID)
		return s.handleExistingRepository(repoID, repoURL, barePath, branch)
	}
	
	log.Printf("🔄 Cloning new repository: %s", repoID)
	
	return s.cloneNewRepository(repoID, repoURL, barePath, branch)
}

// handleExistingRepository handles checkout when bare repo already exists
func (s *GitService) handleExistingRepository(repoID, repoURL, barePath, branch string) (*models.Repository, *models.Worktree, error) {
	// Load existing repository if we have state
	var repo *models.Repository
	if existingRepo, exists := s.repositories[repoID]; exists {
		log.Printf("📦 Repository already loaded: %s", repoID)
		repo = existingRepo
	} else {
		// Create repository object for existing bare repo
		defaultBranch, err := s.getRepositoryDefaultBranch(barePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get default branch: %v", err)
		}
		
		repo = &models.Repository{
			ID:            repoID,
			URL:           repoURL,
			Path:          barePath,
			DefaultBranch: defaultBranch,
			CreatedAt:     time.Now(), // We don't know the real creation time
			LastAccessed:  time.Now(),
		}
		s.repositories[repoID] = repo
	}
	
	// If no branch specified, use default
	if branch == "" {
		branch = repo.DefaultBranch
	}
	
	// Check if the requested branch exists in the bare repo
	if !s.branchExistsInBareRepo(barePath, branch) {
		log.Printf("🔄 Branch %s not found, fetching from remote", branch)
		if err := s.fetchBranchIntoBareRepo(barePath, branch); err != nil {
			return nil, nil, fmt.Errorf("failed to fetch branch %s: %v", branch, err)
		}
	}
	
	// Create new worktree with fun name
	funName := generateSessionName()
	// Creating worktree
	worktree, err := s.createWorktreeInternalForRepo(repo, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree: %v", err)
	}
	
	// Save state
	s.saveState()
	
	log.Printf("✅ Worktree created from existing repository: %s", repoID)
	return repo, worktree, nil
}

// cloneNewRepository clones a new bare repository
func (s *GitService) cloneNewRepository(repoID, repoURL, barePath, branch string) (*models.Repository, *models.Worktree, error) {
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
		branch, err = s.getRepositoryDefaultBranch(barePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get default branch: %v", err)
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
	
	// Add to repositories map
	s.repositories[repoID] = repository
	
	// Start background unshallow process for the requested branch
	go s.unshallowRepository(barePath, branch)
	
	// Create initial worktree with fun name to avoid conflicts with local branches
	funName := generateSessionName()
	// Creating initial worktree
	worktree, err := s.createWorktreeInternalForRepo(repository, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create initial worktree: %v", err)
	}
	
	// Save state
	s.saveState()
	
	log.Printf("✅ Repository cloned successfully: %s", repository.ID)
	return repository, worktree, nil
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


// GetStatus returns the current Git status
func (s *GitService) GetStatus() *models.GitStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Find most recently accessed worktree for backward compatibility
	var mostRecentWorktree *models.Worktree
	var mostRecentRepo *models.Repository
	
	for _, wt := range s.worktrees {
		if mostRecentWorktree == nil || wt.LastAccessed.After(mostRecentWorktree.LastAccessed) {
			mostRecentWorktree = wt
		}
	}
	
	if mostRecentWorktree != nil {
		mostRecentRepo = s.repositories[mostRecentWorktree.RepoID]
	}
	
	return &models.GitStatus{
		Repository:     mostRecentRepo,    // Repository of most recent worktree for backward compatibility
		Repositories:   s.repositories,   // All repositories
		ActiveWorktree: mostRecentWorktree, // Most recent worktree for backward compatibility
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
		"repositories": s.repositories,
		"worktrees":    s.worktrees,
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
	
	// Load repositories - support both old single repo format and new multi-repo format
	if reposData, exists := state["repositories"]; exists {
		// New multi-repo format
		var repos map[string]*models.Repository
		if err := json.Unmarshal(reposData, &repos); err == nil {
			s.repositories = repos
		}
	} else if repoData, exists := state["repository"]; exists {
		// Old single repo format - migrate to new format
		var repo models.Repository
		if err := json.Unmarshal(repoData, &repo); err == nil {
			s.repositories[repo.ID] = &repo
		}
	}
	
	// Load worktrees
	if worktreesData, exists := state["worktrees"]; exists {
		var worktrees map[string]*models.Worktree
		if err := json.Unmarshal(worktreesData, &worktrees); err == nil {
			s.worktrees = worktrees
		}
	}
	
	// Note: No longer loading activeWorktree since we removed single active worktree concept
	
	return nil
}

// GetDefaultWorktreePath returns the path to the most recently accessed worktree
func (s *GitService) GetDefaultWorktreePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Find most recently accessed worktree
	var mostRecentWorktree *models.Worktree
	for _, wt := range s.worktrees {
		if mostRecentWorktree == nil || wt.LastAccessed.After(mostRecentWorktree.LastAccessed) {
			mostRecentWorktree = wt
		}
	}
	
	if mostRecentWorktree != nil {
		return mostRecentWorktree.Path
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

// TriggerManualSync is no longer needed - git worktrees sync automatically
func (s *GitService) TriggerManualSync() error {
	return nil // No-op
}

// ListGitHubRepositories returns a list of GitHub repositories accessible to the user
func (s *GitService) ListGitHubRepositories() ([]map[string]interface{}, error) {
	var repos []map[string]interface{}
	
	// Add dev repository if it exists
	s.mu.RLock()
	if _, exists := s.repositories["catnip-dev"]; exists {
		repos = append(repos, map[string]interface{}{
			"name":        "catnip-dev",
			"url":         "catnip-dev/dev", // Special format for dev repo
			"private":     false,
			"description": "Development repository (mounted)",
			"fullName":    "catnip-dev",
		})
	}
	s.mu.RUnlock()
	
	// Get GitHub repositories
	cmd := exec.Command("gh", "repo", "list", "--limit", "100", "--json", "name,url,isPrivate,description,owner")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	output, err := cmd.Output()
	if err != nil {
		// If GitHub CLI fails, still return dev repo if it exists
		if len(repos) > 0 {
			return repos, nil
		}
		return nil, fmt.Errorf("failed to list GitHub repositories: %w", err)
	}
	
	var githubRepos []map[string]interface{}
	if err := json.Unmarshal(output, &githubRepos); err != nil {
		// If parsing fails, still return dev repo if it exists
		if len(repos) > 0 {
			return repos, nil
		}
		return nil, fmt.Errorf("failed to parse repository list: %w", err)
	}
	
	// Transform the GitHub data to match frontend expectations
	for _, repo := range githubRepos {
		// Add full name for display
		if owner, ok := repo["owner"].(map[string]interface{}); ok {
			if login, ok := owner["login"].(string); ok {
				if name, ok := repo["name"].(string); ok {
					repo["fullName"] = fmt.Sprintf("%s/%s", login, name)
				}
			}
		}
		// Rename isPrivate to private
		if isPrivate, ok := repo["isPrivate"]; ok {
			repo["private"] = isPrivate
			delete(repo, "isPrivate")
		}
	}
	
	// Add GitHub repos to the list
	repos = append(repos, githubRepos...)
	
	return repos, nil
}

// detectDevRepo checks if the dev repo exists and loads it into the service
func (s *GitService) detectDevRepo() {
	// Check if dev repo exists and is a git repository
	if _, err := os.Stat(devRepoPath); os.IsNotExist(err) {
		return
	}
	
	if _, err := os.Stat(filepath.Join(devRepoPath, ".git")); os.IsNotExist(err) {
		return
	}
	
	log.Printf("🔍 Detected dev repository at %s", devRepoPath)
	
	// Create repository object for the dev repo
	repo := &models.Repository{
		ID:            "catnip-dev",
		URL:           "file://" + devRepoPath, // Local file URL
		Path:          devRepoPath,
		DefaultBranch: s.getDevRepoDefaultBranch(),
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
	}
	
	// Add to repositories map
	s.repositories[repo.ID] = repo
	
	log.Printf("✅ Dev repository loaded: %s", repo.ID)
}

// getDevRepoDefaultBranch gets the current branch of the dev repo
func (s *GitService) getDevRepoDefaultBranch() string {
	cmd := exec.Command("git", "-C", devRepoPath, "branch", "--show-current")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ Could not get current branch for dev repo, using fallback: main")
		return "main"
	}
	
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "main"
	}
	
	return branch
}

// handleDevRepoWorktree creates a worktree for the dev repo
func (s *GitService) handleDevRepoWorktree(branch string) (*models.Repository, *models.Worktree, error) {
	// Get the dev repo from repositories map
	devRepo, exists := s.repositories["catnip-dev"]
	if !exists {
		return nil, nil, fmt.Errorf("dev repository not found - it may not be mounted")
	}
	
	// If no branch specified, use current branch
	if branch == "" {
		branch = devRepo.DefaultBranch
	}
	
	// Check if branch exists in the dev repo
	if !s.devRepoBranchExists(branch) {
		return nil, nil, fmt.Errorf("branch %s does not exist in dev repository", branch)
	}
	
	// Create new worktree with fun name
	funName := generateSessionName()
	
	// Create worktree for dev repo
	worktree, err := s.createDevRepoWorktree(devRepo, branch, funName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree for dev repo: %v", err)
	}
	
	// Save state
	s.saveState()
	
	log.Printf("✅ Dev repo worktree created: %s", worktree.Name)
	return devRepo, worktree, nil
}

// devRepoBranchExists checks if a branch exists in the dev repo
func (s *GitService) devRepoBranchExists(branch string) bool {
	cmd := exec.Command("git", "-C", devRepoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	return cmd.Run() == nil
}

// createDevRepoWorktree creates a worktree for the dev repo
func (s *GitService) createDevRepoWorktree(repo *models.Repository, branch, name string) (*models.Worktree, error) {
	id := uuid.New().String()
	
	// Create worktree path with catnip prefix
	worktreePath := filepath.Join(workspaceDir, "catnip", name)
	
	// Create worktree directory first
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %v", err)
	}
	
	// Create worktree with new branch using the fun name
	cmd := exec.Command("git", "-C", devRepoPath, "worktree", "add", "-b", name, worktreePath, branch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v\n%s", err, output)
	}
	
	// Get current commit hash
	cmd = exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	commitOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}
	
	// Calculate commit count ahead of source
	commitCount := 0
	if branch != name { // Only count if different from current branch
		cmd = exec.Command("git", "-C", worktreePath, "rev-list", "--count", fmt.Sprintf("%s..HEAD", branch))
		cmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)
		countOutput, err := cmd.Output()
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(countOutput))); parseErr == nil {
				commitCount = count
			}
		}
	}
	
	// Create display name with catnip prefix
	displayName := fmt.Sprintf("catnip/%s", name)
	
	worktree := &models.Worktree{
		ID:           id,
		RepoID:       repo.ID,
		Name:         displayName,
		Path:         worktreePath,
		Branch:       name,
		SourceBranch: branch,
		CommitHash:   strings.TrimSpace(string(commitOutput)),
		CommitCount:  commitCount,
		IsDirty:      false,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}
	
	s.worktrees[id] = worktree
	
	// Update current symlink to point to this worktree if it's the first one
	if len(s.worktrees) == 1 {
		s.updateCurrentSymlink(worktreePath)
	}
	
	return worktree, nil
}

// GetRepositoryBranches returns the remote branches for a repository
func (s *GitService) GetRepositoryBranches(repoID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	repo, exists := s.repositories[repoID]
	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}
	
	// Handle dev repo specially
	if repoID == "catnip-dev" {
		return s.getDevRepoBranches()
	}
	
	// Start with the default branch
	branches := []string{repo.DefaultBranch}
	branchSet := map[string]bool{repo.DefaultBranch: true}
	
	cmd := exec.Command("git", "-C", repo.Path, "branch", "-r")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	output, err := cmd.Output()
	if err != nil {
		return branches, nil // Return at least the default branch
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "HEAD ->") {
			// Remove "origin/" prefix
			branch := line
			if strings.HasPrefix(line, "origin/") {
				branch = strings.TrimPrefix(line, "origin/")
			}
			
			// Add to list if not already present
			if !branchSet[branch] {
				branches = append(branches, branch)
				branchSet[branch] = true
			}
		}
	}
	
	return branches, nil
}

// getDevRepoBranches returns the local branches for the dev repo
func (s *GitService) getDevRepoBranches() ([]string, error) {
	cmd := exec.Command("git", "-C", devRepoPath, "branch")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get dev repo branches: %v", err)
	}
	
	var branches []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// Remove the * prefix for current branch
			branch := strings.TrimPrefix(line, "* ")
			branches = append(branches, branch)
		}
	}
	
	return branches, nil
}

// DeleteWorktree removes a worktree
func (s *GitService) DeleteWorktree(worktreeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	worktree, exists := s.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}
	
	// Note: No longer checking for "active" worktree since we removed single active worktree concept
	
	// Get repository for worktree deletion
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return fmt.Errorf("repository %s not found", worktree.RepoID)
	}
	
	// Remove the worktree
	cmd := exec.Command("git", "-C", repo.Path, "worktree", "remove", "--force", worktree.Path)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}
	
	// Remove from memory
	delete(s.worktrees, worktreeID)
	
	// Save state
	s.saveState()
	
	return nil
}

// UpdateWorktreeStatus updates commit count and dirty status for a worktree
func (s *GitService) UpdateWorktreeStatus(worktreeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	worktree, exists := s.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}
	
	// Update dirty status
	worktree.IsDirty = s.checkIfDirty(worktree.Path)
	
	// Update commit count
	if worktree.SourceBranch != "" && worktree.SourceBranch != worktree.Branch {
		cmd := exec.Command("git", "-C", worktree.Path, "rev-list", "--count", fmt.Sprintf("%s..HEAD", worktree.SourceBranch))
		countOutput, err := cmd.Output()
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(countOutput))); parseErr == nil {
				worktree.CommitCount = count
			}
		}
	}
	
	return nil
}

// Stop stops the Git service
func (s *GitService) Stop() {
	// No background services to stop
}

// getRepositoryDefaultBranch gets the default branch from a bare repository
func (s *GitService) getRepositoryDefaultBranch(barePath string) (string, error) {
	// First try to get the symbolic ref
	cmd := exec.Command("git", "-C", barePath, "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(strings.TrimPrefix(string(output), "refs/remotes/origin/")), nil
	}
	
	// If symbolic ref doesn't work, try to find the default branch from remote refs
	cmd = exec.Command("git", "-C", barePath, "branch", "-r")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "origin/main") {
				return "main", nil
			} else if strings.Contains(line, "origin/master") {
				return "master", nil
			}
		}
	}
	
	// Final fallback
	log.Printf("⚠️ Could not detect default branch, using fallback: main")
	return "main", nil
}

// branchExistsInBareRepo checks if a branch exists in the bare repository
func (s *GitService) branchExistsInBareRepo(barePath, branch string) bool {
	// Check for local branch first
	cmd := exec.Command("git", "-C", barePath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	if cmd.Run() == nil {
		return true
	}
	
	// Check for remote branch
	cmd = exec.Command("git", "-C", barePath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/remotes/origin/%s", branch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	return cmd.Run() == nil
}

// fetchBranchIntoBareRepo fetches a specific branch into the bare repository
func (s *GitService) fetchBranchIntoBareRepo(barePath, branch string) error {
	// First, try to fetch just the remote ref without updating local branch
	// This avoids the "refusing to fetch into branch checked out" error
	cmd := exec.Command("git", "-C", barePath, "fetch", "origin", fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch), "--depth", "1")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch branch: %v\n%s", err, output)
	}
	
	// Now create/update the local branch ref from the remote ref
	// Only do this if the branch isn't currently checked out in a worktree
	updateCmd := exec.Command("git", "-C", barePath, "update-ref", fmt.Sprintf("refs/heads/%s", branch), fmt.Sprintf("refs/remotes/origin/%s", branch))
	updateCmd.Env = cmd.Env
	updateOutput, err := updateCmd.CombinedOutput()
	if err != nil {
		// If update-ref fails because branch is checked out, that's okay
		// The remote ref is still updated and worktrees can use it
		log.Printf("⚠️ Could not update local branch ref (branch may be checked out): %s", string(updateOutput))
	}
	
	return nil
}

// createWorktreeForExistingRepo creates a worktree for an already loaded repository
func (s *GitService) createWorktreeForExistingRepo(repo *models.Repository, branch string) (*models.Repository, *models.Worktree, error) {
	// If no branch specified, use default
	if branch == "" {
		branch = repo.DefaultBranch
	}
	
	// Handle dev repo specially (it doesn't have a bare repo)
	if repo.ID == "catnip-dev" {
		return s.handleDevRepoWorktree(branch)
	}
	
	// Check if the requested branch exists in the bare repo
	if !s.branchExistsInBareRepo(repo.Path, branch) {
		log.Printf("🔄 Branch %s not found, fetching from remote", branch)
		if err := s.fetchBranchIntoBareRepo(repo.Path, branch); err != nil {
			return nil, nil, fmt.Errorf("failed to fetch branch %s: %v", branch, err)
		}
	}
	
	// Create new worktree with fun name
	funName := generateSessionName()
	// Creating worktree
	worktree, err := s.createWorktreeInternalForRepo(repo, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree: %v", err)
	}
	
	// Save state
	s.saveState()
	
	log.Printf("✅ Worktree created for existing repository: %s", repo.ID)
	return repo, worktree, nil
}

// createWorktreeInternalForRepo creates a worktree for a specific repository
func (s *GitService) createWorktreeInternalForRepo(repo *models.Repository, source, name string, isInitial bool) (*models.Worktree, error) {
	id := uuid.New().String()
	
	// Extract repo name from repo ID (e.g., "owner/repo" -> "repo")
	repoParts := strings.Split(repo.ID, "/")
	repoName := repoParts[len(repoParts)-1]
	
	// All worktrees use repo/branch pattern for consistency
	worktreePath := filepath.Join(workspaceDir, repoName, name)
	
	// Create worktree with new branch using the fun name
	cmd := exec.Command("git", "-C", repo.Path, "worktree", "add", "-b", name, worktreePath, source)
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
	
	// Determine source branch (resolve if it's a commit or branch)
	sourceBranch := source
	if len(source) == 40 { // Looks like a commit hash
		// Try to find which branch contains this commit
		cmd = exec.Command("git", "-C", repo.Path, "branch", "--contains", source)
		branchOutput, err := cmd.Output()
		if err == nil {
			branches := strings.Split(strings.TrimSpace(string(branchOutput)), "\n")
			if len(branches) > 0 {
				// Use the first branch found, clean it up
				sourceBranch = strings.TrimSpace(strings.TrimPrefix(branches[0], "*"))
				sourceBranch = strings.TrimPrefix(sourceBranch, "origin/")
			}
		}
	}
	
	// Calculate commit count ahead of source
	commitCount := 0
	if sourceBranch != name { // Only count if different from current branch
		cmd = exec.Command("git", "-C", worktreePath, "rev-list", "--count", fmt.Sprintf("%s..HEAD", sourceBranch))
		countOutput, err := cmd.Output()
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(countOutput))); parseErr == nil {
				commitCount = count
			}
		}
	}

	// Extract repo name from repo ID (e.g., "owner/repo" -> "repo")
	repoParts = strings.Split(repo.ID, "/")
	repoName = repoParts[len(repoParts)-1]
	
	// Create display name with repo name prefix
	displayName := fmt.Sprintf("%s/%s", repoName, name)

	worktree := &models.Worktree{
		ID:           id,
		RepoID:       repo.ID,
		Name:         displayName,
		Path:         worktreePath,
		Branch:       name,
		SourceBranch: sourceBranch,
		CommitHash:   strings.TrimSpace(string(commitOutput)),
		CommitCount:  commitCount,
		IsDirty:      false,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}
	
	s.worktrees[id] = worktree
	
	if isInitial || len(s.worktrees) == 1 {
		// Update current symlink to point to the first/initial worktree
		s.updateCurrentSymlink(worktreePath)
	}
	
	// Git worktrees automatically sync to bare repository
	
	return worktree, nil
}

// unshallowRepository unshallows a specific branch in the background
func (s *GitService) unshallowRepository(barePath, branch string) {
	// Wait a bit before starting to avoid interfering with initial setup
	time.Sleep(5 * time.Second)
	
	// Only fetch the specific branch to be more efficient
	cmd := exec.Command("git", "-C", barePath, "fetch", "origin", "--unshallow", branch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	if output, err := cmd.CombinedOutput(); err != nil {
		// Silent failure - unshallow is optional optimization
		_ = output // Avoid unused variable
		_ = err
	}
}

// GetRepositoryByID returns a repository by its ID
func (s *GitService) GetRepositoryByID(repoID string) *models.Repository {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.repositories[repoID]
}

// ListRepositories returns all loaded repositories
func (s *GitService) ListRepositories() []*models.Repository {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	repos := make([]*models.Repository, 0, len(s.repositories))
	for _, repo := range s.repositories {
		repos = append(repos, repo)
	}
	return repos
}