package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

const (
	defaultWorkspaceDir = "/workspace"
	liveDir             = "/live"
	devRepoPath         = "/live/catnip" // Kept for backwards compatibility
)

// getWorkspaceDir returns the workspace directory, configurable via CATNIP_WORKSPACE_DIR
func getWorkspaceDir() string {
	if dir := os.Getenv("CATNIP_WORKSPACE_DIR"); dir != "" {
		return dir
	}
	return defaultWorkspaceDir
}

// getGitStateDir returns the git state directory based on workspace dir
func getGitStateDir() string {
	return filepath.Join(getWorkspaceDir(), ".git-state")
}

// generateUniqueSessionName generates a unique session name that doesn't already exist as a branch
func (s *GitService) generateUniqueSessionName(repoPath string) string {
	// Use the shared function with GitService's branch checking logic
	return git.GenerateUniqueSessionName(func(name string) bool {
		return s.branchExists(repoPath, name, false)
	})
}

// isCatnipBranch checks if a branch name has a catnip/ prefix
func isCatnipBranch(branchName string) bool {
	return git.IsCatnipBranch(branchName)
}

// cleanupUnusedBranches removes catnip branches that have no commits
func (s *GitService) cleanupUnusedBranches() {
	log.Printf("🧹 Starting cleanup of unused catnip branches...")

	s.mu.RLock()
	repos := make([]*models.Repository, 0, len(s.repositories))
	for _, repo := range s.repositories {
		repos = append(repos, repo)
	}
	s.mu.RUnlock()

	totalDeleted := 0

	for _, repo := range repos {
		// List all branches in the bare repository
		cmd := exec.Command("git", "-C", repo.Path, "branch", "-a")
		output, err := cmd.Output()
		if err != nil {
			log.Printf("⚠️  Failed to list branches for %s: %v", repo.ID, err)
			continue
		}

		branches := strings.Split(strings.TrimSpace(string(output)), "\n")
		deletedInRepo := 0

		for _, branch := range branches {
			// Clean up branch name
			branchName := strings.TrimSpace(branch)
			branchName = strings.TrimPrefix(branchName, "*")
			branchName = strings.TrimPrefix(branchName, "+")
			branchName = strings.TrimSpace(branchName)
			branchName = strings.TrimPrefix(branchName, "remotes/origin/")

			// Skip if not a catnip branch
			if !isCatnipBranch(branchName) {
				continue
			}

			// Check if branch has any commits different from its parent
			// First, try to find the merge-base with main/master
			var baseRef string
			for _, ref := range []string{"main", "master"} {
				cmd = exec.Command("git", "-C", repo.Path, "rev-parse", "--verify", ref)
				if err := cmd.Run(); err == nil {
					baseRef = ref
					break
				}
			}

			if baseRef == "" {
				continue // Skip if we can't find a base branch
			}

			// Check if branch exists locally
			cmd = exec.Command("git", "-C", repo.Path, "rev-parse", "--verify", branchName)
			if err := cmd.Run(); err != nil {
				continue // Branch doesn't exist locally
			}

			// Count commits ahead of base
			cmd = exec.Command("git", "-C", repo.Path, "rev-list", "--count", fmt.Sprintf("%s..%s", baseRef, branchName))
			output, err := cmd.Output()
			if err != nil {
				continue // Skip on error
			}

			commitCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
			if err != nil || commitCount > 0 {
				continue // Skip if there are commits or error parsing
			}

			// Also check if there's an active worktree using this branch
			worktreeCmd := exec.Command("git", "-C", repo.Path, "worktree", "list", "--porcelain")
			worktreeOutput, err := worktreeCmd.Output()
			if err == nil && strings.Contains(string(worktreeOutput), fmt.Sprintf("branch refs/heads/%s", branchName)) {
				continue // Skip if branch is currently checked out in a worktree
			}

			// Delete the branch (local)
			cmd = exec.Command("git", "-C", repo.Path, "branch", "-D", branchName)
			if err := cmd.Run(); err == nil {
				deletedInRepo++
				totalDeleted++
				log.Printf("🗑️  Deleted unused branch: %s in %s", branchName, repo.ID)
			}
		}

		if deletedInRepo > 0 {
			log.Printf("✅ Cleaned up %d unused branches in %s", deletedInRepo, repo.ID)
		}
	}

	if totalDeleted > 0 {
		log.Printf("🧹 Cleanup complete: removed %d unused catnip branches", totalDeleted)
	} else {
		log.Printf("✅ No unused catnip branches found")
	}
}

// GitService manages multiple Git repositories and their worktrees
type GitService struct {
	repositories map[string]*models.Repository // key: repoID (e.g., "owner/repo")
	worktrees    map[string]*models.Worktree   // key: worktree ID
	manager      git.Manager                   // Delegate for core git operations
	helper       *git.ServiceHelper            // NEW: Helper for extracted git operations
	mu           sync.RWMutex
}

// Helper functions for standardized command execution

// Repository type detection helpers
func (s *GitService) isLocalRepo(repoID string) bool {
	return strings.HasPrefix(repoID, "local/")
}

// getSourceRef returns the appropriate source reference for a worktree
func (s *GitService) getSourceRef(worktree *models.Worktree) string {
	if s.isLocalRepo(worktree.RepoID) {
		return fmt.Sprintf("live/%s", worktree.SourceBranch)
	}
	return fmt.Sprintf("origin/%s", worktree.SourceBranch)
}

// execCommand executes any command with standard environment (DEPRECATED: use s.helper.ExecuteCommand)
func (s *GitService) execCommand(command string, args ...string) *exec.Cmd {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	return cmd
}

// execGitCommand executes a git command with standard environment (DEPRECATED: use s.helper.ExecuteGit)
func (s *GitService) execGitCommand(workingDir string, args ...string) *exec.Cmd {
	// For backward compatibility during migration
	cmd := exec.Command("git", args...)
	if workingDir != "" {
		cmd.Args = append([]string{"git", "-C", workingDir}, args...)
	}
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	return cmd
}

// runGitCommand runs a git command and returns output (DEPRECATED: use s.helper.ExecuteGit)
func (s *GitService) runGitCommand(workingDir string, args ...string) ([]byte, error) {
	return s.helper.ExecuteGit(workingDir, args...)
}

// Removed RemoteURLManager - functionality moved to git.URLManager

// PushStrategy defines the strategy for pushing branches (DEPRECATED: use git.PushStrategy)
type PushStrategy struct {
	Branch       string // Branch to push (defaults to worktree.Branch)
	Remote       string // Remote name (defaults to "origin")
	RemoteURL    string // Remote URL (optional, for local repos)
	SyncOnFail   bool   // Whether to sync with upstream on push failure
	SetUpstream  bool   // Whether to set upstream (-u flag)
	ConvertHTTPS bool   // Whether to convert SSH URLs to HTTPS (includes workflow detection)
}

// pushBranch unified push method with strategy pattern
func (s *GitService) pushBranch(worktree *models.Worktree, repo *models.Repository, strategy PushStrategy) error {
	// Convert to git package strategy
	gitStrategy := git.PushStrategy{
		Branch:       strategy.Branch,
		Remote:       strategy.Remote,
		RemoteURL:    strategy.RemoteURL,
		SyncOnFail:   false, // We handle sync retry at this level
		SetUpstream:  strategy.SetUpstream,
		ConvertHTTPS: strategy.ConvertHTTPS,
	}

	// Set defaults
	if gitStrategy.Branch == "" {
		gitStrategy.Branch = worktree.Branch
	}
	if gitStrategy.Remote == "" {
		gitStrategy.Remote = "origin"
	}

	// Execute push using helper
	err := s.helper.PushWithStrategy(worktree.Path, gitStrategy)

	// Handle push failure with sync retry (if requested)
	if err != nil && strategy.SyncOnFail && git.IsPushRejected(err, err.Error()) {
		log.Printf("🔄 Push rejected due to upstream changes, syncing and retrying")

		// Sync with upstream
		if syncErr := s.syncBranchWithUpstream(worktree); syncErr != nil {
			return fmt.Errorf("failed to sync with upstream: %v", syncErr)
		}

		// Retry the push (without sync this time to avoid infinite loop)
		retryStrategy := strategy
		retryStrategy.SyncOnFail = false
		return s.pushBranch(worktree, repo, retryStrategy)
	}

	return err
}

// parseGitHubURL parses a GitHub URL and returns owner/repo
func (s *GitService) parseGitHubURL(url string) (string, error) {
	if strings.HasPrefix(url, "git@github.com:") {
		parts := strings.TrimPrefix(url, "git@github.com:")
		return strings.TrimSuffix(parts, ".git"), nil
	}
	if strings.Contains(url, "github.com/") {
		parts := strings.Split(url, "github.com/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid GitHub URL format")
		}
		return strings.TrimSuffix(parts[1], ".git"), nil
	}
	return "", fmt.Errorf("URL does not appear to be a GitHub repository")
}

// branchExists checks if a branch exists in a repository with configurable options
func (s *GitService) branchExists(repoPath, branch string, isRemote bool) bool {
	return s.helper.BranchExists(repoPath, branch, isRemote)
}

// Removed branchExistsWithOptions - use s.helper.BranchOps.BranchExists directly

// getCommitCount counts commits between two refs
func (s *GitService) getCommitCount(repoPath, fromRef, toRef string) (int, error) {
	return s.helper.GetCommitCount(repoPath, fromRef, toRef)
}

// getRemoteURL gets the remote URL for a repository
func (s *GitService) getRemoteURL(repoPath string) (string, error) {
	return s.helper.GetRemoteURL(repoPath)
}

// getDefaultBranch gets the default branch from a repository
func (s *GitService) getDefaultBranch(repoPath string) (string, error) {
	return s.helper.GetDefaultBranch(repoPath)
}

// fetchBranch unified fetch method with strategy pattern
func (s *GitService) fetchBranch(repoPath string, strategy git.FetchStrategy) error {
	return s.helper.FetchWithStrategy(repoPath, strategy)
}

// isDirty checks if a worktree has uncommitted changes
func (s *GitService) isDirty(worktreePath string) bool {
	return s.helper.IsDirty(worktreePath)
}

// hasConflicts checks if a worktree is in a conflicted state (rebase/merge in progress)
func (s *GitService) hasConflicts(worktreePath string) bool {
	return s.helper.HasConflicts(worktreePath)
}

// NewGitService creates a new Git service instance
func NewGitService() *GitService {
	return NewGitServiceWithHelper(git.NewServiceHelper())
}

// NewGitServiceWithHelper creates a new Git service instance with injectable git operations
func NewGitServiceWithHelper(helper *git.ServiceHelper) *GitService {
	// Create the underlying git manager
	manager := git.NewManager()

	s := &GitService{
		repositories: make(map[string]*models.Repository),
		worktrees:    make(map[string]*models.Worktree),
		manager:      manager,
		helper:       helper, // Use injected helper
	}

	// Ensure workspace directory exists
	_ = os.MkdirAll(getWorkspaceDir(), 0755)
	_ = os.MkdirAll(getGitStateDir(), 0755)

	// Configure Git to use gh as credential helper if available
	s.configureGitCredentials()

	// Load existing state (repositories and worktrees) from previous sessions
	if err := s.loadState(); err != nil {
		log.Printf("⚠️ Failed to load GitService state: %v", err)
	}

	// Detect and load any local repositories in /live
	s.detectLocalRepos()

	// Clean up unused catnip branches (skip in dev mode to avoid deleting active branches)
	if os.Getenv("CATNIP_DEV") != "true" {
		s.cleanupUnusedBranches()
	} else {
		log.Printf("🔧 Skipping branch cleanup in dev mode")
	}

	return s
}

// CheckoutRepository clones a GitHub repository as a bare repo and creates initial worktree
func (s *GitService) CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	repoID := fmt.Sprintf("%s/%s", org, repo)

	// Handle local repo specially
	if s.isLocalRepo(repoID) {
		return s.handleLocalRepoWorktree(repoID, branch)
	}

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", org, repo)
	repoName := strings.ReplaceAll(repo, "/", "-")
	barePath := filepath.Join(getWorkspaceDir(), fmt.Sprintf("%s.git", repoName))

	// Check if a directory is already mounted at the repo location
	if s.isRepoMounted(getWorkspaceDir(), repoName) {
		return nil, nil, fmt.Errorf("a repository already exists at %s (possibly mounted)",
			filepath.Join(getWorkspaceDir(), repoName))
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

// isRepoMounted checks if a repo directory is already mounted
func (s *GitService) isRepoMounted(workspaceDir, repoName string) bool {
	potentialMountPath := filepath.Join(workspaceDir, repoName)
	if info, err := os.Stat(potentialMountPath); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(potentialMountPath, ".git")); err == nil {
			log.Printf("⚠️ Found existing Git repository at %s, skipping checkout", potentialMountPath)
			return true
		}
	}
	return false
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
		defaultBranch, err := s.getDefaultBranch(barePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get default branch: %v", err)
		}

		repo = &models.Repository{
			ID:            repoID,
			URL:           repoURL,
			Path:          barePath,
			DefaultBranch: defaultBranch,
			CreatedAt:     time.Now(),
			LastAccessed:  time.Now(),
		}
		s.repositories[repoID] = repo
	}

	// If no branch specified, use default
	if branch == "" {
		branch = repo.DefaultBranch
	}

	// Check if the requested branch exists in the bare repo
	if !s.branchExists(barePath, branch, true) {
		log.Printf("🔄 Branch %s not found, fetching from remote", branch)
		if err := s.fetchBranch(barePath, git.FetchStrategy{
			Branch:         branch,
			Depth:          1,
			UpdateLocalRef: true,
		}); err != nil {
			return nil, nil, fmt.Errorf("failed to fetch branch %s: %v", branch, err)
		}
	}

	// Create new worktree with fun name
	funName := s.generateUniqueSessionName(repo.Path)
	worktree, err := s.createWorktreeInternalForRepo(repo, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree: %v", err)
	}

	_ = s.saveState()
	log.Printf("✅ Worktree created from existing repository: %s", repoID)
	return repo, worktree, nil
}

// cloneNewRepository clones a new bare repository
func (s *GitService) cloneNewRepository(repoID, repoURL, barePath, branch string) (*models.Repository, *models.Worktree, error) {
	// Clone as bare repository with shallow depth
	args := []string{"clone", "--bare", "--depth", "1", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, barePath)

	if _, err := s.runGitCommand("", args...); err != nil {
		return nil, nil, fmt.Errorf("failed to clone repository: %v", err)
	}

	// Get default branch if not specified
	if branch == "" {
		var err error
		branch, err = s.getDefaultBranch(barePath)
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

	s.repositories[repoID] = repository

	// Start background unshallow process for the requested branch
	go s.unshallowRepository(barePath, branch)

	// Create initial worktree with fun name to avoid conflicts with local branches
	funName := s.generateUniqueSessionName(repository.Path)
	worktree, err := s.createWorktreeInternalForRepo(repository, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create initial worktree: %v", err)
	}

	_ = s.saveState()
	log.Printf("✅ Repository cloned successfully: %s", repository.ID)
	return repository, worktree, nil
}

// ListWorktrees returns all worktrees
func (s *GitService) ListWorktrees() []*models.Worktree {
	s.mu.RLock()
	defer s.mu.RUnlock()

	worktrees := make([]*models.Worktree, 0, len(s.worktrees))
	for _, wt := range s.worktrees {
		// Update dirty status and conflict status
		wt.IsDirty = s.isDirty(wt.Path)
		wt.HasConflicts = s.hasConflicts(wt.Path)

		// Update commit count and commits behind without fetching
		s.updateWorktreeStatusInternal(wt, false)

		worktrees = append(worktrees, wt)
	}

	return worktrees
}

// GetStatus returns the current Git status
func (s *GitService) GetStatus() *models.GitStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &models.GitStatus{
		Repositories:  s.repositories, // All repositories
		WorktreeCount: len(s.worktrees),
	}
}

// updateCurrentSymlink updates the /workspace/current symlink
func (s *GitService) updateCurrentSymlink(targetPath string) error {
	currentPath := filepath.Join(getWorkspaceDir(), "current")

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

	return os.WriteFile(filepath.Join(getGitStateDir(), "state.json"), data, 0644)
}

func (s *GitService) loadState() error {
	data, err := os.ReadFile(filepath.Join(getGitStateDir(), "state.json"))
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

	return getWorkspaceDir() // fallback
}

// configureGitCredentials sets up Git to use gh CLI for GitHub authentication
func (s *GitService) configureGitCredentials() {
	// Check if gh CLI is authenticated
	cmd := s.execCommand("gh", "auth", "status")

	if err := cmd.Run(); err != nil {
		log.Printf("ℹ️ GitHub CLI not authenticated, Git operations will only work with public repositories")
		return
	}

	log.Printf("🔐 Configuring Git to use GitHub CLI for authentication")

	// Configure Git to use gh as credential helper for GitHub
	configCmd := s.execCommand("git", "config", "--global", "credential.https://github.com.helper", "!gh auth git-credential")

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
	for repoID := range s.repositories {
		if s.isLocalRepo(repoID) {
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
	cmd := s.execCommand("gh", "repo", "list", "--limit", "100", "--json", "name,url,isPrivate,description,owner")

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

		// Check if any worktrees exist for this repo
		if s.shouldCreateInitialWorktree(repoID) {
			log.Printf("🌱 Creating initial worktree for %s", repoID)
			if _, worktree, err := s.handleLocalRepoWorktree(repoID, repo.DefaultBranch); err != nil {
				log.Printf("❌ Failed to create initial worktree for %s: %v", repoID, err)
			} else {
				log.Printf("✅ Initial worktree created: %s", worktree.Name)
			}
		}
	}
}

// shouldCreateInitialWorktree checks if we should create an initial worktree for a repo
func (s *GitService) shouldCreateInitialWorktree(repoID string) bool {
	// Check if any worktrees exist for this repo in /workspace
	dirName := filepath.Base(strings.TrimPrefix(repoID, "local/"))
	repoWorkspaceDir := filepath.Join(getWorkspaceDir(), dirName)

	// Check if the repo workspace directory exists and has any worktrees
	if entries, err := os.ReadDir(repoWorkspaceDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				// Check if this directory is a valid git worktree
				if _, err := os.Stat(filepath.Join(repoWorkspaceDir, entry.Name(), ".git")); err == nil {
					log.Printf("🔍 Found existing worktree for %s: %s", repoID, entry.Name())
					return false
				}
			}
		}
	}

	log.Printf("🔍 No existing worktrees found for %s, will create initial worktree", repoID)
	return true
}

// getLocalRepoDefaultBranch gets the current branch of a local repo
func (s *GitService) getLocalRepoDefaultBranch(repoPath string) string {
	output, err := s.runGitCommand(repoPath, "branch", "--show-current")
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
	if !s.branchExists(localRepo.Path, branch, false) {
		return nil, nil, fmt.Errorf("branch %s does not exist in repository %s", branch, repoID)
	}

	// Create new worktree with fun name
	funName := s.generateUniqueSessionName(localRepo.Path)

	// Create worktree for local repo
	worktree, err := s.createLocalRepoWorktree(localRepo, branch, funName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree for local repo: %v", err)
	}

	// Save state
	_ = s.saveState()

	log.Printf("✅ Local repo worktree created: %s from branch %s", worktree.Name, worktree.SourceBranch)
	return localRepo, worktree, nil
}

// createLocalRepoWorktree creates a worktree for any local repo
func (s *GitService) createLocalRepoWorktree(repo *models.Repository, branch, name string) (*models.Worktree, error) {
	id := uuid.New().String()

	// Extract directory name from repo ID (e.g., "local/myproject" -> "myproject")
	dirName := filepath.Base(repo.Path)

	// Create worktree path with repo directory prefix
	// Extract workspace name (remove catnip/ prefix for filesystem paths)
	workspaceName := git.ExtractWorkspaceName(name)
	worktreePath := filepath.Join(getWorkspaceDir(), dirName, workspaceName)

	// Create worktree directory first
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %v", err)
	}

	// Create worktree with new branch using the fun name
	cmd := s.execGitCommand(repo.Path, "worktree", "add", "-b", name, worktreePath, branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v\n%s", err, output)
	}

	// Add the "live" remote to the worktree pointing back to the main repo
	// This allows status updates to fetch latest changes from the main repo
	addRemoteCmd := s.execGitCommand(worktreePath, "remote", "add", "live", repo.Path)
	if output, err := addRemoteCmd.CombinedOutput(); err != nil {
		log.Printf("⚠️ Failed to add live remote: %v\n%s", err, output)
	} else {
		// Fetch the source branch from the live remote to get latest state
		log.Printf("🔄 Fetching latest %s from live remote", branch)
		fetchCmd := s.execGitCommand(worktreePath, "fetch", "live", branch)
		if output, err := fetchCmd.CombinedOutput(); err != nil {
			log.Printf("⚠️ Failed to fetch %s from live remote: %v\n%s", branch, err, output)
		}
	}

	// Get current commit hash
	cmd = s.execGitCommand(worktreePath, "rev-parse", "HEAD")
	commitOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}

	// Clean up branch name to ensure it's a proper source branch
	// Remove any git prefixes that might have been passed in
	sourceBranch := strings.TrimSpace(branch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "*")
	sourceBranch = strings.TrimPrefix(sourceBranch, "+")
	sourceBranch = strings.TrimSpace(sourceBranch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "origin/")

	// Calculate commit count ahead of source
	commitCount := 0
	if sourceBranch != name { // Only count if different from current branch
		cmd = s.execGitCommand(worktreePath, "rev-list", "--count", fmt.Sprintf("%s..HEAD", sourceBranch))
		countOutput, err := cmd.Output()
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(countOutput))); parseErr == nil {
				commitCount = count
			}
		}
	}

	// Create display name with repo directory prefix (use already extracted workspaceName from line 1102)
	displayName := fmt.Sprintf("%s/%s", dirName, workspaceName)

	worktree := &models.Worktree{
		ID:            id,
		RepoID:        repo.ID,
		Name:          displayName,
		Path:          worktreePath,
		Branch:        name,
		SourceBranch:  sourceBranch,
		CommitHash:    strings.TrimSpace(string(commitOutput)),
		CommitCount:   commitCount,
		CommitsBehind: 0, // Will be calculated later
		IsDirty:       false,
		HasConflicts:  false,
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
	}

	s.worktrees[id] = worktree

	// Update current symlink to point to this worktree if it's the first one
	if len(s.worktrees) == 1 {
		_ = s.updateCurrentSymlink(worktreePath)
	}

	return worktree, nil
}

// getLocalRepoBranches returns the local branches for a local repository
func (s *GitService) getLocalRepoBranches(repoPath string) ([]string, error) {
	output, err := s.runGitCommand(repoPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("failed to get local branches: %w", err)
	}

	var branches []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}

	return branches, nil
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
	if s.isLocalRepo(repoID) {
		return s.getLocalRepoBranches(repo.Path)
	}

	// Start with the default branch
	branches := []string{repo.DefaultBranch}
	branchSet := map[string]bool{repo.DefaultBranch: true}

	cmd := s.execGitCommand(repo.Path, "branch", "-r")

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

// DeleteWorktree removes a worktree
func (s *GitService) DeleteWorktree(worktreeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	worktree, exists := s.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Get repository for worktree deletion
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return fmt.Errorf("repository %s not found", worktree.RepoID)
	}

	log.Printf("🗑️ Starting comprehensive cleanup for worktree %s", worktree.Name)

	// Step 1: Remove the worktree directory first (this also removes git worktree registration)
	cmd := s.execGitCommand(repo.Path, "worktree", "remove", "--force", worktree.Path)

	if err := cmd.Run(); err != nil {
		log.Printf("⚠️ Failed to remove worktree directory (continuing with cleanup): %v", err)
		// Continue with cleanup even if worktree removal fails
	} else {
		log.Printf("✅ Removed worktree directory: %s", worktree.Path)
	}

	// Step 2: Remove the worktree branch from the repository
	if worktree.Branch != "" && worktree.Branch != worktree.SourceBranch {
		cmd = s.execGitCommand(repo.Path, "branch", "-D", worktree.Branch)
		if err := cmd.Run(); err != nil {
			log.Printf("⚠️ Failed to remove branch %s (may not exist or be in use): %v", worktree.Branch, err)
		} else {
			log.Printf("✅ Removed branch: %s", worktree.Branch)
		}
	}

	// Step 3: Remove preview branch if it exists
	previewBranchName := fmt.Sprintf("preview/%s", worktree.Branch)
	cmd = s.execGitCommand(repo.Path, "branch", "-D", previewBranchName)
	if err := cmd.Run(); err != nil {
		// Preview branch might not exist, don't log as warning
		log.Printf("ℹ️ No preview branch to remove: %s", previewBranchName)
	} else {
		log.Printf("✅ Removed preview branch: %s", previewBranchName)
	}

	// Step 4: Clean up any active PTY sessions for this worktree
	s.cleanupActiveSessions(worktree.Path)

	// Step 5: Force remove any remaining files in the worktree directory
	if _, err := os.Stat(worktree.Path); err == nil {
		if removeErr := os.RemoveAll(worktree.Path); removeErr != nil {
			log.Printf("⚠️ Failed to force remove worktree directory %s: %v", worktree.Path, removeErr)
		} else {
			log.Printf("✅ Force removed remaining worktree directory: %s", worktree.Path)
		}
	}

	// Step 6: Remove from memory
	delete(s.worktrees, worktreeID)

	// Step 7: Run git garbage collection to clean up dangling objects
	gcCmd := s.execGitCommand(repo.Path, "gc", "--prune=now")
	if err := gcCmd.Run(); err != nil {
		log.Printf("⚠️ Failed to run garbage collection after worktree deletion: %v", err)
	} else {
		log.Printf("✅ Ran garbage collection to clean up dangling objects")
	}

	// Step 8: Save state
	_ = s.saveState()

	log.Printf("✅ Completed comprehensive cleanup for worktree %s", worktree.Name)
	return nil
}

// CleanupMergedWorktrees removes worktrees that have been fully merged into their source branch
func (s *GitService) CleanupMergedWorktrees() (int, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var cleanedUp []string
	var errors []error

	log.Printf("🧹 Starting cleanup of merged worktrees, checking %d worktrees", len(s.worktrees))

	for worktreeID, worktree := range s.worktrees {
		log.Printf("🔍 Checking worktree %s: dirty=%v, conflicts=%v, commits_ahead=%d, source=%s",
			worktree.Name, worktree.IsDirty, worktree.HasConflicts, worktree.CommitCount, worktree.SourceBranch)

		// Skip if worktree has uncommitted changes or conflicts
		if worktree.IsDirty {
			log.Printf("⏭️  Skipping cleanup of dirty worktree: %s", worktree.Name)
			continue
		}
		if worktree.HasConflicts {
			log.Printf("⏭️  Skipping cleanup of conflicted worktree: %s", worktree.Name)
			continue
		}

		// Skip if worktree has commits ahead of source
		if worktree.CommitCount > 0 {
			log.Printf("⏭️  Skipping cleanup of worktree with %d commits ahead: %s", worktree.CommitCount, worktree.Name)
			continue
		}

		// Check if the worktree branch exists in the source repo
		repo, exists := s.repositories[worktree.RepoID]
		if !exists {
			continue
		}

		// For local repos, check if the worktree branch no longer exists or if it matches the source branch
		isLocal := s.isLocalRepo(worktree.RepoID)
		var isMerged bool

		if isLocal {
			log.Printf("🔍 Checking local worktree %s: branch=%s, source=%s", worktree.Name, worktree.Branch, worktree.SourceBranch)

			// For local repos, check if the branch exists in the main repo
			// If it doesn't exist, it was likely deleted after merge
			branchExistsCmd := s.execGitCommand(repo.Path, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", worktree.Branch))
			branchExists := branchExistsCmd.Run() == nil

			if !branchExists {
				log.Printf("✅ Branch %s no longer exists in main repo (likely merged and deleted)", worktree.Branch)
				isMerged = true
			} else {
				// Branch still exists, check if it's merged
				mergedCmd := s.execGitCommand(repo.Path, "branch", "--merged", worktree.SourceBranch)
				output, err := mergedCmd.Output()
				if err != nil {
					log.Printf("⚠️ Failed to check merged status for %s: %v", worktree.Name, err)
					continue
				}

				mergedBranches := strings.Split(string(output), "\n")
				for _, branch := range mergedBranches {
					// Handle both regular branches and worktree branches (marked with +)
					branch = strings.TrimSpace(branch)
					branch = strings.TrimPrefix(branch, "*") // Current branch indicator
					branch = strings.TrimPrefix(branch, "+") // Worktree branch indicator
					branch = strings.TrimSpace(branch)
					if branch == worktree.Branch {
						isMerged = true
						log.Printf("✅ Found %s in merged branches list", worktree.Branch)
						break
					}
				}
			}
		} else {
			// Regular repo logic (existing code)
			log.Printf("🔍 Checking if branch %s is merged into %s in repo %s", worktree.Branch, worktree.SourceBranch, repo.Path)
			mergedCmd := s.execGitCommand(repo.Path, "branch", "--merged", worktree.SourceBranch)
			output, err := mergedCmd.Output()
			if err != nil {
				log.Printf("⚠️ Failed to check merged status for %s: %v", worktree.Name, err)
				continue
			}

			// Check if our branch appears in the merged branches list
			mergedBranches := strings.Split(string(output), "\n")
			log.Printf("📋 Merged branches into %s: %d branches found", worktree.SourceBranch, len(mergedBranches))

			for _, branch := range mergedBranches {
				// Handle both regular branches and worktree branches (marked with +)
				branch = strings.TrimSpace(branch)
				branch = strings.TrimPrefix(branch, "*") // Current branch indicator
				branch = strings.TrimPrefix(branch, "+") // Worktree branch indicator
				branch = strings.TrimSpace(branch)
				if branch == worktree.Branch {
					isMerged = true
					log.Printf("✅ Found %s in merged branches list", worktree.Branch)
					break
				}
			}
		}

		if !isMerged {
			log.Printf("❌ Branch %s not eligible for cleanup", worktree.Branch)
		}

		if isMerged {
			log.Printf("🧹 Found merged worktree to cleanup: %s", worktree.Name)

			// Use the existing deletion logic but don't hold the mutex
			s.mu.Unlock()
			if cleanupErr := s.DeleteWorktree(worktreeID); cleanupErr != nil {
				errors = append(errors, fmt.Errorf("failed to cleanup worktree %s: %v", worktree.Name, cleanupErr))
			} else {
				cleanedUp = append(cleanedUp, worktree.Name)
			}
			s.mu.Lock()
		}
	}

	if len(cleanedUp) > 0 {
		log.Printf("✅ Cleaned up %d merged worktrees: %s", len(cleanedUp), strings.Join(cleanedUp, ", "))
	}

	if len(errors) > 0 {
		return len(cleanedUp), cleanedUp, fmt.Errorf("cleanup completed with %d errors: %v", len(errors), errors)
	}

	return len(cleanedUp), cleanedUp, nil
}

// cleanupActiveSessions attempts to cleanup any active terminal sessions for this worktree
func (s *GitService) cleanupActiveSessions(worktreePath string) {
	// Kill any processes that might be running in the worktree directory
	// This is a best-effort cleanup
	cmd := s.execCommand("pkill", "-f", worktreePath)
	if err := cmd.Run(); err != nil {
		// Don't log this as an error since it's common for no processes to be found
		log.Printf("ℹ️ No active processes found for worktree path: %s", worktreePath)
	} else {
		log.Printf("✅ Terminated processes for worktree: %s", worktreePath)
	}

	// Also try to cleanup any session directories that might exist
	// Session IDs are typically derived from worktree names
	parts := strings.Split(strings.TrimPrefix(worktreePath, "/workspace/"), "/")
	if len(parts) >= 2 {
		sessionID := fmt.Sprintf("%s/%s", parts[0], parts[1])
		sessionWorkDir := filepath.Join("/workspace", sessionID)

		// If there's a session directory different from the worktree, clean it up too
		if sessionWorkDir != worktreePath {
			if _, err := os.Stat(sessionWorkDir); err == nil {
				if removeErr := os.RemoveAll(sessionWorkDir); removeErr != nil {
					log.Printf("⚠️ Failed to remove session directory %s: %v", sessionWorkDir, removeErr)
				} else {
					log.Printf("✅ Removed session directory: %s", sessionWorkDir)
				}
			}
		}
	}
}

// updateWorktreeStatusInternal updates commit count and commits behind for a worktree (internal, no mutex)
func (s *GitService) updateWorktreeStatusInternal(worktree *models.Worktree, shouldFetch bool) {
	if worktree.SourceBranch == "" || worktree.SourceBranch == worktree.Branch {
		return
	}

	// Fetch latest reference only if requested
	if shouldFetch {
		s.fetchLatestReference(worktree)
	}

	// Determine source reference based on repo type
	sourceRef := s.getSourceRef(worktree)

	// Count commits ahead (our commits)
	if count, err := s.getCommitCount(worktree.Path, sourceRef, "HEAD"); err == nil {
		worktree.CommitCount = count
	}

	// Count commits behind (missing commits)
	if count, err := s.getCommitCount(worktree.Path, "HEAD", sourceRef); err == nil {
		worktree.CommitsBehind = count
	}
}

// fetchLatestReference fetches the latest reference for a worktree (shallow fetch for status)
func (s *GitService) fetchLatestReference(worktree *models.Worktree) {
	s.fetchLatestReferenceWithDepth(worktree, true)
}

// fetchFullHistory fetches the full history for a worktree (needed for PR/push operations)
func (s *GitService) fetchFullHistory(worktree *models.Worktree) {
	s.fetchLatestReferenceWithDepth(worktree, false)
}

// fetchLatestReferenceWithDepth fetches the latest reference with optional shallow fetch
func (s *GitService) fetchLatestReferenceWithDepth(worktree *models.Worktree, shallow bool) {
	if s.isLocalRepo(worktree.RepoID) {
		// Get the local repo path
		repo, exists := s.repositories[worktree.RepoID]
		if exists {
			// Local repos: use shallow or full fetch based on need
			if shallow {
				_ = s.fetchLocalBranch(worktree.Path, repo.Path, worktree.SourceBranch)
			} else {
				_ = s.fetchLocalBranchFull(worktree.Path, repo.Path, worktree.SourceBranch)
			}
		}
	} else {
		// Remote repos: use shallow or full fetch based on need
		if shallow {
			_ = s.fetchBranchFast(worktree.Path, worktree.SourceBranch)
		} else {
			_ = s.fetchBranchFull(worktree.Path, worktree.SourceBranch)
		}
	}
}

// fetchBranchFast performs a highly optimized fetch for status updates
func (s *GitService) fetchBranchFast(repoPath, branch string) error {
	return s.helper.FetchBranchFast(repoPath, branch)
}

// fetchBranchFull performs a full fetch for operations that need complete history
func (s *GitService) fetchBranchFull(repoPath, branch string) error {
	return s.helper.FetchBranchFull(repoPath, branch)
}

// fetchLocalBranch performs a highly optimized fetch for local repos
func (s *GitService) fetchLocalBranch(worktreePath, mainRepoPath, branch string) error {
	// First, check if we even need to fetch by comparing commit hashes
	// Get the current commit hash of the remote branch in our worktree
	currentRemoteHash, err := s.runGitCommand(worktreePath, "rev-parse", fmt.Sprintf("live/%s", branch))
	if err != nil {
		// If we don't have the remote ref yet, we need to fetch
		return s.fetchLocalBranchInternal(worktreePath, mainRepoPath, branch)
	}

	// Get the latest commit hash from the main repo
	latestHash, err := s.runGitCommand(mainRepoPath, "rev-parse", branch)
	if err != nil {
		return fmt.Errorf("failed to get latest commit from main repo: %v", err)
	}

	// Compare hashes - if they're the same, no need to fetch
	if strings.TrimSpace(string(currentRemoteHash)) == strings.TrimSpace(string(latestHash)) {
		return nil // No changes, skip fetch
	}

	// Only fetch if there are actual changes
	return s.fetchLocalBranchInternal(worktreePath, mainRepoPath, branch)
}

// fetchLocalBranchInternal performs minimal fetch for local repos when needed
func (s *GitService) fetchLocalBranchInternal(worktreePath, mainRepoPath, branch string) error {
	// Highly optimized fetch for local repos - only fetch the specific branch tip
	args := []string{
		"fetch",
		mainRepoPath,
		fmt.Sprintf("%s:refs/remotes/live/%s", branch, branch),
		"--depth", "1", // Only fetch the latest commit
		"--quiet", // Reduce output noise
	}

	// Execute minimal fetch
	output, err := s.runGitCommand(worktreePath, args...)
	if err != nil {
		return fmt.Errorf("failed to fetch local branch minimal: %v\n%s", err, output)
	}

	return nil
}

// fetchLocalBranchFull performs a full fetch for local repos (needed for PR/push operations)
func (s *GitService) fetchLocalBranchFull(worktreePath, mainRepoPath, branch string) error {
	// First, check if we even need to fetch by comparing commit hashes
	// Get the current commit hash of the remote branch in our worktree
	currentRemoteHash, err := s.runGitCommand(worktreePath, "rev-parse", fmt.Sprintf("live/%s", branch))
	if err != nil {
		// If we don't have the remote ref yet, we need to fetch
		return s.fetchLocalBranchInternalFull(worktreePath, mainRepoPath, branch)
	}

	// Get the latest commit hash from the main repo
	latestHash, err := s.runGitCommand(mainRepoPath, "rev-parse", branch)
	if err != nil {
		return fmt.Errorf("failed to get latest commit from main repo: %v", err)
	}

	// Compare hashes - if they're the same, no need to fetch
	if strings.TrimSpace(string(currentRemoteHash)) == strings.TrimSpace(string(latestHash)) {
		return nil // No changes, skip fetch
	}

	// Only fetch if there are actual changes
	return s.fetchLocalBranchInternalFull(worktreePath, mainRepoPath, branch)
}

// fetchLocalBranchInternalFull performs full fetch for local repos when needed
func (s *GitService) fetchLocalBranchInternalFull(worktreePath, mainRepoPath, branch string) error {
	// Full fetch for local repos - fetch complete history
	args := []string{
		"fetch",
		mainRepoPath,
		fmt.Sprintf("%s:refs/remotes/live/%s", branch, branch),
		"--quiet", // Reduce output noise
		// Note: No --depth flag for full history
	}

	// Execute full fetch
	output, err := s.runGitCommand(worktreePath, args...)
	if err != nil {
		return fmt.Errorf("failed to fetch local branch full: %v\n%s", err, output)
	}

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

	return s.syncWorktreeInternal(worktree, strategy)
}

// syncWorktreeInternal consolidated sync logic for both local and regular repos
func (s *GitService) syncWorktreeInternal(worktree *models.Worktree, strategy string) error {
	// Ensure we have full history for sync operations
	s.fetchFullHistory(worktree)

	// Get the appropriate source reference (fetch already done by fetchFullHistory)
	sourceRef := s.getSourceRef(worktree)

	// Apply the sync strategy
	if err := s.applySyncStrategy(worktree, strategy, sourceRef); err != nil {
		return err
	}

	// Update worktree status (no need to fetch since we already did fetchFullHistory)
	s.updateWorktreeStatusInternal(worktree, false)

	log.Printf("✅ Synced worktree %s with %s strategy", worktree.Name, strategy)
	return nil
}

// applySyncStrategy applies merge or rebase strategy
func (s *GitService) applySyncStrategy(worktree *models.Worktree, strategy, sourceRef string) error {
	var cmd *exec.Cmd

	switch strategy {
	case "merge":
		cmd = s.execGitCommand(worktree.Path, "merge", sourceRef)
	case "rebase":
		cmd = s.execGitCommand(worktree.Path, "rebase", sourceRef)
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if this is a merge conflict
		if s.isMergeConflict(worktree.Path, string(output)) {
			return s.createMergeConflictError("sync", worktree, string(output))
		}
		return fmt.Errorf("failed to %s: %v\n%s", strategy, err, output)
	}

	return nil
}

// MergeWorktreeToMain merges a local repo worktree's changes back to the main repository
func (s *GitService) MergeWorktreeToMain(worktreeID string, squash bool) error {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Only works for local repos
	if !s.isLocalRepo(worktree.RepoID) {
		return fmt.Errorf("merge to main only supported for local repositories")
	}

	// Get the local repo
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return fmt.Errorf("local repository %s not found", worktree.RepoID)
	}

	log.Printf("🔄 Merging worktree %s back to main repository", worktree.Name)

	// Ensure we have full history for merge operations
	s.fetchFullHistory(worktree)

	// First, push the worktree branch to the main repo
	cmd := s.execGitCommand(worktree.Path, "push", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, worktree.Branch))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push worktree branch to main repo: %v\n%s", err, output)
	}

	// Switch to the source branch in main repo and merge
	cmd = s.execGitCommand(repo.Path, "checkout", worktree.SourceBranch)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to checkout source branch in main repo: %v\n%s", err, output)
	}

	// Merge the worktree branch
	var mergeArgs []string
	if squash {
		mergeArgs = []string{"merge", worktree.Branch, "--squash"}
	} else {
		mergeArgs = []string{"merge", worktree.Branch, "--no-ff", "-m", fmt.Sprintf("Merge branch '%s' from worktree", worktree.Branch)}
	}
	cmd = s.execGitCommand(repo.Path, mergeArgs...)
	output, err = cmd.CombinedOutput()
	if err != nil {
		// Check if this is a merge conflict
		if s.isMergeConflict(repo.Path, string(output)) {
			return s.createMergeConflictError("merge", worktree, string(output))
		}
		return fmt.Errorf("failed to merge worktree branch: %v\n%s", err, output)
	}

	// For squash merges, we need to commit the staged changes
	if squash {
		cmd = s.execGitCommand(repo.Path, "commit", "-m", fmt.Sprintf("Squash merge branch '%s' from worktree", worktree.Branch))
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to commit squash merge: %v\n%s", err, output)
		}
	}

	// Delete the feature branch from main repo (cleanup)
	cmd = s.execGitCommand(repo.Path, "branch", "-d", worktree.Branch)
	_ = cmd.Run() // Ignore errors - branch might be in use

	// Get the new commit hash from the main branch after merge
	cmd = s.execGitCommand(repo.Path, "rev-parse", "HEAD")
	output, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("⚠️  Failed to get new commit hash after merge: %v", err)
	} else {
		newCommitHash := strings.TrimSpace(string(output))
		// Update the worktree's commit hash to the new merge point
		s.mu.Lock()
		worktree.CommitHash = newCommitHash
		s.mu.Unlock()
		log.Printf("📝 Updated worktree %s CommitHash to %s", worktree.Name, newCommitHash)
	}

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
	if !s.isLocalRepo(worktree.RepoID) {
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
				resetCmd := s.execGitCommand(worktree.Path, "reset", "--mixed", "HEAD~1")
				_ = resetCmd.Run()
			}
		}()
	}

	// Check if preview branch already exists and handle accordingly
	shouldForceUpdate, err := s.shouldForceUpdatePreviewBranch(repo.Path, previewBranchName)
	if err != nil {
		return fmt.Errorf("failed to check preview branch status: %v", err)
	}

	// Push the worktree branch to a preview branch in main repo
	pushArgs := []string{"push"}
	if shouldForceUpdate {
		pushArgs = append(pushArgs, "--force")
		log.Printf("🔄 Updating existing preview branch %s", previewBranchName)
	}
	pushArgs = append(pushArgs, repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, previewBranchName))

	cmd := s.execGitCommand(worktree.Path, pushArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create preview branch: %v\n%s", err, output)
	}

	action := "created"
	if shouldForceUpdate {
		action = "updated"
	}

	if hasUncommittedChanges {
		log.Printf("✅ Preview branch %s %s with uncommitted changes - you can now checkout this branch outside the container", previewBranchName, action)
	} else {
		log.Printf("✅ Preview branch %s %s - you can now checkout this branch outside the container", previewBranchName, action)
	}
	return nil
}

// shouldForceUpdatePreviewBranch determines if we should force-update an existing preview branch
func (s *GitService) shouldForceUpdatePreviewBranch(repoPath, previewBranchName string) (bool, error) {
	// Check if the preview branch exists
	cmd := s.execGitCommand(repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", previewBranchName))
	if err := cmd.Run(); err != nil {
		// Branch doesn't exist, safe to create
		return false, nil
	}

	// Branch exists, check if the last commit was made by us (preview commit)
	cmd = s.execGitCommand(repoPath, "log", "-1", "--pretty=format:%s", previewBranchName)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to get last commit message: %v", err)
	}

	lastCommitMessage := strings.TrimSpace(string(output))

	// Check if this looks like our preview commit
	isOurPreviewCommit := strings.Contains(lastCommitMessage, "Preview:") ||
		strings.Contains(lastCommitMessage, "Include all uncommitted changes") ||
		strings.Contains(lastCommitMessage, "preview") // Case insensitive fallback

	if isOurPreviewCommit {
		log.Printf("🔍 Found existing preview branch %s with our commit: '%s'", previewBranchName, lastCommitMessage)
		return true, nil
	}

	// The preview branch exists but doesn't appear to be our commit
	// Let's still allow force update but warn about it
	log.Printf("⚠️  Preview branch %s exists with non-preview commit: '%s' - will force update anyway", previewBranchName, lastCommitMessage)
	return true, nil
}

// hasUncommittedChanges checks if the worktree has any uncommitted changes
func (s *GitService) hasUncommittedChanges(worktreePath string) (bool, error) {
	return s.helper.StatusChecker.HasUncommittedChanges(worktreePath)
}

// createTemporaryCommit creates a temporary commit with all uncommitted changes
func (s *GitService) createTemporaryCommit(worktreePath string) (string, error) {
	// Add all changes (staged, unstaged, and untracked)
	cmd := s.execGitCommand(worktreePath, "add", ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to stage changes: %v\n%s", err, output)
	}

	// Create the commit
	cmd = s.execGitCommand(worktreePath, "commit", "-m", "Preview: Include all uncommitted changes")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to create temporary commit: %v\n%s", err, output)
	}

	// Get the commit hash
	cmd = s.execGitCommand(worktreePath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %v", err)
	}

	commitHash := strings.TrimSpace(string(output))
	log.Printf("📝 Created temporary commit %s with uncommitted changes", commitHash[:8])
	return commitHash, nil
}

// isMergeConflict checks if the git command output indicates a merge conflict
func (s *GitService) isMergeConflict(repoPath, output string) bool {
	// Check for common merge conflict indicators in git output
	conflictIndicators := []string{
		"CONFLICT",
		"Automatic merge failed",
		"fix conflicts and then commit",
		"Merge conflict",
	}

	for _, indicator := range conflictIndicators {
		if strings.Contains(output, indicator) {
			return true
		}
	}

	// Also check git status for unmerged paths
	cmd := s.execGitCommand(repoPath, "status", "--porcelain")
	statusOutput, err := cmd.Output()
	if err != nil {
		return false
	}

	// Look for unmerged files (status codes AA, AU, DD, DU, UA, UD, UU)
	lines := strings.Split(string(statusOutput), "\n")
	for _, line := range lines {
		if len(line) >= 2 {
			status := line[:2]
			if strings.Contains("AA AU DD DU UA UD UU", status) {
				return true
			}
		}
	}

	return false
}

// createMergeConflictError creates a detailed merge conflict error
func (s *GitService) createMergeConflictError(operation string, worktree *models.Worktree, output string) *models.MergeConflictError {
	// Get list of conflicted files
	conflictFiles := s.getConflictedFiles(worktree.Path)

	message := fmt.Sprintf("Merge conflict occurred during %s operation in worktree '%s'. Please resolve conflicts in the terminal.", operation, worktree.Name)

	return &models.MergeConflictError{
		Operation:     operation,
		WorktreeName:  worktree.Name,
		WorktreePath:  worktree.Path,
		ConflictFiles: conflictFiles,
		Message:       message,
	}
}

// getConflictedFiles returns a list of files with merge conflicts
func (s *GitService) getConflictedFiles(repoPath string) []string {
	cmd := s.execGitCommand(repoPath, "diff", "--name-only", "--diff-filter=U")
	output, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var conflictFiles []string
	for _, file := range files {
		if file != "" {
			conflictFiles = append(conflictFiles, file)
		}
	}

	return conflictFiles
}

// CheckSyncConflicts checks if syncing a worktree would cause merge conflicts
func (s *GitService) CheckSyncConflicts(worktreeID string) (*models.MergeConflictError, error) {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	return s.checkConflictsInternal(worktree, "sync")
}

// checkConflictsInternal consolidated conflict checking logic
func (s *GitService) checkConflictsInternal(worktree *models.Worktree, operation string) (*models.MergeConflictError, error) {
	// Ensure we have full history for accurate conflict detection
	s.fetchFullHistory(worktree)

	// Get the appropriate source reference
	sourceRef := s.getSourceRef(worktree)

	// Try a dry-run merge to detect conflicts
	output, err := s.runGitCommand(worktree.Path, "merge-tree", "HEAD", sourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to check for conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	outputStr := string(output)
	if s.hasConflictMarkers(outputStr) {
		// Parse conflicted files from merge-tree output
		conflictFiles := s.parseConflictFiles(outputStr)

		return &models.MergeConflictError{
			Operation:     operation,
			WorktreeName:  worktree.Name,
			WorktreePath:  worktree.Path,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("%s would cause conflicts in worktree '%s'", operation, worktree.Name),
		}, nil
	}

	return nil, nil
}

// hasConflictMarkers checks if the output contains conflict markers
func (s *GitService) hasConflictMarkers(output string) bool {
	return strings.Contains(output, "<<<<<<< ") ||
		strings.Contains(output, "======= ") ||
		strings.Contains(output, ">>>>>>> ")
}

// CheckMergeConflicts checks if merging a worktree to main would cause conflicts
func (s *GitService) CheckMergeConflicts(worktreeID string) (*models.MergeConflictError, error) {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Only works for local repos
	if !s.isLocalRepo(worktree.RepoID) {
		return nil, fmt.Errorf("merge conflict check only supported for local repositories")
	}

	// Get the local repo
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return nil, fmt.Errorf("local repository %s not found", worktree.RepoID)
	}

	// Create a temporary branch in the main repo to test the merge
	tempBranch := fmt.Sprintf("temp-merge-check-%d", time.Now().Unix())

	// Push the worktree branch to temp branch in main repo
	cmd := s.execGitCommand(worktree.Path, "push", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, tempBranch))
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to push temp branch for conflict check: %v", err)
	}

	// Clean up temp branch when done
	defer func() {
		cmd := s.execGitCommand(repo.Path, "branch", "-D", tempBranch)
		_ = cmd.Run() // Ignore errors
	}()

	// Try a dry-run merge to detect conflicts
	cmd = s.execGitCommand(repo.Path, "merge-tree",
		worktree.SourceBranch,
		tempBranch)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("failed to check merge conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	outputStr := string(output)
	if strings.Contains(outputStr, "<<<<<<< ") || strings.Contains(outputStr, "======= ") || strings.Contains(outputStr, ">>>>>>> ") {
		// Parse conflicted files from merge-tree output
		conflictFiles := s.parseConflictFiles(outputStr)

		return &models.MergeConflictError{
			Operation:     "merge",
			WorktreeName:  worktree.Name,
			WorktreePath:  worktree.Path,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("Merge would cause conflicts in worktree '%s'", worktree.Name),
		}, nil
	}

	return nil, nil
}

// parseConflictFiles extracts file names from merge-tree conflict output
func (s *GitService) parseConflictFiles(output string) []string {
	var conflictFiles []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		// Look for conflict markers that indicate file paths
		if strings.HasPrefix(line, "<<<<<<< ") {
			// Extract file path from conflict marker context
			// This is a simplified approach - merge-tree output format can vary
			continue
		}
		// Look for "CONFLICT" lines that often contain file paths
		if strings.Contains(line, "CONFLICT") && strings.Contains(line, "in ") {
			parts := strings.Split(line, " in ")
			if len(parts) > 1 {
				file := strings.TrimSpace(parts[len(parts)-1])
				if file != "" && !contains(conflictFiles, file) {
					conflictFiles = append(conflictFiles, file)
				}
			}
		}
	}

	// Fallback: if we couldn't parse files, indicate conflicts exist
	if len(conflictFiles) == 0 && (strings.Contains(output, "<<<<<<< ") || strings.Contains(output, "CONFLICT")) {
		conflictFiles = []string{"(multiple files)"}
	}

	return conflictFiles
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Stop stops the Git service
func (s *GitService) Stop() {
	// No background services to stop
}

// GitAddCommitGetHash performs git add, commit, and returns the commit hash
// Returns empty string if not a git repository or no changes to commit
func (s *GitService) GitAddCommitGetHash(workspaceDir, message string) (string, error) {
	// Check if it's a git repository
	if err := s.execGitCommand(workspaceDir, "rev-parse", "--git-dir").Run(); err != nil {
		log.Printf("📂 Not a git repository, skipping git operations")
		return "", nil
	}

	// Stage all changes
	if output, err := s.runGitCommand(workspaceDir, "add", "."); err != nil {
		return "", fmt.Errorf("git add failed: %v, output: %s", err, string(output))
	}

	// Check if there are staged changes to commit
	if err := s.execGitCommand(workspaceDir, "diff", "--cached", "--quiet").Run(); err == nil {
		return "", nil
	}

	// Commit with the message
	if output, err := s.runGitCommand(workspaceDir, "commit", "-m", message, "-n"); err != nil {
		return "", fmt.Errorf("git commit failed: %v, output: %s", err, string(output))
	}

	// Get the commit hash
	output, err := s.runGitCommand(workspaceDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %v", err)
	}

	hash := strings.TrimSpace(string(output))
	return hash, nil
}

// createWorktreeForExistingRepo creates a worktree for an already loaded repository
func (s *GitService) createWorktreeForExistingRepo(repo *models.Repository, branch string) (*models.Repository, *models.Worktree, error) {
	// If no branch specified, use default
	if branch == "" {
		branch = repo.DefaultBranch
	}

	// Handle local repos specially (they don't have a bare repo)
	if s.isLocalRepo(repo.ID) {
		return s.handleLocalRepoWorktree(repo.ID, branch)
	}

	// Always fetch the latest state for checkout operations (full history)
	log.Printf("🔄 Fetching latest state for branch %s", branch)
	if err := s.fetchBranch(repo.Path, git.FetchStrategy{
		Branch:         branch,
		UpdateLocalRef: true,
	}); err != nil {
		// If fetch fails, check if branch exists locally and proceed if so
		if !s.branchExists(repo.Path, branch, true) {
			return nil, nil, fmt.Errorf("failed to fetch branch %s: %v", branch, err)
		}
		log.Printf("⚠️ Fetch failed but branch exists locally, proceeding with checkout")
	}

	// Create new worktree with fun name
	funName := s.generateUniqueSessionName(repo.Path)
	// Creating worktree
	worktree, err := s.createWorktreeInternalForRepo(repo, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree: %v", err)
	}

	// Save state
	_ = s.saveState()

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
	// Extract workspace name (remove catnip/ prefix for filesystem paths)
	workspaceName := git.ExtractWorkspaceName(name)
	worktreePath := filepath.Join(getWorkspaceDir(), repoName, workspaceName)

	// Create worktree with new branch using the fun name
	cmd := exec.Command("git", "-C", repo.Path, "worktree", "add", "-b", name, worktreePath, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is because branch already exists
		if strings.Contains(string(output), "already exists") {
			log.Printf("⚠️  Branch %s already exists, trying a new name...", name)
			// Generate a unique name that doesn't already exist
			newName := s.generateUniqueSessionName(repo.Path)
			return s.createWorktreeInternalForRepo(repo, source, newName, isInitial)
		}
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
		// Try to find which branch contains this commit, excluding preview branches
		cmd = exec.Command("git", "-C", repo.Path, "branch", "--contains", source)
		branchOutput, err := cmd.Output()
		if err == nil {
			branches := strings.Split(strings.TrimSpace(string(branchOutput)), "\n")
			// Filter out preview branches and find the best source branch
			for _, branch := range branches {
				// Clean up branch name - remove *, +, and leading/trailing spaces
				cleanBranch := strings.TrimSpace(branch)
				cleanBranch = strings.TrimPrefix(cleanBranch, "*")
				cleanBranch = strings.TrimPrefix(cleanBranch, "+")
				cleanBranch = strings.TrimSpace(cleanBranch)
				cleanBranch = strings.TrimPrefix(cleanBranch, "origin/")

				// Skip preview branches - they're not real source branches
				if strings.HasPrefix(cleanBranch, "preview/") {
					continue
				}

				// Skip the current branch itself (it can't be its own source)
				if cleanBranch == name {
					continue
				}

				// Prefer main/master branches over others
				if cleanBranch == "main" || cleanBranch == "master" {
					sourceBranch = cleanBranch
					break
				}

				// Use the first non-preview branch as fallback
				if sourceBranch == source { // Still the original source (commit hash)
					sourceBranch = cleanBranch
				}
			}
		}
	}

	// Calculate commit count ahead of source
	commitCount := 0
	if sourceBranch != name { // Only count if different from current branch
		cmd = s.execGitCommand(worktreePath, "rev-list", "--count", fmt.Sprintf("%s..HEAD", sourceBranch))
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

	// Create display name with repo name prefix (use already extracted workspaceName from line 2313)
	displayName := fmt.Sprintf("%s/%s", repoName, workspaceName)

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
		HasConflicts: false,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}

	s.worktrees[id] = worktree

	if isInitial || len(s.worktrees) == 1 {
		// Update current symlink to point to the first/initial worktree
		_ = s.updateCurrentSymlink(worktreePath)
	}

	// Git worktrees automatically sync to bare repository

	return worktree, nil
}

// unshallowRepository unshallows a specific branch in the background
func (s *GitService) unshallowRepository(barePath, branch string) {
	// Wait a bit before starting to avoid interfering with initial setup
	time.Sleep(5 * time.Second)

	// Only fetch the specific branch to be more efficient
	cmd := s.execGitCommand(barePath, "fetch", "origin", "--unshallow", branch)

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

// FileDiff represents a single file's diff
type FileDiff struct {
	FilePath   string `json:"file_path"`
	ChangeType string `json:"change_type"` // "added", "deleted", "modified"
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
	DiffText   string `json:"diff_text,omitempty"`
	IsExpanded bool   `json:"is_expanded"` // Default expansion state
}

// WorktreeDiffResponse represents the diff response
type WorktreeDiffResponse struct {
	WorktreeID   string     `json:"worktree_id"`
	WorktreeName string     `json:"worktree_name"`
	SourceBranch string     `json:"source_branch"`
	ForkCommit   string     `json:"fork_commit"` // The commit where this worktree was forked from
	FileDiffs    []FileDiff `json:"file_diffs"`
	TotalFiles   int        `json:"total_files"`
	Summary      string     `json:"summary"`
}

// GetWorktreeDiff returns the diff for a worktree against its source branch
func (s *GitService) GetWorktreeDiff(worktreeID string) (*WorktreeDiffResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	worktree, exists := s.worktrees[worktreeID]
	if !exists {
		return nil, fmt.Errorf("worktree not found: %s", worktreeID)
	}

	// Try to get diff without fetching first (much faster for local changes)
	sourceRef := s.getSourceRef(worktree)

	// Attempt to find merge base with existing references
	mergeBaseCmd := s.execGitCommand(worktree.Path, "merge-base", "HEAD", sourceRef)
	mergeBaseOutput, err := mergeBaseCmd.Output()

	// If merge base fails, try fetching the latest reference and retry
	if err != nil {
		log.Printf("🔄 Merge base not found with existing refs, fetching latest reference for diff")
		s.fetchLatestReference(worktree)
		sourceRef = s.getSourceRef(worktree)

		mergeBaseCmd = s.execGitCommand(worktree.Path, "merge-base", "HEAD", sourceRef)
		mergeBaseOutput, err = mergeBaseCmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to find merge base: %v", err)
		}
	}

	forkCommit := strings.TrimSpace(string(mergeBaseOutput))

	// Get the list of changed files from the fork point
	cmd := s.execGitCommand(worktree.Path, "diff", "--name-status", fmt.Sprintf("%s..HEAD", forkCommit))

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get diff list: %v", err)
	}

	var fileDiffs []FileDiff
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Process committed changes
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		changeType := parts[0]
		filePath := parts[1]

		fileDiff := FileDiff{
			FilePath:   filePath,
			IsExpanded: false, // Default to collapsed for added/deleted files
		}

		switch changeType {
		case "A":
			fileDiff.ChangeType = "added"
			fileDiff.IsExpanded = false // Collapse by default
		case "D":
			fileDiff.ChangeType = "deleted"
			fileDiff.IsExpanded = false // Collapse by default
		case "M":
			fileDiff.ChangeType = "modified"
			fileDiff.IsExpanded = true // Expand by default for modifications
		default:
			fileDiff.ChangeType = "modified"
			fileDiff.IsExpanded = true
		}

		// Get the old content (from fork commit)
		oldContentCmd := s.execGitCommand(worktree.Path, "show", fmt.Sprintf("%s:%s", forkCommit, filePath))

		if oldOutput, err := oldContentCmd.Output(); err == nil {
			fileDiff.OldContent = string(oldOutput)
		}

		// Get the new content (current HEAD)
		newContentCmd := s.execGitCommand(worktree.Path, "show", fmt.Sprintf("HEAD:%s", filePath))

		if newOutput, err := newContentCmd.Output(); err == nil {
			fileDiff.NewContent = string(newOutput)
		}

		// Also keep the unified diff for fallback
		diffCmd := s.execGitCommand(worktree.Path, "diff", fmt.Sprintf("%s..HEAD", forkCommit), "--", filePath)

		if diffOutput, err := diffCmd.Output(); err == nil {
			fileDiff.DiffText = string(diffOutput)
		}

		fileDiffs = append(fileDiffs, fileDiff)
	}

	// Also check for unstaged changes
	unstagedCmd := s.execGitCommand(worktree.Path, "diff", "--name-status")

	if unstagedOutput, err := unstagedCmd.Output(); err == nil {
		unstagedLines := strings.Split(strings.TrimSpace(string(unstagedOutput)), "\n")
		for _, line := range unstagedLines {
			if line == "" {
				continue
			}

			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}

			changeType := parts[0]
			filePath := parts[1]

			// Check if this file already exists in our diff list
			found := false
			for i := range fileDiffs {
				if fileDiffs[i].FilePath == filePath {
					// Update the existing entry to show it has unstaged changes
					if fileDiffs[i].ChangeType == "added" {
						fileDiffs[i].ChangeType = "added + modified (unstaged)"
					} else {
						fileDiffs[i].ChangeType = "modified (unstaged)"
					}

					// Update content to show working directory state
					if newContent, err := os.ReadFile(filepath.Join(worktree.Path, filePath)); err == nil {
						fileDiffs[i].NewContent = string(newContent)
					}

					// Update diff to show unstaged changes
					diffCmd := s.execGitCommand(worktree.Path, "diff", "--", filePath)
					if diffOutput, err := diffCmd.Output(); err == nil {
						fileDiffs[i].DiffText = string(diffOutput)
					}

					fileDiffs[i].IsExpanded = true
					found = true
					break
				}
			}

			if !found {
				fileDiff := FileDiff{
					FilePath:   filePath,
					IsExpanded: true, // Unstaged changes should be visible
				}

				switch changeType {
				case "A":
					fileDiff.ChangeType = "added (unstaged)"
				case "D":
					fileDiff.ChangeType = "deleted (unstaged)"
				case "M":
					fileDiff.ChangeType = "modified (unstaged)"
				default:
					fileDiff.ChangeType = "modified (unstaged)"
				}

				// Get old content (HEAD version)
				oldContentCmd := s.execGitCommand(worktree.Path, "show", fmt.Sprintf("HEAD:%s", filePath))

				if oldOutput, err := oldContentCmd.Output(); err == nil {
					fileDiff.OldContent = string(oldOutput)
				}

				// Get new content (working directory)
				if newContent, err := os.ReadFile(filepath.Join(worktree.Path, filePath)); err == nil {
					fileDiff.NewContent = string(newContent)
				}

				// Get unstaged diff content as fallback
				diffCmd := s.execGitCommand(worktree.Path, "diff", "--", filePath)

				if diffOutput, err := diffCmd.Output(); err == nil {
					fileDiff.DiffText = string(diffOutput)
				}

				fileDiffs = append(fileDiffs, fileDiff)
			}
		}
	}

	// Check for untracked files
	untrackedCmd := s.execGitCommand(worktree.Path, "ls-files", "--others", "--exclude-standard")

	if untrackedOutput, err := untrackedCmd.Output(); err == nil {
		untrackedFiles := strings.Split(strings.TrimSpace(string(untrackedOutput)), "\n")
		for _, filePath := range untrackedFiles {
			if filePath == "" {
				continue
			}

			fileDiff := FileDiff{
				FilePath:   filePath,
				ChangeType: "added (untracked)",
				IsExpanded: false, // Collapse by default
			}

			// Read file content for untracked files
			if content, err := os.ReadFile(filepath.Join(worktree.Path, filePath)); err == nil {
				fileDiff.NewContent = string(content)
			}

			fileDiffs = append(fileDiffs, fileDiff)
		}
	}

	// Generate summary
	var summary string
	totalFiles := len(fileDiffs)
	switch totalFiles {
	case 0:
		summary = "No changes"
	case 1:
		summary = "1 file changed"
	default:
		summary = fmt.Sprintf("%d files changed", totalFiles)
	}

	return &WorktreeDiffResponse{
		WorktreeID:   worktreeID,
		WorktreeName: worktree.Name,
		SourceBranch: worktree.SourceBranch,
		ForkCommit:   forkCommit,
		FileDiffs:    fileDiffs,
		TotalFiles:   totalFiles,
		Summary:      summary,
	}, nil
}

// CreatePullRequest creates a pull request for a worktree branch
func (s *GitService) CreatePullRequest(worktreeID, title, body string) (*models.PullRequestResponse, error) {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Get the repository
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return nil, fmt.Errorf("repository %s not found", worktree.RepoID)
	}

	log.Printf("🔄 Creating pull request for worktree %s", worktree.Name)

	// Check if base branch exists on remote and push if needed
	if err := s.ensureBaseBranchOnRemote(worktree, repo); err != nil {
		return nil, fmt.Errorf("failed to ensure base branch exists on remote: %v", err)
	}

	return s.createPullRequestInternal(worktree, repo, title, body, false)
}

// UpdatePullRequest updates an existing pull request for a worktree branch
func (s *GitService) UpdatePullRequest(worktreeID, title, body string) (*models.PullRequestResponse, error) {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Get the repository
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return nil, fmt.Errorf("repository %s not found", worktree.RepoID)
	}

	log.Printf("🔄 Updating pull request for worktree %s", worktree.Name)

	// Check if base branch exists on remote and push if needed
	if err := s.ensureBaseBranchOnRemote(worktree, repo); err != nil {
		return nil, fmt.Errorf("failed to ensure base branch exists on remote: %v", err)
	}

	return s.createPullRequestInternal(worktree, repo, title, body, true)
}

// createPullRequestInternal consolidated PR creation/update logic
func (s *GitService) createPullRequestInternal(worktree *models.Worktree, repo *models.Repository, title, body string, isUpdate bool) (*models.PullRequestResponse, error) {
	// Ensure we have full history for PR operations
	s.fetchFullHistory(worktree)

	// Get remote URL and owner/repo
	ownerRepo, pushTarget, err := s.getRepoInfo(worktree, repo)
	if err != nil {
		return nil, err
	}

	// Push the worktree branch with sync handling
	if err := s.pushBranchWithSync(worktree, repo, pushTarget); err != nil {
		return nil, fmt.Errorf("failed to push branch: %v", err)
	}

	// Create or update the pull request using GitHub CLI
	if isUpdate {
		return s.updatePullRequestWithGH(worktree, ownerRepo, title, body)
	}
	return s.createPullRequestWithGH(worktree, ownerRepo, title, body)
}

// getRepoInfo gets the owner/repo and push target for a repository
func (s *GitService) getRepoInfo(worktree *models.Worktree, repo *models.Repository) (string, string, error) {
	if s.isLocalRepo(worktree.RepoID) {
		// Get the remote URL
		remoteURL, err := s.getRemoteURL(worktree.Path)
		if err != nil {
			// Try the main repo path as fallback
			remoteURL, err = s.getRemoteURL(repo.Path)
			if err != nil {
				// Try to infer from git config or suggest adding remote
				inferredURL, inferErr := s.inferRemoteURL(repo.Path)
				if inferErr == nil && inferredURL != "" {
					remoteURL = inferredURL
				} else {
					return "", "", fmt.Errorf("local repository does not have a remote 'origin' configured and could not infer GitHub repository URL. Please add a remote first with: git remote add origin <github-repo-url>")
				}
			}
		}

		// Parse the remote URL to get owner/repo
		ownerRepo, err := s.parseGitHubURL(remoteURL)
		if err != nil {
			return "", "", fmt.Errorf("failed to parse remote URL %s: %v", remoteURL, err)
		}

		return ownerRepo, remoteURL, nil
	}

	// Parse the repository URL to get owner/repo
	ownerRepo, err := s.parseGitHubURL(repo.URL)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse repository URL %s: %v", repo.URL, err)
	}

	return ownerRepo, "origin", nil
}

// ensureBaseBranchOnRemote checks if the base branch exists on remote and pushes it if needed
func (s *GitService) ensureBaseBranchOnRemote(worktree *models.Worktree, repo *models.Repository) error {
	// For local repositories, check if base branch exists on remote
	if s.isLocalRepo(worktree.RepoID) {
		// Get the remote URL
		remoteURL, err := s.getRemoteURL(worktree.Path)
		if err != nil {
			// Try the main repo path as fallback
			remoteURL, err = s.getRemoteURL(repo.Path)
			if err != nil {
				// If no remote is configured, we can't check - assume it's handled locally
				log.Printf("⚠️ No remote configured for local repo %s, skipping base branch check", worktree.RepoID)
				return nil
			}
		}

		// Check if base branch exists on remote
		if err := s.checkBaseBranchOnRemote(worktree, remoteURL); err != nil {
			log.Printf("🔄 Base branch %s not found on remote, pushing it", worktree.SourceBranch)
			if err := s.pushBaseBranchToRemote(worktree, repo, remoteURL); err != nil {
				return fmt.Errorf("failed to push base branch to remote: %v", err)
			}
		}
	} else {
		// For remote repositories, ensure we have the latest base branch
		if err := s.fetchBaseBranchFromOrigin(worktree); err != nil {
			log.Printf("⚠️ Could not fetch base branch from origin: %v", err)
			// This is not a fatal error, continue with PR creation
		}
	}

	return nil
}

// checkBaseBranchOnRemote checks if the base branch exists on the remote repository
func (s *GitService) checkBaseBranchOnRemote(worktree *models.Worktree, remoteURL string) error {
	// Use git ls-remote to check if the base branch exists on remote
	cmd := s.execCommand("git", "ls-remote", "--heads", remoteURL, worktree.SourceBranch)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check remote branches: %v", err)
	}

	// If output is empty, the branch doesn't exist on remote
	if len(strings.TrimSpace(string(output))) == 0 {
		return fmt.Errorf("base branch %s does not exist on remote", worktree.SourceBranch)
	}

	return nil
}

// pushBaseBranchToRemote pushes the base branch to the remote repository
func (s *GitService) pushBaseBranchToRemote(worktree *models.Worktree, repo *models.Repository, remoteURL string) error {
	strategy := PushStrategy{
		Branch:       worktree.SourceBranch,
		RemoteURL:    remoteURL,
		ConvertHTTPS: true,
	}

	return s.pushBranch(worktree, repo, strategy)
}

// fetchBaseBranchFromOrigin fetches the latest base branch from origin
func (s *GitService) fetchBaseBranchFromOrigin(worktree *models.Worktree) error {
	return s.fetchBranch(worktree.Path, git.FetchStrategy{
		Branch: worktree.SourceBranch,
	})
}

// syncBranchWithUpstream syncs the current branch with upstream when push fails due to being behind
func (s *GitService) syncBranchWithUpstream(worktree *models.Worktree) error {
	log.Printf("🔄 Syncing branch %s with upstream due to push failure", worktree.Branch)

	// First, fetch the latest changes from remote
	if err := s.fetchBranch(worktree.Path, git.FetchStrategy{
		Branch: worktree.Branch,
	}); err != nil {
		// If fetch fails, the branch might not exist on remote yet - that's OK
		log.Printf("⚠️ Could not fetch remote branch %s (might not exist yet): %v", worktree.Branch, err)
		return nil
	}

	// Check if we're behind the remote branch
	cmd := s.execGitCommand(worktree.Path, "rev-list", "--count", fmt.Sprintf("HEAD..origin/%s", worktree.Branch))

	output, err := cmd.Output()
	if err != nil {
		// If this fails, assume we're not behind
		return nil
	}

	behindCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || behindCount == 0 {
		// We're not behind, no need to sync
		return nil
	}

	log.Printf("🔄 Branch %s is %d commits behind remote, syncing", worktree.Branch, behindCount)

	// Rebase our changes on top of the remote branch
	cmd = s.execGitCommand(worktree.Path, "rebase", fmt.Sprintf("origin/%s", worktree.Branch))

	output, err = cmd.CombinedOutput()
	if err != nil {
		// Check if this is a rebase conflict
		if strings.Contains(string(output), "CONFLICT") {
			return fmt.Errorf("rebase conflict occurred while syncing with upstream. Please resolve conflicts manually in the terminal")
		}
		return fmt.Errorf("failed to rebase on upstream: %v\n%s", err, output)
	}

	log.Printf("✅ Successfully synced branch %s with upstream", worktree.Branch)
	return nil
}

// pushBranchWithSync pushes a branch to remote, syncing with upstream if needed
func (s *GitService) pushBranchWithSync(worktree *models.Worktree, repo *models.Repository, remote string) error {
	strategy := PushStrategy{
		SetUpstream:  true,
		SyncOnFail:   true,
		ConvertHTTPS: true, // This now includes automatic workflow detection
	}

	if s.isLocalRepo(worktree.RepoID) {
		// For local repos, we need to handle the remote URL
		remoteURL, err := s.getRemoteURL(worktree.Path)
		if err != nil {
			return fmt.Errorf("failed to get remote URL: %v", err)
		}
		strategy.RemoteURL = remoteURL
	}

	return s.pushBranch(worktree, repo, strategy)
}

// Removed isPushRejectedDueToUpstream - use git.IsPushRejected directly

// updatePullRequestWithGH updates a pull request using GitHub CLI
func (s *GitService) updatePullRequestWithGH(worktree *models.Worktree, ownerRepo, title, body string) (*models.PullRequestResponse, error) {
	// First, check if a PR exists for this branch
	cmd := s.execCommand("gh", "pr", "view", worktree.Branch, "--repo", ownerRepo, "--json", "number,url,title,body")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("no existing pull request found for branch %s", worktree.Branch)
	}

	// Parse the existing PR info
	var existingPR struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}

	if err := json.Unmarshal(output, &existingPR); err != nil {
		return nil, fmt.Errorf("failed to parse existing PR info: %v", err)
	}

	// Use existing values if not provided
	if title == "" {
		title = existingPR.Title
	}
	if body == "" {
		body = existingPR.Body
	}

	// Update the PR
	cmd = s.execCommand("gh", "pr", "edit", fmt.Sprintf("%d", existingPR.Number),
		"--repo", ownerRepo,
		"--title", title,
		"--body", body)

	output, err = cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to update pull request: %v\n%s", err, output)
	}

	log.Printf("✅ Updated pull request #%d: %s", existingPR.Number, existingPR.URL)

	return &models.PullRequestResponse{
		Number:     existingPR.Number,
		URL:        existingPR.URL,
		Title:      title,
		Body:       body,
		HeadBranch: worktree.Branch,
		BaseBranch: worktree.SourceBranch,
		Repository: ownerRepo,
	}, nil
}

// createPullRequestWithGH creates a pull request using GitHub CLI
func (s *GitService) createPullRequestWithGH(worktree *models.Worktree, ownerRepo, title, body string) (*models.PullRequestResponse, error) {
	// If title is empty, generate one from the worktree name
	if title == "" {
		title = fmt.Sprintf("Pull request from %s", worktree.Branch)
	}

	// If body is empty, provide a default
	if body == "" {
		body = fmt.Sprintf("Automated pull request created from worktree %s", worktree.Name)
	}

	// Check if there are commits between the remote base and local head
	commitCheckCmd := s.execGitCommand(worktree.Path, "rev-list", "--count", fmt.Sprintf("origin/%s..HEAD", worktree.SourceBranch))

	if commitOutput, err := commitCheckCmd.Output(); err == nil {
		commitCount := strings.TrimSpace(string(commitOutput))
		if commitCount == "0" {
			return nil, fmt.Errorf("no commits found between origin/%s and HEAD - cannot create pull request", worktree.SourceBranch)
		}
	}

	// Create the PR using GitHub CLI
	cmd := s.execCommand("gh", "pr", "create",
		"--repo", ownerRepo,
		"--title", title,
		"--body", body,
		"--head", worktree.Branch,
		"--base", worktree.SourceBranch)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %v\n%s", err, output)
	}

	// Parse the PR URL from the output
	prURL := strings.TrimSpace(string(output))

	// Extract PR number from URL (e.g., https://github.com/owner/repo/pull/123)
	var prNumber int
	if strings.Contains(prURL, "/pull/") {
		parts := strings.Split(prURL, "/pull/")
		if len(parts) == 2 {
			if num, err := strconv.Atoi(parts[1]); err == nil {
				prNumber = num
			}
		}
	}

	log.Printf("✅ Created pull request #%d: %s", prNumber, prURL)

	return &models.PullRequestResponse{
		Number:     prNumber,
		URL:        prURL,
		Title:      title,
		Body:       body,
		HeadBranch: worktree.Branch,
		BaseBranch: worktree.SourceBranch,
		Repository: ownerRepo,
	}, nil
}

// inferRemoteURL attempts to infer the remote URL from git config or other sources
func (s *GitService) inferRemoteURL(repoPath string) (string, error) {
	// Check git config for remote.origin.url
	if output, err := s.runGitCommand(repoPath, "config", "--get", "remote.origin.url"); err == nil {
		url := strings.TrimSpace(string(output))
		if url != "" {
			log.Printf("🔍 [DEBUG] Found remote.origin.url in config: %s", url)
			return url, nil
		}
	}

	// Check if we can find any GitHub-related URLs in git config
	if output, err := s.runGitCommand(repoPath, "config", "--get-regexp", "remote\\..*\\.url"); err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "github.com") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					url := parts[1]
					log.Printf("🔍 [DEBUG] Found GitHub URL in config: %s", url)
					return url, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not infer remote URL from repository")
}

// Removed setupRemoteOrigin - use s.helper.SetupRemoteURL directly

// GetPullRequestInfo gets information about an existing pull request for a worktree
func (s *GitService) GetPullRequestInfo(worktreeID string) (*models.PullRequestInfo, error) {
	s.mu.RLock()
	worktree, exists := s.worktrees[worktreeID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Get the repository
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return nil, fmt.Errorf("repository %s not found", worktree.RepoID)
	}

	// Check if branch has commits ahead of the base branch
	hasCommitsAhead, err := s.checkHasCommitsAhead(worktree)
	if err != nil {
		log.Printf("⚠️ Could not check commits ahead: %v", err)
		hasCommitsAhead = false // Default to false if we can't determine
	}

	// Initialize PR info with commits ahead status
	prInfo := &models.PullRequestInfo{
		HasCommitsAhead: hasCommitsAhead,
		Exists:          false,
	}

	// Check if a PR exists for this branch
	var ownerRepo string
	if s.isLocalRepo(worktree.RepoID) {
		// For local repos, get the remote URL
		remoteURL, err := s.getRemoteURL(worktree.Path)
		if err != nil {
			// Try the main repo path as fallback
			remoteURL, err = s.getRemoteURL(repo.Path)
			if err != nil {
				// If no remote is configured, we can't check for PRs
				log.Printf("⚠️ No remote configured for local repo %s, cannot check for existing PR", worktree.RepoID)
				return prInfo, nil
			}
		}

		// Parse the remote URL to get owner/repo
		ownerRepo, err = s.parseGitHubURL(remoteURL)
		if err != nil {
			log.Printf("⚠️ Could not parse remote URL %s: %v", remoteURL, err)
			return prInfo, nil
		}
	} else {
		// For remote repos, parse the repository URL
		var err error
		ownerRepo, err = s.parseGitHubURL(repo.URL)
		if err != nil {
			log.Printf("⚠️ Could not parse repository URL %s: %v", repo.URL, err)
			return prInfo, nil
		}
	}

	// Check if PR exists using GitHub CLI
	if err := s.checkExistingPR(worktree, ownerRepo, prInfo); err != nil {
		log.Printf("⚠️ Could not check for existing PR: %v", err)
		// Not a fatal error, just means we couldn't determine PR status
	}

	return prInfo, nil
}

// checkHasCommitsAhead checks if the worktree branch has commits ahead of the base branch
func (s *GitService) checkHasCommitsAhead(worktree *models.Worktree) (bool, error) {
	// Ensure we have the latest base branch reference
	var baseRef string
	if s.isLocalRepo(worktree.RepoID) {
		// For local repos, use the local base branch reference
		baseRef = worktree.SourceBranch
	} else {
		// For remote repos, fetch the latest base branch and use origin reference
		cmd := s.execGitCommand(worktree.Path, "fetch", "origin", worktree.SourceBranch)
		if err := cmd.Run(); err != nil {
			log.Printf("⚠️ Could not fetch base branch %s: %v", worktree.SourceBranch, err)
		}
		baseRef = fmt.Sprintf("origin/%s", worktree.SourceBranch)
	}

	// Count commits ahead of base branch
	cmd := s.execGitCommand(worktree.Path, "rev-list", "--count", fmt.Sprintf("%s..HEAD", baseRef))

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to count commits ahead: %v", err)
	}

	commitCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return false, fmt.Errorf("failed to parse commit count: %v", err)
	}

	return commitCount > 0, nil
}

// checkExistingPR checks if a PR exists for the worktree branch and populates PR info
func (s *GitService) checkExistingPR(worktree *models.Worktree, ownerRepo string, prInfo *models.PullRequestInfo) error {
	// Use GitHub CLI to check for existing PR
	cmd := s.execCommand("gh", "pr", "view", worktree.Branch, "--repo", ownerRepo, "--json", "number,url,title,body")

	output, err := cmd.Output()
	if err != nil {
		// PR doesn't exist or we can't access it
		return nil
	}

	// Parse the PR information
	var existingPR struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}

	if err := json.Unmarshal(output, &existingPR); err != nil {
		return fmt.Errorf("failed to parse PR info: %v", err)
	}

	// Update PR info with existing PR details
	prInfo.Exists = true
	prInfo.Number = existingPR.Number
	prInfo.URL = existingPR.URL
	prInfo.Title = existingPR.Title
	prInfo.Body = existingPR.Body

	log.Printf("✅ Found existing PR #%d for branch %s", existingPR.Number, worktree.Branch)
	return nil
}
