package services

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
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

// Helper functions for common patterns

// setupGitEnv returns the standard environment for git commands
func setupGitEnv() []string {
	return append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
}

// execGitCommand runs a git command with standard environment setup
func execGitCommand(workingDir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if workingDir != "" {
		cmd.Args = append([]string{"git", "-C", workingDir}, args...)
	}
	cmd.Env = setupGitEnv()
	return cmd.Run()
}

// execGitCommandWithOutput runs a git command and returns its output
func execGitCommandWithOutput(workingDir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if workingDir != "" {
		cmd.Args = append([]string{"git", "-C", workingDir}, args...)
	}
	cmd.Env = setupGitEnv()
	return cmd.Output()
}

// execGitCommandWithCombinedOutput runs a git command and returns combined output
func execGitCommandWithCombinedOutput(workingDir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if workingDir != "" {
		cmd.Args = append([]string{"git", "-C", workingDir}, args...)
	}
	cmd.Env = setupGitEnv()
	return cmd.CombinedOutput()
}

// execCommand runs a non-git command with standard environment setup
func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = setupGitEnv()
	return cmd.Run()
}

// execCommandWithOutput runs a non-git command and returns its output
func execCommandWithOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = setupGitEnv()
	return cmd.Output()
}

// execCommandWithCombinedOutput runs a non-git command and returns combined output
func execCommandWithCombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = setupGitEnv()
	return cmd.CombinedOutput()
}

// getCommitHash gets the commit hash from a repository path
func getCommitHash(repoPath string) (string, error) {
	output, err := execGitCommandWithOutput(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %v", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// getCurrentBranchFromRepo gets the current branch name from a repository path
func getCurrentBranchFromRepo(repoPath string) (string, error) {
	output, err := execGitCommandWithOutput(repoPath, "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %v", err)
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "main", nil // fallback
	}
	return branch, nil
}

// branchExists checks if a branch exists in a repository
func branchExists(repoPath, branch string) bool {
	err := execGitCommand(repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
	return err == nil
}

// remoteBranchExists checks if a remote branch exists
func remoteBranchExists(repoPath, remote, branch string) bool {
	err := execGitCommand(repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/remotes/%s/%s", remote, branch))
	return err == nil
}

// getRemoteURL gets the remote URL for a repository
func getRemoteURL(repoPath, remote string) (string, error) {
	output, err := execGitCommandWithOutput(repoPath, "remote", "get-url", remote)
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %v", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// convertSSHToHTTPS converts SSH GitHub URLs to HTTPS
func convertSSHToHTTPS(url string) string {
	if strings.HasPrefix(url, "git@github.com:") {
		path := strings.TrimPrefix(url, "git@github.com:")
		return "https://github.com/" + path
	}
	return url
}

// parseGitHubURL parses a GitHub URL and returns owner/repo
func parseGitHubURL(url string) (string, error) {
	if strings.HasPrefix(url, "git@github.com:") {
		parts := strings.TrimPrefix(url, "git@github.com:")
		parts = strings.TrimSuffix(parts, ".git")
		return parts, nil
	} else if strings.Contains(url, "github.com/") {
		parts := strings.Split(url, "github.com/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid GitHub URL format")
		}
		ownerRepo := strings.TrimSuffix(parts[1], ".git")
		return ownerRepo, nil
	}
	return "", fmt.Errorf("URL does not appear to be a GitHub repository")
}

// parseRepoName extracts the repo name from a repo ID
func parseRepoName(repoID string) string {
	parts := strings.Split(repoID, "/")
	return parts[len(parts)-1]
}

// buildWorktreePath constructs a worktree path
func buildWorktreePath(repoID, name string) string {
	repoName := parseRepoName(repoID)
	return filepath.Join(workspaceDir, repoName, name)
}

// buildDisplayName constructs a display name for a worktree
func buildDisplayName(repoID, name string) string {
	repoName := parseRepoName(repoID)
	return fmt.Sprintf("%s/%s", repoName, name)
}

// isGitRepository checks if a path is a git repository
func isGitRepository(path string) bool {
	gitPath := filepath.Join(path, ".git")
	_, err := os.Stat(gitPath)
	return err == nil
}

// isDirty checks if a worktree has uncommitted changes
func isDirty(worktreePath string) bool {
	output, err := execGitCommandWithOutput(worktreePath, "status", "--porcelain")
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// getDefaultBranch gets the default branch from a bare repository
func getDefaultBranch(barePath string) (string, error) {
	// First try to get the symbolic ref
	output, err := execGitCommandWithOutput(barePath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		return strings.TrimSpace(strings.TrimPrefix(string(output), "refs/remotes/origin/")), nil
	}

	// If symbolic ref doesn't work, try to find the default branch from remote refs
	output, err = execGitCommandWithOutput(barePath, "branch", "-r")
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

	log.Printf("⚠️ Could not detect default branch, using fallback: main")
	return "main", nil
}

// countCommitsAhead counts commits ahead of a reference
func countCommitsAhead(repoPath, fromRef, toRef string) (int, error) {
	output, err := execGitCommandWithOutput(repoPath, "rev-list", "--count", fmt.Sprintf("%s..%s", fromRef, toRef))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(output)))
}

// tempRemoteURLManager manages temporary remote URL changes
type tempRemoteURLManager struct {
	repoPath    string
	remote      string
	originalURL string
	changed     bool
}

// newTempRemoteURLManager creates a new temporary remote URL manager
func newTempRemoteURLManager(repoPath, remote string) (*tempRemoteURLManager, error) {
	originalURL, err := getRemoteURL(repoPath, remote)
	if err != nil {
		return &tempRemoteURLManager{repoPath: repoPath, remote: remote}, nil // Continue without URL conversion
	}

	return &tempRemoteURLManager{
		repoPath:    repoPath,
		remote:      remote,
		originalURL: originalURL,
	}, nil
}

// setHTTPS temporarily sets the remote URL to HTTPS if it's SSH
func (m *tempRemoteURLManager) setHTTPS() error {
	if m.originalURL == "" {
		return nil
	}

	httpsURL := convertSSHToHTTPS(m.originalURL)
	if httpsURL != m.originalURL {
		err := execGitCommand(m.repoPath, "remote", "set-url", m.remote, httpsURL)
		if err != nil {
			return fmt.Errorf("failed to set HTTPS remote URL: %v", err)
		}
		m.changed = true
	}
	return nil
}

// restore restores the original remote URL if it was changed
func (m *tempRemoteURLManager) restore() error {
	if m.changed && m.originalURL != "" {
		err := execGitCommand(m.repoPath, "remote", "set-url", m.remote, m.originalURL)
		if err != nil {
			return fmt.Errorf("failed to restore original remote URL %s: %v", m.originalURL, err)
		}
		m.changed = false
	}
	return nil
}

// generateSessionName generates a fun session name
func generateSessionName() string {
	verbIndex, _ := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(verbs))))
	nounIndex, _ := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(nouns))))
	verb := verbs[verbIndex.Int64()]
	noun := nouns[nounIndex.Int64()]
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
	_ = os.MkdirAll(workspaceDir, 0755)
	_ = os.MkdirAll(gitStateDir, 0755)

	// Configure Git to use gh as credential helper if available
	s.configureGitCredentials()

	// Load existing state if available
	_ = s.loadState()

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
		defaultBranch, err := getDefaultBranch(barePath)
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
	_ = s.saveState()

	log.Printf("✅ Worktree created from existing repository: %s", repoID)
	return repo, worktree, nil
}

// cloneNewRepository clones a new bare repository
func (s *GitService) cloneNewRepository(repoID, repoURL, barePath, branch string) (*models.Repository, *models.Worktree, error) {
	// Clone as bare repository with shallow depth
	cloneArgs := []string{"clone", "--bare", "--depth", "1", "--single-branch"}
	if branch != "" {
		cloneArgs = append(cloneArgs, "--branch", branch)
	}
	cloneArgs = append(cloneArgs, repoURL, barePath)

	output, err := execGitCommandWithCombinedOutput("", cloneArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to clone repository: %v\n%s", err, output)
	}

	// Get default branch if not specified
	if branch == "" {
		branch, err = getDefaultBranch(barePath)
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

	return &models.GitStatus{
		Repositories:  s.repositories, // All repositories
		WorktreeCount: len(s.worktrees),
	}
}

// checkIfDirty checks if a worktree has uncommitted changes
func (s *GitService) checkIfDirty(worktreePath string) bool {
	return isDirty(worktreePath)
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
	if err := execCommand("gh", "auth", "status"); err != nil {
		log.Printf("ℹ️ GitHub CLI not authenticated, Git operations will only work with public repositories")
		return
	}

	log.Printf("🔐 Configuring Git to use GitHub CLI for authentication")

	// Configure Git to use gh as credential helper for GitHub
	if err := execCommand("git", "config", "--global", "credential.https://github.com.helper", "!gh auth git-credential"); err != nil {
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
	branch, err := getCurrentBranchFromRepo(repoPath)
	if err != nil {
		log.Printf("⚠️ Could not get current branch for repo at %s, using fallback: main", repoPath)
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
	_ = s.saveState()

	log.Printf("✅ Local repo worktree created: %s from branch %s", worktree.Name, worktree.SourceBranch)
	return localRepo, worktree, nil
}

// localRepoBranchExists checks if a branch exists in a local repo
func (s *GitService) localRepoBranchExists(repoPath, branch string) bool {
	return branchExists(repoPath, branch)
}

// createLocalRepoWorktree creates a worktree for any local repo
func (s *GitService) createLocalRepoWorktree(repo *models.Repository, branch, name string) (*models.Worktree, error) {
	id := uuid.New().String()
	worktreePath := buildWorktreePath(repo.ID, name)

	// Create worktree directory first
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %v", err)
	}

	// Create worktree with new branch using the fun name
	output, err := execGitCommandWithCombinedOutput(repo.Path, "worktree", "add", "-b", name, worktreePath, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v\n%s", err, output)
	}

	// Get current commit hash
	commitHash, err := getCommitHash(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}

	// Clean up branch name to ensure it's a proper source branch
	sourceBranch := strings.TrimSpace(branch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "*")
	sourceBranch = strings.TrimPrefix(sourceBranch, "+")
	sourceBranch = strings.TrimSpace(sourceBranch)
	sourceBranch = strings.TrimPrefix(sourceBranch, "origin/")

	// Calculate commit count ahead of source
	commitCount := 0
	if sourceBranch != name { // Only count if different from current branch
		if count, err := countCommitsAhead(worktreePath, sourceBranch, "HEAD"); err == nil {
			commitCount = count
		}
	}

	worktree := &models.Worktree{
		ID:            id,
		RepoID:        repo.ID,
		Name:          buildDisplayName(repo.ID, name),
		Path:          worktreePath,
		Branch:        name,
		SourceBranch:  sourceBranch,
		CommitHash:    commitHash,
		CommitCount:   commitCount,
		CommitsBehind: 0, // Will be calculated later
		IsDirty:       false,
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
	output, err := execGitCommandWithOutput(repoPath, "branch", "--format=%(refname:short)")
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
	if strings.HasPrefix(repoID, "local/") {
		return s.getLocalRepoBranches(repo.Path)
	}

	// Start with the default branch
	branches := []string{repo.DefaultBranch}
	branchSet := map[string]bool{repo.DefaultBranch: true}

	output, err := execGitCommandWithOutput(repo.Path, "branch", "-r")
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
	if err := execGitCommand(repo.Path, "worktree", "remove", "--force", worktree.Path); err != nil {
		log.Printf("⚠️ Failed to remove worktree directory (continuing with cleanup): %v", err)
		// Continue with cleanup even if worktree removal fails
	} else {
		log.Printf("✅ Removed worktree directory: %s", worktree.Path)
	}

	// Step 2: Remove the worktree branch from the repository
	if worktree.Branch != "" && worktree.Branch != worktree.SourceBranch {
		if err := execGitCommand(repo.Path, "branch", "-D", worktree.Branch); err != nil {
			log.Printf("⚠️ Failed to remove branch %s (may not exist or be in use): %v", worktree.Branch, err)
		} else {
			log.Printf("✅ Removed branch: %s", worktree.Branch)
		}
	}

	// Step 3: Remove preview branch if it exists
	previewBranchName := fmt.Sprintf("preview/%s", worktree.Branch)
	if err := execGitCommand(repo.Path, "branch", "-D", previewBranchName); err != nil {
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

	// Step 7: Save state
	_ = s.saveState()

	log.Printf("✅ Completed comprehensive cleanup for worktree %s", worktree.Name)
	return nil
}

// cleanupActiveSessions attempts to cleanup any active terminal sessions for this worktree
func (s *GitService) cleanupActiveSessions(worktreePath string) {
	// Kill any processes that might be running in the worktree directory
	// This is a best-effort cleanup
	if err := execCommand("pkill", "-f", worktreePath); err != nil {
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
func (s *GitService) updateWorktreeStatusInternal(worktree *models.Worktree) {
	// Update commit count and commits behind
	if worktree.SourceBranch != "" && worktree.SourceBranch != worktree.Branch {
		// For local repos, ensure we have the latest reference
		if strings.HasPrefix(worktree.RepoID, "local/") {
			// Get the local repo path
			repo, exists := s.repositories[worktree.RepoID]
			if exists {
				// Fetch latest from local main repo
				_ = execGitCommand(worktree.Path, "fetch", repo.Path, fmt.Sprintf("%s:refs/remotes/live/%s", worktree.SourceBranch, worktree.SourceBranch))
			}
		} else {
			// Fetch latest from origin for regular repos
			_ = execGitCommand(worktree.Path, "fetch", "origin", worktree.SourceBranch)
		}

		// Determine source reference based on repo type
		var sourceRef string
		if strings.HasPrefix(worktree.RepoID, "local/") {
			sourceRef = fmt.Sprintf("live/%s", worktree.SourceBranch)
		} else {
			sourceRef = fmt.Sprintf("origin/%s", worktree.SourceBranch)
		}

		// Count commits ahead (our commits)
		if count, err := countCommitsAhead(worktree.Path, sourceRef, "HEAD"); err == nil {
			worktree.CommitCount = count
		}

		// Count commits behind (missing commits)
		if count, err := countCommitsAhead(worktree.Path, "HEAD", sourceRef); err == nil {
			worktree.CommitsBehind = count
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
	output, err := execGitCommandWithCombinedOutput(worktree.Path, "fetch", repo.Path, fmt.Sprintf("%s:refs/remotes/live/%s", worktree.SourceBranch, worktree.SourceBranch))
	if err != nil {
		return fmt.Errorf("failed to fetch from main repo: %v\n%s", err, output)
	}

	// Apply the sync strategy
	var syncArgs []string
	switch strategy {
	case "merge":
		syncArgs = []string{"merge", fmt.Sprintf("live/%s", worktree.SourceBranch)}
	case "rebase":
		syncArgs = []string{"rebase", fmt.Sprintf("live/%s", worktree.SourceBranch)}
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy)
	}

	output, err = execGitCommandWithCombinedOutput(worktree.Path, syncArgs...)
	if err != nil {
		// Check if this is a merge conflict
		if s.isMergeConflict(worktree.Path, string(output)) {
			return s.createMergeConflictError("sync", worktree, string(output))
		}
		return fmt.Errorf("failed to %s: %v\n%s", strategy, err, output)
	}

	// Update worktree status
	_ = s.UpdateWorktreeStatus(worktree.ID)

	log.Printf("✅ Synced worktree %s with %s strategy", worktree.Name, strategy)
	return nil
}

// syncRegularWorktree syncs a regular repo worktree with origin
func (s *GitService) syncRegularWorktree(worktree *models.Worktree, strategy string) error {
	// Fetch from origin
	output, err := execGitCommandWithCombinedOutput(worktree.Path, "fetch", "origin", worktree.SourceBranch)
	if err != nil {
		return fmt.Errorf("failed to fetch from origin: %v\n%s", err, output)
	}

	// Apply the sync strategy
	var syncArgs []string
	switch strategy {
	case "merge":
		syncArgs = []string{"merge", fmt.Sprintf("origin/%s", worktree.SourceBranch)}
	case "rebase":
		syncArgs = []string{"rebase", fmt.Sprintf("origin/%s", worktree.SourceBranch)}
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy)
	}

	output, err = execGitCommandWithCombinedOutput(worktree.Path, syncArgs...)
	if err != nil {
		return fmt.Errorf("failed to %s: %v\n%s", strategy, err, output)
	}

	// Update worktree status
	_ = s.UpdateWorktreeStatus(worktree.ID)

	log.Printf("✅ Synced worktree %s with %s strategy", worktree.Name, strategy)
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
	output, err := execGitCommandWithCombinedOutput(worktree.Path, "push", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, worktree.Branch))
	if err != nil {
		return fmt.Errorf("failed to push worktree branch to main repo: %v\n%s", err, output)
	}

	// Switch to the source branch in main repo and merge
	output, err = execGitCommandWithCombinedOutput(repo.Path, "checkout", worktree.SourceBranch)
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
	output, err = execGitCommandWithCombinedOutput(repo.Path, mergeArgs...)
	if err != nil {
		// Check if this is a merge conflict
		if s.isMergeConflict(repo.Path, string(output)) {
			return s.createMergeConflictError("merge", worktree, string(output))
		}
		return fmt.Errorf("failed to merge worktree branch: %v\n%s", err, output)
	}

	// For squash merges, we need to commit the staged changes
	if squash {
		output, err = execGitCommandWithCombinedOutput(repo.Path, "commit", "-m", fmt.Sprintf("Squash merge branch '%s' from worktree", worktree.Branch))
		if err != nil {
			return fmt.Errorf("failed to commit squash merge: %v\n%s", err, output)
		}
	}

	// Delete the feature branch from main repo (cleanup)
	_ = execGitCommand(repo.Path, "branch", "-d", worktree.Branch) // Ignore errors - branch might be in use

	// Get the new commit hash from the main branch after merge
	output, err = execGitCommandWithCombinedOutput(repo.Path, "rev-parse", "HEAD")
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
	pushArgs := []string{"-C", worktree.Path, "push"}
	if shouldForceUpdate {
		pushArgs = append(pushArgs, "--force")
		log.Printf("🔄 Updating existing preview branch %s", previewBranchName)
	}
	pushArgs = append(pushArgs, repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, previewBranchName))

	cmd := exec.Command("git", pushArgs...)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
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
	if err := execGitCommand(repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", previewBranchName)); err != nil {
		// Branch doesn't exist, safe to create
		return false, nil
	}

	// Branch exists, check if the last commit was made by us (preview commit)
	output, err := execGitCommandWithOutput(repoPath, "log", "-1", "--pretty=format:%s", previewBranchName)
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
	// Check for staged changes
	if execGitCommand(worktreePath, "diff", "--cached", "--quiet") != nil {
		return true, nil // Has staged changes
	}

	// Check for unstaged changes
	if execGitCommand(worktreePath, "diff", "--quiet") != nil {
		return true, nil // Has unstaged changes
	}

	// Check for untracked files
	output, err := execGitCommandWithOutput(worktreePath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return false, fmt.Errorf("failed to check for untracked files: %v", err)
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

// createTemporaryCommit creates a temporary commit with all uncommitted changes
func (s *GitService) createTemporaryCommit(worktreePath string) (string, error) {
	// Add all changes (staged, unstaged, and untracked)
	if output, err := execGitCommandWithCombinedOutput(worktreePath, "add", "."); err != nil {
		return "", fmt.Errorf("failed to stage changes: %v\n%s", err, output)
	}

	// Create the commit
	if output, err := execGitCommandWithCombinedOutput(worktreePath, "commit", "-m", "Preview: Include all uncommitted changes"); err != nil {
		return "", fmt.Errorf("failed to create temporary commit: %v\n%s", err, output)
	}

	// Get the commit hash
	output, err := execGitCommandWithOutput(worktreePath, "rev-parse", "HEAD")
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
	statusOutput, err := execGitCommandWithOutput(repoPath, "status", "--porcelain")
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
	output, err := execGitCommandWithOutput(repoPath, "diff", "--name-only", "--diff-filter=U")
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

	// For local repo worktrees, check conflicts with local main repo
	if strings.HasPrefix(worktree.RepoID, "local/") {
		return s.checkLocalSyncConflicts(worktree)
	}

	// For regular repos, check conflicts with origin
	return s.checkRegularSyncConflicts(worktree)
}

// checkLocalSyncConflicts checks for potential conflicts when syncing a local worktree
func (s *GitService) checkLocalSyncConflicts(worktree *models.Worktree) (*models.MergeConflictError, error) {
	// Get the local repo path
	repo, exists := s.repositories[worktree.RepoID]
	if !exists {
		return nil, fmt.Errorf("local repository %s not found", worktree.RepoID)
	}

	// Fetch from the local main repo to ensure we have latest changes
	if err := execGitCommand(worktree.Path, "fetch", repo.Path, fmt.Sprintf("%s:refs/remotes/live/%s", worktree.SourceBranch, worktree.SourceBranch)); err != nil {
		return nil, fmt.Errorf("failed to fetch for conflict check: %v", err)
	}

	// Try a dry-run merge to detect conflicts
	output, err := execGitCommandWithCombinedOutput(worktree.Path, "merge-tree", "HEAD", fmt.Sprintf("live/%s", worktree.SourceBranch))

	if err != nil {
		return nil, fmt.Errorf("failed to check for conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	outputStr := string(output)
	if strings.Contains(outputStr, "<<<<<<< ") || strings.Contains(outputStr, "======= ") || strings.Contains(outputStr, ">>>>>>> ") {
		// Parse conflicted files from merge-tree output
		conflictFiles := s.parseConflictFiles(outputStr)

		return &models.MergeConflictError{
			Operation:     "sync",
			WorktreeName:  worktree.Name,
			WorktreePath:  worktree.Path,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("Sync would cause conflicts in worktree '%s'", worktree.Name),
		}, nil
	}

	return nil, nil
}

// checkRegularSyncConflicts checks for potential conflicts when syncing a regular worktree
func (s *GitService) checkRegularSyncConflicts(worktree *models.Worktree) (*models.MergeConflictError, error) {
	// Fetch from origin to ensure we have latest changes
	if err := execGitCommand(worktree.Path, "fetch", "origin", worktree.SourceBranch); err != nil {
		return nil, fmt.Errorf("failed to fetch for conflict check: %v", err)
	}

	// Try a dry-run merge to detect conflicts
	output, err := execGitCommandWithCombinedOutput(worktree.Path, "merge-tree", "HEAD", fmt.Sprintf("origin/%s", worktree.SourceBranch))

	if err != nil {
		return nil, fmt.Errorf("failed to check for conflicts: %v", err)
	}

	// Check if merge-tree output indicates conflicts
	outputStr := string(output)
	if strings.Contains(outputStr, "<<<<<<< ") || strings.Contains(outputStr, "======= ") || strings.Contains(outputStr, ">>>>>>> ") {
		// Parse conflicted files from merge-tree output
		conflictFiles := s.parseConflictFiles(outputStr)

		return &models.MergeConflictError{
			Operation:     "sync",
			WorktreeName:  worktree.Name,
			WorktreePath:  worktree.Path,
			ConflictFiles: conflictFiles,
			Message:       fmt.Sprintf("Sync would cause conflicts in worktree '%s'", worktree.Name),
		}, nil
	}

	return nil, nil
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
	if !strings.HasPrefix(worktree.RepoID, "local/") {
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
	if err := execGitCommand(worktree.Path, "push", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, tempBranch)); err != nil {
		return nil, fmt.Errorf("failed to push temp branch for conflict check: %v", err)
	}

	// Clean up temp branch when done
	defer func() {
		_ = execGitCommand(repo.Path, "branch", "-D", tempBranch) // Ignore errors
	}()

	// Try a dry-run merge to detect conflicts
	output, err := execGitCommandWithCombinedOutput(repo.Path, "merge-tree", worktree.SourceBranch, tempBranch)

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

// getRepositoryDefaultBranch gets the default branch from a bare repository
func (s *GitService) getRepositoryDefaultBranch(barePath string) (string, error) {
	return getDefaultBranch(barePath)
}

// branchExistsInBareRepo checks if a branch exists in the bare repository
func (s *GitService) branchExistsInBareRepo(barePath, branch string) bool {
	// Check for local branch first
	if branchExists(barePath, branch) {
		return true
	}

	// Check for remote branch
	return remoteBranchExists(barePath, "origin", branch)
}

// fetchBranchIntoBareRepo fetches a specific branch into the bare repository
func (s *GitService) fetchBranchIntoBareRepo(barePath, branch string) error {
	// First, try to fetch just the remote ref without updating local branch
	// This avoids the "refusing to fetch into branch checked out" error
	output, err := execGitCommandWithCombinedOutput(barePath, "fetch", "origin", fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch), "--depth", "1")
	if err != nil {
		return fmt.Errorf("failed to fetch branch: %v\n%s", err, output)
	}

	// Now create/update the local branch ref from the remote ref
	// Only do this if the branch isn't currently checked out in a worktree
	updateOutput, err := execGitCommandWithCombinedOutput(barePath, "update-ref", fmt.Sprintf("refs/heads/%s", branch), fmt.Sprintf("refs/remotes/origin/%s", branch))
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
	worktreePath := filepath.Join(workspaceDir, repoName, name)

	// Create worktree with new branch using the fun name
	output, err := execGitCommandWithCombinedOutput(repo.Path, "worktree", "add", "-b", name, worktreePath, source)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %v\n%s", err, output)
	}

	// Get current commit hash
	commitOutput, err := execGitCommandWithOutput(worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}

	// Determine source branch (resolve if it's a commit or branch)
	sourceBranch := source
	if len(source) == 40 { // Looks like a commit hash
		// Try to find which branch contains this commit, excluding preview branches
		branchOutput, err := execGitCommandWithOutput(repo.Path, "branch", "--contains", source)
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
		if count, err := countCommitsAhead(worktreePath, sourceBranch, "HEAD"); err == nil {
			commitCount = count
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
	if output, err := execGitCommandWithCombinedOutput(barePath, "fetch", "origin", "--unshallow", branch); err != nil {
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

	// Find the merge base (fork point) between this worktree and its source branch
	mergeBaseOutput, err := execGitCommandWithOutput(worktree.Path, "merge-base", "HEAD", worktree.SourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to find merge base: %v", err)
	}

	forkCommit := strings.TrimSpace(string(mergeBaseOutput))

	// Get the list of changed files from the fork point
	output, err := execGitCommandWithOutput(worktree.Path, "diff", "--name-status", fmt.Sprintf("%s..HEAD", forkCommit))
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
		if oldOutput, err := execGitCommandWithOutput(worktree.Path, "show", fmt.Sprintf("%s:%s", forkCommit, filePath)); err == nil {
			fileDiff.OldContent = string(oldOutput)
		}

		// Get the new content (current HEAD)
		if newOutput, err := execGitCommandWithOutput(worktree.Path, "show", fmt.Sprintf("HEAD:%s", filePath)); err == nil {
			fileDiff.NewContent = string(newOutput)
		}

		// Also keep the unified diff for fallback
		if diffOutput, err := execGitCommandWithOutput(worktree.Path, "diff", fmt.Sprintf("%s..HEAD", forkCommit), "--", filePath); err == nil {
			fileDiff.DiffText = string(diffOutput)
		}

		fileDiffs = append(fileDiffs, fileDiff)
	}

	// Also check for unstaged changes
	if unstagedOutput, err := execGitCommandWithOutput(worktree.Path, "diff", "--name-status"); err == nil {
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
					// Mark as having unstaged changes
					if fileDiffs[i].ChangeType == "modified" {
						fileDiffs[i].ChangeType = "modified (unstaged)"
					}
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
				if oldOutput, err := execGitCommandWithOutput(worktree.Path, "show", fmt.Sprintf("HEAD:%s", filePath)); err == nil {
					fileDiff.OldContent = string(oldOutput)
				}

				// Get new content (working directory)
				if newContent, err := os.ReadFile(filepath.Join(worktree.Path, filePath)); err == nil {
					fileDiff.NewContent = string(newContent)
				}

				// Get unstaged diff content as fallback
				if diffOutput, err := execGitCommandWithOutput(worktree.Path, "diff", "--", filePath); err == nil {
					fileDiff.DiffText = string(diffOutput)
				}

				fileDiffs = append(fileDiffs, fileDiff)
			}
		}
	}

	// Check for untracked files
	if untrackedOutput, err := execGitCommandWithOutput(worktree.Path, "ls-files", "--others", "--exclude-standard"); err == nil {
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

	// Handle local repositories
	if strings.HasPrefix(worktree.RepoID, "local/") {
		return s.createPullRequestForLocalRepo(worktree, repo, title, body)
	}

	// Handle remote repositories
	return s.createPullRequestForRemoteRepo(worktree, repo, title, body)
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

	// First, sync/push any new commits
	if err := s.syncAndPushWorktree(worktree, repo); err != nil {
		return nil, fmt.Errorf("failed to sync worktree before updating PR: %v", err)
	}

	// Get the owner/repo from the repository
	var ownerRepo string
	if strings.HasPrefix(worktree.RepoID, "local/") {
		// Get remote URL from local repo
		remoteURL, err := s.getRemoteURL(worktree.Path)
		if err != nil {
			remoteURL, err = s.getRemoteURL(repo.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to get remote URL: %v", err)
			}
		}
		ownerRepo, err = s.parseGitHubURL(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse remote URL: %v", err)
		}
	} else {
		var err error
		ownerRepo, err = s.parseGitHubURL(repo.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse repository URL: %v", err)
		}
	}

	// Update the PR using GitHub CLI
	return s.updatePullRequestWithGH(worktree, ownerRepo, title, body)
}

// CheckPullRequestExists checks if a pull request exists for a worktree branch
func (s *GitService) CheckPullRequestExists(worktreeID string) (*models.PullRequestInfo, error) {
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

	// Get the owner/repo from the repository
	var ownerRepo string
	if strings.HasPrefix(worktree.RepoID, "local/") {
		// Get remote URL from local repo
		remoteURL, err := s.getRemoteURL(worktree.Path)
		if err != nil {
			remoteURL, err = s.getRemoteURL(repo.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to get remote URL: %v", err)
			}
		}
		ownerRepo, err = s.parseGitHubURL(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse remote URL: %v", err)
		}
	} else {
		var err error
		ownerRepo, err = s.parseGitHubURL(repo.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse repository URL: %v", err)
		}
	}

	// Check if PR exists using GitHub CLI
	return s.checkPullRequestExistsWithGH(worktree, ownerRepo)
}

// createPullRequestForLocalRepo creates a PR for a local repository
func (s *GitService) createPullRequestForLocalRepo(worktree *models.Worktree, repo *models.Repository, title, body string) (*models.PullRequestResponse, error) {
	// First, try to get the remote URL from the worktree (not the main repo)
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
				return nil, fmt.Errorf("local repository does not have a remote 'origin' configured and could not infer GitHub repository URL. Please add a remote first with: git remote add origin <github-repo-url>")
			}
		}
	}

	// Parse the remote URL to get owner/repo
	ownerRepo, err := s.parseGitHubURL(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote URL %s: %v", remoteURL, err)
	}

	// Push the worktree branch to the remote
	if err := s.pushBranchToRemote(worktree, repo, remoteURL); err != nil {
		return nil, fmt.Errorf("failed to push branch to remote: %v", err)
	}

	// Create the pull request using GitHub CLI
	return s.createPullRequestWithGH(worktree, ownerRepo, title, body)
}

// createPullRequestForRemoteRepo creates a PR for a remote repository
func (s *GitService) createPullRequestForRemoteRepo(worktree *models.Worktree, repo *models.Repository, title, body string) (*models.PullRequestResponse, error) {
	// Parse the repository URL to get owner/repo
	ownerRepo, err := s.parseGitHubURL(repo.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository URL %s: %v", repo.URL, err)
	}

	// Push the worktree branch to origin
	if err := s.pushBranchToOrigin(worktree); err != nil {
		return nil, fmt.Errorf("failed to push branch to origin: %v", err)
	}

	// Create the pull request using GitHub CLI
	return s.createPullRequestWithGH(worktree, ownerRepo, title, body)
}

// parseGitHubURL parses a GitHub URL and returns owner/repo
func (s *GitService) parseGitHubURL(url string) (string, error) {
	// Handle various GitHub URL formats
	// https://github.com/owner/repo.git
	// git@github.com:owner/repo.git
	// https://github.com/owner/repo

	if strings.HasPrefix(url, "git@github.com:") {
		// SSH format: git@github.com:owner/repo.git
		parts := strings.TrimPrefix(url, "git@github.com:")
		parts = strings.TrimSuffix(parts, ".git")
		return parts, nil
	} else if strings.Contains(url, "github.com/") {
		// HTTPS format: https://github.com/owner/repo.git
		parts := strings.Split(url, "github.com/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid GitHub URL format")
		}
		ownerRepo := strings.TrimSuffix(parts[1], ".git")
		return ownerRepo, nil
	}

	return "", fmt.Errorf("URL does not appear to be a GitHub repository")
}

// pushBranchToRemote pushes a worktree branch to the remote repository (for local repos)
func (s *GitService) pushBranchToRemote(worktree *models.Worktree, repo *models.Repository, remoteURL string) error {
	// Store the original remote URL for restoration
	originalURL := remoteURL

	// Convert SSH URL to HTTPS if needed to use GitHub CLI authentication
	httpsURL := s.convertToHTTPS(remoteURL)
	needsRestore := httpsURL != remoteURL

	if needsRestore {
		remoteURL = httpsURL
	}

	// First, check if remote already exists
	cmd := exec.Command("git", "-C", worktree.Path, "remote", "get-url", "origin")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	var existingURL string
	var remoteExists bool

	if output, err := cmd.Output(); err == nil {
		existingURL = strings.TrimSpace(string(output))
		remoteExists = true

		// If it's different from what we want, update it
		if existingURL != remoteURL {
			cmd = exec.Command("git", "-C", worktree.Path, "remote", "set-url", "origin", remoteURL)
			cmd.Env = append(os.Environ(),
				"HOME=/home/catnip",
				"USER=catnip",
			)
			if err := cmd.Run(); err != nil {
				log.Printf("⚠️ Failed to update remote URL: %v", err)
			}
		}
	} else {
		// Add the remote if it doesn't exist
		cmd = exec.Command("git", "-C", worktree.Path, "remote", "add", "origin", remoteURL)
		cmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)
		if err := cmd.Run(); err != nil {
			log.Printf("⚠️ Failed to add remote: %v", err)
		}
		remoteExists = false
	}

	// Push the branch to the remote (but don't let it handle URL conversion again)
	pushErr := s.pushBranchToOriginDirect(worktree)

	// Always restore the original URL if we changed it
	if needsRestore && remoteExists {
		restoreCmd := exec.Command("git", "-C", worktree.Path, "remote", "set-url", "origin", originalURL)
		restoreCmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)
		if err := restoreCmd.Run(); err != nil {
			log.Printf("⚠️ Failed to restore original remote URL %s: %v", originalURL, err)
		}
	}

	if pushErr != nil {
		return pushErr
	}

	log.Printf("✅ Pushed branch %s to remote", worktree.Branch)
	return nil
}

// pushBranchToOrigin pushes a worktree branch to origin (for remote repos)
func (s *GitService) pushBranchToOrigin(worktree *models.Worktree) error {
	// Get the current remote URL
	originalURL, err := s.getRemoteURL(worktree.Path)
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %v", err)
	}

	// Convert SSH URL to HTTPS if needed to use GitHub CLI authentication
	httpsURL := s.convertToHTTPS(originalURL)
	urlWasChanged := false

	if httpsURL != originalURL {
		// Temporarily set the remote URL to HTTPS
		cmd := exec.Command("git", "-C", worktree.Path, "remote", "set-url", "origin", httpsURL)
		cmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)

		if err := cmd.Run(); err != nil {
			log.Printf("⚠️ Failed to set HTTPS remote URL: %v", err)
		} else {
			urlWasChanged = true
		}
	}

	// Execute the push
	cmd := exec.Command("git", "-C", worktree.Path, "push", "-u", "origin", worktree.Branch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	output, err := cmd.CombinedOutput()
	pushErr := err

	// Always restore the original URL if we changed it
	if urlWasChanged {
		restoreCmd := exec.Command("git", "-C", worktree.Path, "remote", "set-url", "origin", originalURL)
		restoreCmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)
		if err := restoreCmd.Run(); err != nil {
			log.Printf("⚠️ Failed to restore original remote URL %s: %v", originalURL, err)
		}
	}

	// Return the push error if there was one
	if pushErr != nil {
		return fmt.Errorf("failed to push branch %s to origin: %v\n%s", worktree.Branch, pushErr, output)
	}

	log.Printf("✅ Pushed branch %s to origin", worktree.Branch)
	return nil
}

// pushBranchToOriginDirect pushes a worktree branch to origin without URL conversion (used by pushBranchToRemote)
func (s *GitService) pushBranchToOriginDirect(worktree *models.Worktree) error {
	cmd := exec.Command("git", "-C", worktree.Path, "push", "-u", "origin", worktree.Branch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push branch %s to origin: %v\n%s", worktree.Branch, err, output)
	}

	return nil
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
	commitCheckCmd := exec.Command("git", "-C", worktree.Path, "rev-list", "--count", fmt.Sprintf("origin/%s..HEAD", worktree.SourceBranch))
	commitCheckCmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	if commitOutput, err := commitCheckCmd.Output(); err == nil {
		commitCount := strings.TrimSpace(string(commitOutput))
		if commitCount == "0" {
			return nil, fmt.Errorf("no commits found between origin/%s and HEAD - cannot create pull request", worktree.SourceBranch)
		}
	}

	// Create the PR using GitHub CLI
	cmd := exec.Command("gh", "pr", "create",
		"--repo", ownerRepo,
		"--title", title,
		"--body", body,
		"--head", worktree.Branch,
		"--base", worktree.SourceBranch)

	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is due to missing base branch on remote
		errorOutput := string(output)
		if strings.Contains(errorOutput, "Base ref must be a branch") ||
			strings.Contains(errorOutput, "Base sha can't be blank") ||
			strings.Contains(errorOutput, "Not Found") {
			log.Printf("⚠️ Base branch %s not found on remote, attempting to push it", worktree.SourceBranch)

			// Try to push the base branch to remote
			if pushErr := s.pushBaseBranchToRemote(worktree); pushErr != nil {
				return nil, fmt.Errorf("failed to push base branch %s: %v", worktree.SourceBranch, pushErr)
			}

			// Retry PR creation
			log.Printf("🔄 Retrying PR creation after pushing base branch")
			retryCmd := exec.Command("gh", "pr", "create",
				"--repo", ownerRepo,
				"--title", title,
				"--body", body,
				"--head", worktree.Branch,
				"--base", worktree.SourceBranch)

			retryCmd.Env = append(os.Environ(),
				"HOME=/home/catnip",
				"USER=catnip",
			)

			retryOutput, retryErr := retryCmd.CombinedOutput()
			if retryErr != nil {
				// If still fails, try with main as fallback
				log.Printf("⚠️ PR creation still failed, attempting with 'main' as base")
				return s.retryPullRequestWithMain(worktree, ownerRepo, title, body)
			}
			output = retryOutput
		} else {
			return nil, fmt.Errorf("failed to create pull request: %v\n%s", err, output)
		}
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

// getRemoteURL attempts to get the remote URL from the repository
func (s *GitService) getRemoteURL(repoPath string) (string, error) {
	return getRemoteURL(repoPath, "origin")
}

// convertToHTTPS converts SSH GitHub URLs to HTTPS for GitHub CLI authentication
func (s *GitService) convertToHTTPS(url string) string {
	// Convert git@github.com:owner/repo.git to https://github.com/owner/repo.git
	if strings.HasPrefix(url, "git@github.com:") {
		// Remove git@github.com: prefix
		path := strings.TrimPrefix(url, "git@github.com:")
		// Return HTTPS URL
		return "https://github.com/" + path
	}

	// Already HTTPS or other format, return as-is
	return url
}

// inferRemoteURL attempts to infer the remote URL from git config or other sources
func (s *GitService) inferRemoteURL(repoPath string) (string, error) {
	// Method 1: Check git config for remote.origin.url
	cmd := exec.Command("git", "-C", repoPath, "config", "--get", "remote.origin.url")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	if output, err := cmd.Output(); err == nil {
		url := strings.TrimSpace(string(output))
		if url != "" {
			return url, nil
		}
	}

	// Method 2: Check if we can find any GitHub-related URLs in git config
	cmd = exec.Command("git", "-C", repoPath, "config", "--get-regexp", "remote\\..*\\.url")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "github.com") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					url := parts[1]
					return url, nil
				}
			}
		}
	}

	// Method 3: Check git log for any GitHub-related information
	cmd = exec.Command("git", "-C", repoPath, "log", "--oneline", "--grep=github.com", "-n", "10")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	if output, err := cmd.Output(); err == nil && len(output) > 0 {
		log.Printf("🔍 [DEBUG] Found GitHub references in git log, but cannot automatically infer URL")
	}

	return "", fmt.Errorf("could not infer remote URL from repository")
}

// pushBaseBranchToRemote pushes the base branch to remote if it doesn't exist
func (s *GitService) pushBaseBranchToRemote(worktree *models.Worktree) error {
	// Check if the base branch exists locally
	cmd := exec.Command("git", "-C", worktree.Path, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", worktree.SourceBranch))
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("base branch %s does not exist locally", worktree.SourceBranch)
	}

	// Push the base branch to remote
	cmd = exec.Command("git", "-C", worktree.Path, "push", "-u", "origin", worktree.SourceBranch)
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push base branch %s: %v\n%s", worktree.SourceBranch, err, output)
	}

	log.Printf("✅ Successfully pushed base branch %s to remote", worktree.SourceBranch)
	return nil
}

// retryPullRequestWithMain retries PR creation with 'main' as the base branch
func (s *GitService) retryPullRequestWithMain(worktree *models.Worktree, ownerRepo, title, body string) (*models.PullRequestResponse, error) {
	// Check if there are commits between main and HEAD
	commitCheckCmd := exec.Command("git", "-C", worktree.Path, "rev-list", "--count", "origin/main..HEAD")
	commitCheckCmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	commitOutput, err := commitCheckCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("retry failed: could not check commits between origin/main and HEAD: %v", err)
	}

	commitCount := strings.TrimSpace(string(commitOutput))
	if commitCount == "0" {
		return nil, fmt.Errorf("retry failed: no commits found between origin/main and HEAD")
	}

	log.Printf("📝 Retry: Found %s commits between origin/main and HEAD", commitCount)

	// Create the PR using GitHub CLI with 'main' as base
	cmd := exec.Command("gh", "pr", "create",
		"--repo", ownerRepo,
		"--title", title,
		"--body", body,
		"--head", worktree.Branch,
		"--base", "main")

	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("retry failed: %v\n%s", err, output)
	}

	// Parse the PR URL from the output
	prURL := strings.TrimSpace(string(output))

	// Extract PR number from URL
	var prNumber int
	if strings.Contains(prURL, "/pull/") {
		parts := strings.Split(prURL, "/pull/")
		if len(parts) == 2 {
			if num, err := strconv.Atoi(parts[1]); err == nil {
				prNumber = num
			}
		}
	}

	log.Printf("✅ Created pull request #%d (retry with main): %s", prNumber, prURL)

	return &models.PullRequestResponse{
		Number:     prNumber,
		URL:        prURL,
		Title:      title,
		Body:       body,
		HeadBranch: worktree.Branch,
		BaseBranch: "main",
		Repository: ownerRepo,
	}, nil
}

// syncAndPushWorktree syncs the worktree and pushes any new commits to remote
func (s *GitService) syncAndPushWorktree(worktree *models.Worktree, repo *models.Repository) error {
	// For local repos, push to remote
	if strings.HasPrefix(worktree.RepoID, "local/") {
		// Get remote URL
		remoteURL, err := s.getRemoteURL(worktree.Path)
		if err != nil {
			remoteURL, err = s.getRemoteURL(repo.Path)
			if err != nil {
				return fmt.Errorf("failed to get remote URL: %v", err)
			}
		}

		// Push the branch to remote
		return s.pushBranchToRemote(worktree, repo, remoteURL)
	}

	// For remote repos, push to origin
	return s.pushBranchToOrigin(worktree)
}

// updatePullRequestWithGH updates an existing pull request using GitHub CLI
func (s *GitService) updatePullRequestWithGH(worktree *models.Worktree, ownerRepo, title, body string) (*models.PullRequestResponse, error) {
	// Update the PR using GitHub CLI
	cmd := exec.Command("gh", "pr", "edit", worktree.Branch,
		"--repo", ownerRepo,
		"--title", title,
		"--body", body)

	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to update pull request: %v\n%s", err, output)
	}

	// Get the updated PR info
	prInfo, err := s.getPullRequestInfo(worktree, ownerRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated PR info: %v", err)
	}

	log.Printf("✅ Updated pull request #%d: %s", prInfo.Number, prInfo.URL)

	return &models.PullRequestResponse{
		Number:     prInfo.Number,
		URL:        prInfo.URL,
		Title:      title,
		Body:       body,
		HeadBranch: worktree.Branch,
		BaseBranch: worktree.SourceBranch,
		Repository: ownerRepo,
	}, nil
}

// checkPullRequestExistsWithGH checks if a pull request exists for a branch using GitHub CLI
func (s *GitService) checkPullRequestExistsWithGH(worktree *models.Worktree, ownerRepo string) (*models.PullRequestInfo, error) {
	// Get the current remote URL
	originalURL, err := s.getRemoteURL(worktree.Path)
	if err != nil {
		log.Printf("⚠️ Failed to get remote URL for PR check: %v", err)
		// Continue without URL conversion
		originalURL = ""
	}

	// Convert SSH URL to HTTPS if needed to use GitHub CLI authentication
	var urlWasChanged bool
	if originalURL != "" {
		httpsURL := s.convertToHTTPS(originalURL)
		if httpsURL != originalURL {
			// Temporarily set the remote URL to HTTPS
			cmd := exec.Command("git", "-C", worktree.Path, "remote", "set-url", "origin", httpsURL)
			cmd.Env = append(os.Environ(),
				"HOME=/home/catnip",
				"USER=catnip",
			)

			if err := cmd.Run(); err != nil {
				log.Printf("⚠️ Failed to set HTTPS remote URL for PR check: %v", err)
			} else {
				urlWasChanged = true
			}
		}
	}

	// Check if there are commits ahead of remote
	hasCommitsAhead, err := s.checkCommitsAheadOfRemote(worktree)
	if err != nil {
		log.Printf("⚠️ Failed to check commits ahead for worktree %s: %v", worktree.Name, err)
		// Don't fail the entire operation, but use a sensible default
		// If we have commits in the worktree, assume they're ahead
		hasCommitsAhead = worktree.CommitCount > 0
	}

	// Check if PR exists using GitHub CLI
	cmd := exec.Command("gh", "pr", "view", worktree.Branch,
		"--repo", ownerRepo,
		"--json", "number,url,title,body,headRefName,baseRefName")

	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)

	output, err := cmd.CombinedOutput()
	prCheckErr := err

	// Always restore the original URL if we changed it
	if urlWasChanged {
		restoreCmd := exec.Command("git", "-C", worktree.Path, "remote", "set-url", "origin", originalURL)
		restoreCmd.Env = append(os.Environ(),
			"HOME=/home/catnip",
			"USER=catnip",
		)
		if err := restoreCmd.Run(); err != nil {
			log.Printf("⚠️ Failed to restore original remote URL %s: %v", originalURL, err)
		}
	}

	// Handle the PR check result
	if prCheckErr != nil {
		// If command fails, it likely means no PR exists
		return &models.PullRequestInfo{
			Exists:          false,
			HasCommitsAhead: hasCommitsAhead,
		}, nil
	}

	// Parse the JSON response
	var prData struct {
		Number      int    `json:"number"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		Body        string `json:"body"`
		HeadRefName string `json:"headRefName"`
		BaseRefName string `json:"baseRefName"`
	}

	if err := json.Unmarshal(output, &prData); err != nil {
		return nil, fmt.Errorf("failed to parse PR data: %v", err)
	}

	return &models.PullRequestInfo{
		Exists:          true,
		Number:          prData.Number,
		URL:             prData.URL,
		Title:           prData.Title,
		Body:            prData.Body,
		HeadBranch:      prData.HeadRefName,
		BaseBranch:      prData.BaseRefName,
		Repository:      ownerRepo,
		HasCommitsAhead: hasCommitsAhead,
	}, nil
}

// getPullRequestInfo gets PR information for a branch
func (s *GitService) getPullRequestInfo(worktree *models.Worktree, ownerRepo string) (*models.PullRequestInfo, error) {
	return s.checkPullRequestExistsWithGH(worktree, ownerRepo)
}

// checkCommitsAheadOfRemote checks if there are commits ahead of the remote branch
func (s *GitService) checkCommitsAheadOfRemote(worktree *models.Worktree) (bool, error) {
	// For PR purposes, we need to check if the current branch has commits that haven't been pushed
	// to the remote branch with the same name (origin/current_branch)
	remoteBranchRef := fmt.Sprintf("origin/%s", worktree.Branch)

	// First, try to fetch the specific branch from origin to ensure we have up-to-date refs
	if _, fetchErr := execGitCommandWithCombinedOutput(worktree.Path, "fetch", "origin", worktree.Branch); fetchErr != nil {
		// If fetch fails, the remote branch might not exist
		// In this case, if we have any commits, they're ahead of remote
		if worktree.CommitCount > 0 {
			return true, nil
		}
		return false, nil
	}

	// Count commits ahead of the remote branch with the same name
	count, err := countCommitsAhead(worktree.Path, remoteBranchRef, "HEAD")
	if err == nil {
		return count > 0, nil
	}

	log.Printf("🔍 [DEBUG] Rev-list error for %s: %v", remoteBranchRef, err)
	// If the rev-list command fails, it might mean the remote branch doesn't exist locally
	// Check if the remote branch exists
	if checkErr := execGitCommand(worktree.Path, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/remotes/%s", remoteBranchRef)); checkErr != nil {
		// Remote branch doesn't exist, so if we have commits, they're ahead
		hasCommits := worktree.CommitCount > 0
		log.Printf("🔍 [DEBUG] Remote branch %s doesn't exist locally, using commit count: %d", remoteBranchRef, worktree.CommitCount)
		return hasCommits, nil
	}

	// Remote branch exists but rev-list failed - return error
	return false, fmt.Errorf("failed to count commits ahead of %s: %v", remoteBranchRef, err)
}
