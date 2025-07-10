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
	liveDir      = "/live"
	devRepoPath  = "/live/catnip" // Kept for backwards compatibility
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
	
	// Detect and load any local repositories in /live
	s.detectLocalRepos()
	
	// Commit sync service removed - git worktrees handle synchronization automatically
	
	return s
}

// CheckoutRepository clones a GitHub repository as a bare repo and creates initial worktree
func (s *GitService) CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	repoID := fmt.Sprintf("%s/%s", org, repo)
	
	// Handle local repo specially
	if strings.HasPrefix(repoID, "local/") {
		return s.handleLocalRepoWorktree(repoID, branch)
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
		
		// Update commit count and commits behind
		s.updateWorktreeStatusInternal(wt)
		
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
	
	// Add all local repositories
	s.mu.RLock()
	for repoID, _ := range s.repositories {
		if strings.HasPrefix(repoID, "local/") {
			// Extract the directory name from the repo ID
			dirName := strings.TrimPrefix(repoID, "local/")
			repos = append(repos, map[string]interface{}{
				"name":        dirName,
				"url":         repoID, // Just use the local repo ID directly
				"private":     false,
				"description": "Local repository (mounted)",
				"fullName":    repoID,
			})
		}
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

// detectLocalRepos scans /live for any Git repositories and loads them
func (s *GitService) detectLocalRepos() {
	// Check if /live directory exists
	if _, err := os.Stat(liveDir); os.IsNotExist(err) {
		log.Printf("📁 No /live directory found, skipping local repo detection")
		return
	}
	
	// Read all entries in /live
	entries, err := os.ReadDir(liveDir)
	if err != nil {
		log.Printf("❌ Failed to read /live directory: %v", err)
		return
	}
	
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		repoPath := filepath.Join(liveDir, entry.Name())
		gitPath := filepath.Join(repoPath, ".git")
		
		// Check if it's a git repository
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			continue
		}
		
		log.Printf("🔍 Detected local repository at %s", repoPath)
		
		// Create repository object
		repoID := fmt.Sprintf("local/%s", entry.Name())
		repo := &models.Repository{
			ID:            repoID,
			URL:           "file://" + repoPath,
			Path:          repoPath,
			DefaultBranch: s.getLocalRepoDefaultBranch(repoPath),
			CreatedAt:     time.Now(),
			LastAccessed:  time.Now(),
		}
		
		// Add to repositories map
		s.repositories[repoID] = repo
		
		log.Printf("✅ Local repository loaded: %s", repoID)
	}
}

// getLocalRepoDefaultBranch gets the current branch of a local repo
func (s *GitService) getLocalRepoDefaultBranch(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "branch", "--show-current")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ Could not get current branch for repo at %s, using fallback: main", repoPath)
		return "main"
	}
	
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "main"
	}
	
	return branch
}

// getDevRepoDefaultBranch gets the current branch of the dev repo (backwards compatibility)
func (s *GitService) getDevRepoDefaultBranch() string {
	return s.getLocalRepoDefaultBranch(devRepoPath)
}

// handleLocalRepoWorktree creates a worktree for any local repo
func (s *GitService) handleLocalRepoWorktree(repoID, branch string) (*models.Repository, *models.Worktree, error) {
	// Get the local repo from repositories map
	localRepo, exists := s.repositories[repoID]
	if !exists {
		return nil, nil, fmt.Errorf("local repository %s not found - it may not be mounted", repoID)
	}
	
	// If no branch specified, use current branch
	if branch == "" {
		branch = localRepo.DefaultBranch
	}
	
	// Check if branch exists in the local repo
	if !s.localRepoBranchExists(localRepo.Path, branch) {
		return nil, nil, fmt.Errorf("branch %s does not exist in repository %s", branch, repoID)
	}
	
	// Create new worktree with fun name
	funName := generateSessionName()
	
	// Create worktree for local repo
	worktree, err := s.createLocalRepoWorktree(localRepo, branch, funName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree for local repo: %v", err)
	}
	
	// Save state
	s.saveState()
	
	log.Printf("✅ Local repo worktree created: %s", worktree.Name)
	return localRepo, worktree, nil
}

// handleDevRepoWorktree creates a worktree for the dev repo (backwards compatibility)
func (s *GitService) handleDevRepoWorktree(branch string) (*models.Repository, *models.Worktree, error) {
	return s.handleLocalRepoWorktree("local/catnip", branch)
}

// localRepoBranchExists checks if a branch exists in a local repo
func (s *GitService) localRepoBranchExists(repoPath, branch string) bool {
	cmd := exec.Command("git", "-C", repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	return cmd.Run() == nil
}

// devRepoBranchExists checks if a branch exists in the dev repo (backwards compatibility)
func (s *GitService) devRepoBranchExists(branch string) bool {
	return s.localRepoBranchExists(devRepoPath, branch)
}

// createLocalRepoWorktree creates a worktree for any local repo
func (s *GitService) createLocalRepoWorktree(repo *models.Repository, branch, name string) (*models.Worktree, error) {
	id := uuid.New().String()
	
	// Extract directory name from repo ID (e.g., "local/myproject" -> "myproject")
	dirName := filepath.Base(repo.Path)
	
	// Create worktree path with repo directory prefix
	worktreePath := filepath.Join(workspaceDir, dirName, name)
	
	// Create worktree directory first
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %v", err)
	}
	
	// Create worktree with new branch using the fun name
	cmd := exec.Command("git", "-C", repo.Path, "worktree", "add", "-b", name, worktreePath, branch)
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
	
	// Create display name with repo directory prefix
	displayName := fmt.Sprintf("%s/%s", dirName, name)
	
	worktree := &models.Worktree{
		ID:           id,
		RepoID:       repo.ID,
		Name:         displayName,
		Path:         worktreePath,
		Branch:       name,
		SourceBranch: branch,
		CommitHash:   strings.TrimSpace(string(commitOutput)),
		CommitCount:  commitCount,
		CommitsBehind: 0, // Will be calculated later
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

// createDevRepoWorktree creates a worktree for the dev repo (backwards compatibility)
func (s *GitService) createDevRepoWorktree(repo *models.Repository, branch, name string) (*models.Worktree, error) {
	return s.createLocalRepoWorktree(repo, branch, name)
}

// GetRepositoryBranches returns the remote branches for a repository
func (s *GitService) GetRepositoryBranches(repoID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	repo, exists := s.repositories[repoID]
	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}
	
	// Handle local repos specially
	if strings.HasPrefix(repoID, "local/") {
		return s.getLocalRepoBranches(repo.Path)
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

// getLocalRepoBranches returns the local branches for any local repository
func (s *GitService) getLocalRepoBranches(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "branch")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get local repo branches: %v", err)
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

// updateWorktreeStatusInternal updates commit count and commits behind for a worktree (internal, no mutex)
func (s *GitService) updateWorktreeStatusInternal(worktree *models.Worktree) {
	// Update commit count and commits behind
	if worktree.SourceBranch != "" && worktree.SourceBranch != worktree.Branch {
		// For local repos, ensure we have the latest reference
		if strings.HasPrefix(worktree.RepoID, "local/") {
			// Get the local repo path
			repo, exists := s.repositories[worktree.RepoID]
			if exists {
				// Fetch latest from local main repo
				fetchCmd := exec.Command("git", "-C", worktree.Path, "fetch", repo.Path, fmt.Sprintf("%s:refs/remotes/live/%s", worktree.SourceBranch, worktree.SourceBranch))
				fetchCmd.Env = append(os.Environ(),
					"HOME=/home/catnip",
					"USER=catnip",
				)
				fetchCmd.Run() // Ignore errors for now
			}
		} else {
			// Fetch latest from origin for regular repos
			fetchCmd := exec.Command("git", "-C", worktree.Path, "fetch", "origin", worktree.SourceBranch)
			fetchCmd.Env = append(os.Environ(),
				"HOME=/home/catnip",
				"USER=catnip",
			)
			fetchCmd.Run() // Ignore errors for now
		}
		
		// Determine source reference based on repo type
		sourceRef := worktree.SourceBranch
		if strings.HasPrefix(worktree.RepoID, "local/") {
			sourceRef = fmt.Sprintf("live/%s", worktree.SourceBranch)
		} else {
			sourceRef = fmt.Sprintf("origin/%s", worktree.SourceBranch)
		}
		
		// Count commits ahead (our commits)
		cmd := exec.Command("git", "-C", worktree.Path, "rev-list", "--count", fmt.Sprintf("%s..HEAD", sourceRef))
		cmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)
		countOutput, err := cmd.Output()
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(countOutput))); parseErr == nil {
				worktree.CommitCount = count
			}
		}
		
		// Count commits behind (missing commits)
		cmd = exec.Command("git", "-C", worktree.Path, "rev-list", "--count", fmt.Sprintf("HEAD..%s", sourceRef))
		cmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)
		behindOutput, err := cmd.Output()
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(behindOutput))); parseErr == nil {
				worktree.CommitsBehind = count
			}
		}
	}
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
	
	// Update commit count and commits behind
	s.updateWorktreeStatusInternal(worktree)
	
	return nil
}

// SyncWorktree syncs a worktree with its source branch
func (s *GitService) SyncWorktree(worktreeID string, strategy string) error {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}
	
	// For local repo worktrees, sync directly from local main repo
	if strings.HasPrefix(worktree.RepoID, "local/") {
		return s.syncLocalWorktree(worktree, strategy)
	}
	
	// For regular repos, sync from origin
	return s.syncRegularWorktree(worktree, strategy)
}

// syncLocalWorktree syncs a local repo worktree with the local main repo
func (s *GitService) syncLocalWorktree(worktree *models.Worktree, strategy string) error {
	// Get the local repo path
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return fmt.Errorf("local repository %s not found", worktree.RepoID)
	}
	
	// Skip pulling from origin for local repo - we're working locally
	log.Printf("🔄 Syncing local worktree %s from local main repo", worktree.Name)
	
	// Fetch from the local main repo
	cmd := exec.Command("git", "-C", worktree.Path, "fetch", repo.Path, fmt.Sprintf("%s:refs/remotes/live/%s", worktree.SourceBranch, worktree.SourceBranch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch from main repo: %v\n%s", err, output)
	}
	
	// Apply the sync strategy
	switch strategy {
	case "merge":
		cmd = exec.Command("git", "-C", worktree.Path, "merge", fmt.Sprintf("live/%s", worktree.SourceBranch))
	case "rebase":
		cmd = exec.Command("git", "-C", worktree.Path, "rebase", fmt.Sprintf("live/%s", worktree.SourceBranch))
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy)
	}
	
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to %s: %v\n%s", strategy, err, output)
	}
	
	// Update worktree status
	s.UpdateWorktreeStatus(worktree.ID)
	
	log.Printf("✅ Synced worktree %s with %s strategy", worktree.Name, strategy)
	return nil
}

// syncRegularWorktree syncs a regular repo worktree with origin
func (s *GitService) syncRegularWorktree(worktree *models.Worktree, strategy string) error {
	// Fetch from origin
	cmd := exec.Command("git", "-C", worktree.Path, "fetch", "origin", worktree.SourceBranch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch from origin: %v\n%s", err, output)
	}
	
	// Apply the sync strategy
	switch strategy {
	case "merge":
		cmd = exec.Command("git", "-C", worktree.Path, "merge", fmt.Sprintf("origin/%s", worktree.SourceBranch))
	case "rebase":
		cmd = exec.Command("git", "-C", worktree.Path, "rebase", fmt.Sprintf("origin/%s", worktree.SourceBranch))
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy)
	}
	
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to %s: %v\n%s", strategy, err, output)
	}
	
	// Update worktree status
	s.UpdateWorktreeStatus(worktree.ID)
	
	log.Printf("✅ Synced worktree %s with %s strategy", worktree.Name, strategy)
	return nil
}

// MergeWorktreeToMain merges a local repo worktree's changes back to the main repository
func (s *GitService) MergeWorktreeToMain(worktreeID string) error {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}
	
	// Only works for local repos
	if !strings.HasPrefix(worktree.RepoID, "local/") {
		return fmt.Errorf("merge to main only supported for local repositories")
	}
	
	// Get the local repo
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return fmt.Errorf("local repository %s not found", worktree.RepoID)
	}
	
	log.Printf("🔄 Merging worktree %s back to main repository", worktree.Name)
	
	// First, push the worktree branch to the main repo
	cmd := exec.Command("git", "-C", worktree.Path, "push", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, worktree.Branch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push worktree branch to main repo: %v\n%s", err, output)
	}
	
	// Switch to the source branch in main repo and merge
	cmd = exec.Command("git", "-C", repo.Path, "checkout", worktree.SourceBranch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to checkout source branch in main repo: %v\n%s", err, output)
	}
	
	// Merge the worktree branch
	cmd = exec.Command("git", "-C", repo.Path, "merge", worktree.Branch, "--no-ff", "-m", fmt.Sprintf("Merge branch '%s' from worktree", worktree.Branch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to merge worktree branch: %v\n%s", err, output)
	}
	
	// Delete the feature branch from main repo (cleanup)
	cmd = exec.Command("git", "-C", repo.Path, "branch", "-d", worktree.Branch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	cmd.Run() // Ignore errors - branch might be in use
	
	log.Printf("✅ Merged worktree %s to main repository", worktree.Name)
	return nil
}

// CreateWorktreePreview creates a preview branch in the main repo for viewing changes outside container
func (s *GitService) CreateWorktreePreview(worktreeID string) error {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}
	
	// Only works for local repos
	if !strings.HasPrefix(worktree.RepoID, "local/") {
		return fmt.Errorf("preview only supported for local repositories")
	}
	
	// Get the local repo
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return fmt.Errorf("local repository %s not found", worktree.RepoID)
	}
	
	previewBranchName := fmt.Sprintf("preview/%s", worktree.Branch)
	log.Printf("🔍 Creating preview branch %s for worktree %s", previewBranchName, worktree.Name)
	
	// Check if there are uncommitted changes (staged, unstaged, or untracked)
	hasUncommittedChanges, err := s.hasUncommittedChanges(worktree.Path)
	if err != nil {
		return fmt.Errorf("failed to check for uncommitted changes: %v", err)
	}
	
	var tempCommitHash string
	if hasUncommittedChanges {
		// Create a temporary commit with all uncommitted changes
		tempCommitHash, err = s.createTemporaryCommit(worktree.Path)
		if err != nil {
			return fmt.Errorf("failed to create temporary commit: %v", err)
		}
		defer func() {
			// Reset to remove the temporary commit after pushing
			if tempCommitHash != "" {
				resetCmd := exec.Command("git", "-C", worktree.Path, "reset", "--mixed", "HEAD~1")
				resetCmd.Env = append(os.Environ(), "HOME=/home/catnip", "USER=catnip")
				resetCmd.Run()
			}
		}()
	}
	
	// Push the worktree branch to a preview branch in main repo
	cmd := exec.Command("git", "-C", worktree.Path, "push", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, previewBranchName))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create preview branch: %v\n%s", err, output)
	}
	
	if hasUncommittedChanges {
		log.Printf("✅ Preview branch %s created with uncommitted changes - you can now checkout this branch outside the container", previewBranchName)
	} else {
		log.Printf("✅ Preview branch %s created - you can now checkout this branch outside the container", previewBranchName)
	}
	return nil
}

// hasUncommittedChanges checks if the worktree has any uncommitted changes
func (s *GitService) hasUncommittedChanges(worktreePath string) (bool, error) {
	// Check for staged changes
	cmd := exec.Command("git", "-C", worktreePath, "diff", "--cached", "--quiet")
	cmd.Env = append(os.Environ(), "HOME=/home/catnip", "USER=catnip")
	if cmd.Run() != nil {
		return true, nil // Has staged changes
	}
	
	// Check for unstaged changes
	cmd = exec.Command("git", "-C", worktreePath, "diff", "--quiet")
	cmd.Env = append(os.Environ(), "HOME=/home/catnip", "USER=catnip")
	if cmd.Run() != nil {
		return true, nil // Has unstaged changes
	}
	
	// Check for untracked files
	cmd = exec.Command("git", "-C", worktreePath, "ls-files", "--others", "--exclude-standard")
	cmd.Env = append(os.Environ(), "HOME=/home/catnip", "USER=catnip")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check for untracked files: %v", err)
	}
	
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// createTemporaryCommit creates a temporary commit with all uncommitted changes
func (s *GitService) createTemporaryCommit(worktreePath string) (string, error) {
	// Add all changes (staged, unstaged, and untracked)
	cmd := exec.Command("git", "-C", worktreePath, "add", ".")
	cmd.Env = append(os.Environ(), "HOME=/home/catnip", "USER=catnip")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to stage changes: %v\n%s", err, output)
	}
	
	// Create the commit
	cmd = exec.Command("git", "-C", worktreePath, "commit", "-m", "Preview: Include all uncommitted changes")
	cmd.Env = append(os.Environ(), "HOME=/home/catnip", "USER=catnip")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to create temporary commit: %v\n%s", err, output)
	}
	
	// Get the commit hash
	cmd = exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(), "HOME=/home/catnip", "USER=catnip")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %v", err)
	}
	
	commitHash := strings.TrimSpace(string(output))
	log.Printf("📝 Created temporary commit %s with uncommitted changes", commitHash[:8])
	return commitHash, nil
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
	
	// Handle local repos specially (they don't have a bare repo)
	if strings.HasPrefix(repo.ID, "local/") {
		return s.handleLocalRepoWorktree(repo.ID, branch)
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