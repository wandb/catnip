package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vanpelt/catnip/internal/models"
)

// CommitSyncService monitors worktrees for commits and syncs them to the bare repository
type CommitSyncService struct {
	gitService   *GitService
	watcher      *fsnotify.Watcher
	syncInterval time.Duration
	mu           sync.RWMutex
	stopChan     chan struct{}
	running      bool
}

// CommitInfo represents information about a detected commit
type CommitInfo struct {
	WorktreePath string
	CommitHash   string
	Branch       string
	Message      string
	Author       string
	Timestamp    time.Time
}

// NewCommitSyncService creates a new commit synchronization service
func NewCommitSyncService(gitService *GitService) *CommitSyncService {
	return &CommitSyncService{
		gitService:   gitService,
		syncInterval: 5 * time.Second,
		stopChan:     make(chan struct{}),
	}
}

// findRepositoryForWorktree finds the repository associated with a worktree path
func (css *CommitSyncService) findRepositoryForWorktree(worktreePath string) (*models.Repository, error) {
	// Get all worktrees and find the one matching this path
	worktrees := css.gitService.ListWorktrees()
	for _, worktree := range worktrees {
		if worktree.Path == worktreePath {
			// Get the repository for this worktree
			status := css.gitService.GetStatus()
			if repo, exists := status.Repositories[worktree.RepoID]; exists {
				return repo, nil
			}
			return nil, fmt.Errorf("repository %s not found for worktree %s", worktree.RepoID, worktreePath)
		}
	}
	return nil, fmt.Errorf("worktree not found for path: %s", worktreePath)
}

// Start begins monitoring worktrees for commits
func (css *CommitSyncService) Start() error {
	css.mu.Lock()
	defer css.mu.Unlock()

	if css.running {
		return fmt.Errorf("commit sync service is already running")
	}

	var err error
	css.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create filesystem watcher: %v", err)
	}

	css.running = true
	log.Printf("🔄 Starting commit synchronization service")

	// Clean up any orphaned sync remotes from previous runs
	css.cleanupOrphanedRemotes()

	// Start filesystem monitoring goroutine
	go css.monitorFilesystem()

	// Start periodic sync goroutine as backup
	go css.periodicSync()

	// Set up initial watchers for existing worktrees
	css.setupWatchers()

	return nil
}

// Stop stops the commit synchronization service
func (css *CommitSyncService) Stop() {
	css.mu.Lock()
	defer css.mu.Unlock()

	if !css.running {
		return
	}

	css.running = false
	close(css.stopChan)

	if css.watcher != nil {
		css.watcher.Close()
	}

	log.Printf("🛑 Stopped commit synchronization service")
}

// setupWatchers sets up filesystem watchers for existing worktrees
func (css *CommitSyncService) setupWatchers() {
	worktrees := css.gitService.ListWorktrees()

	for _, worktree := range worktrees {
		css.addWorktreeWatcher(worktree.Path)
	}
}

// AddWorktreeWatcher adds a filesystem watcher for a new worktree
func (css *CommitSyncService) AddWorktreeWatcher(worktreePath string) {
	css.mu.RLock()
	defer css.mu.RUnlock()

	if !css.running {
		return
	}

	css.addWorktreeWatcher(worktreePath)
}

// addWorktreeWatcher adds a watcher for a specific worktree (internal)
func (css *CommitSyncService) addWorktreeWatcher(worktreePath string) {
	if css.watcher == nil {
		return
	}

	// Watch the .git directory for changes
	gitDir := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		// Watch refs/heads for new commits
		refsDir := filepath.Join(gitDir, "refs", "heads")
		if _, err := os.Stat(refsDir); err == nil {
			if err := css.watcher.Add(refsDir); err != nil {
				log.Printf("⚠️ Failed to watch refs directory %s: %v", refsDir, err)
			} else {
				log.Printf("👀 Watching worktree for commits: %s", worktreePath)
			}
		}
	}
}

// monitorFilesystem monitors filesystem events for Git commits
func (css *CommitSyncService) monitorFilesystem() {
	for {
		select {
		case <-css.stopChan:
			return

		case event, ok := <-css.watcher.Events:
			if !ok {
				return
			}

			// Check if this is a ref update (commit)
			if css.isCommitEvent(event) {
				css.handleCommitEvent(event)
			}

		case err, ok := <-css.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("❌ Filesystem watcher error: %v", err)
		}
	}
}

// isCommitEvent checks if a filesystem event represents a new commit
func (css *CommitSyncService) isCommitEvent(event fsnotify.Event) bool {
	// Look for writes to files in refs/heads (branch updates)
	if event.Op&fsnotify.Write == fsnotify.Write {
		return strings.Contains(event.Name, "refs/heads/")
	}
	return false
}

// handleCommitEvent processes a detected commit event
func (css *CommitSyncService) handleCommitEvent(event fsnotify.Event) {
	// Extract worktree path from the event path
	// Event path: /workspace/repo/branch/.git/refs/heads/branchname
	worktreePath := css.extractWorktreePath(event.Name)
	if worktreePath == "" {
		return
	}

	log.Printf("📝 Detected commit in worktree: %s", worktreePath)

	// Get commit information
	commitInfo, err := css.getCommitInfo(worktreePath)
	if err != nil {
		log.Printf("❌ Failed to get commit info for %s: %v", worktreePath, err)
		return
	}

	// Sync the commit to bare repository
	if err := css.syncCommitToBareRepo(commitInfo); err != nil {
		log.Printf("❌ Failed to sync commit to bare repo: %v", err)
	} else {
		log.Printf("✅ Synced commit %s to bare repository", commitInfo.CommitHash[:8])
	}
}

// extractWorktreePath extracts the worktree path from a Git refs file path
func (css *CommitSyncService) extractWorktreePath(refsPath string) string {
	// Convert /workspace/repo/branch/.git/refs/heads/branchname to /workspace/repo/branch
	parts := strings.Split(refsPath, string(filepath.Separator))

	for i, part := range parts {
		if part == ".git" && i > 0 {
			// Return path up to but not including .git
			return filepath.Join(parts[:i]...)
		}
	}

	return ""
}

// getCommitInfo retrieves information about the latest commit in a worktree
func (css *CommitSyncService) getCommitInfo(worktreePath string) (*CommitInfo, error) {
	// Get commit hash
	cmd := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD")
	hashOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}
	commitHash := strings.TrimSpace(string(hashOutput))

	// Get branch name
	cmd = exec.Command("git", "-C", worktreePath, "branch", "--show-current")
	branchOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get branch name: %v", err)
	}
	branch := strings.TrimSpace(string(branchOutput))

	// Get commit message
	cmd = exec.Command("git", "-C", worktreePath, "log", "-1", "--pretty=format:%s")
	messageOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit message: %v", err)
	}
	message := strings.TrimSpace(string(messageOutput))

	// Get author
	cmd = exec.Command("git", "-C", worktreePath, "log", "-1", "--pretty=format:%an <%ae>")
	authorOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit author: %v", err)
	}
	author := strings.TrimSpace(string(authorOutput))

	return &CommitInfo{
		WorktreePath: worktreePath,
		CommitHash:   commitHash,
		Branch:       branch,
		Message:      message,
		Author:       author,
		Timestamp:    time.Now(),
	}, nil
}

// syncCommitToBareRepo syncs a commit from a worktree to the bare repository
func (css *CommitSyncService) syncCommitToBareRepo(commitInfo *CommitInfo) error {
	// Lock to prevent concurrent sync operations from interfering
	css.mu.Lock()
	defer css.mu.Unlock()

	// Find the repository for this worktree
	repo, err := css.findRepositoryForWorktree(commitInfo.WorktreePath)
	if err != nil {
		return fmt.Errorf("failed to find repository for worktree: %v", err)
	}

	bareRepoPath := repo.Path

	// Verify the commit exists in the worktree before trying to sync
	verifyCmd := exec.Command("git", "-C", commitInfo.WorktreePath, "cat-file", "-e", commitInfo.CommitHash)
	if err := verifyCmd.Run(); err != nil {
		log.Printf("⚠️ Commit %s doesn't exist in worktree %s, skipping sync", commitInfo.CommitHash[:8], commitInfo.WorktreePath)
		return nil // Skip rather than error
	}

	// Check if commit already exists in bare repo
	checkCmd := exec.Command("git", "-C", bareRepoPath, "cat-file", "-e", commitInfo.CommitHash)
	if err := checkCmd.Run(); err == nil {
		// Commit already exists, just update the ref
		updateRefCmd := exec.Command("git", "-C", bareRepoPath, "update-ref",
			fmt.Sprintf("refs/heads/%s", commitInfo.Branch), commitInfo.CommitHash)
		updateOutput, err := updateRefCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to update branch ref: %v\n%s", err, updateOutput)
		}
		log.Printf("🔄 Updated branch ref %s to existing commit %s", commitInfo.Branch, commitInfo.CommitHash[:8])
		return nil
	}

	// Fetch the commit from the worktree to the bare repository
	// Create unique remote name using repo ID to avoid conflicts between repositories
	repoID := strings.ReplaceAll(repo.ID, "/", "-")
	remoteName := fmt.Sprintf("sync-%s-%s", repoID, strings.ReplaceAll(commitInfo.Branch, "/", "-"))

	// Remove existing remote first to avoid conflicts
	removeRemoteCmd := exec.Command("git", "-C", bareRepoPath, "remote", "remove", remoteName)
	_ = removeRemoteCmd.Run() // Ignore error - remote might not exist

	// Add remote
	addRemoteCmd := exec.Command("git", "-C", bareRepoPath, "remote", "add", remoteName, commitInfo.WorktreePath)
	if err := addRemoteCmd.Run(); err != nil {
		return fmt.Errorf("failed to add remote: %v", err)
	}

	// Check if bare repo is shallow before deciding on fetch strategy
	isShallow := false
	if _, err := os.Stat(filepath.Join(bareRepoPath, "shallow")); err == nil {
		isShallow = true
	}

	// Fetch from the worktree - use unshallow only if repo is shallow
	var fetchCmd *exec.Cmd
	if isShallow {
		log.Printf("🔄 Bare repo is shallow, using --unshallow for %s", commitInfo.Branch)
		fetchCmd = exec.Command("git", "-C", bareRepoPath, "fetch", "--unshallow", remoteName, commitInfo.Branch)
	} else {
		fetchCmd = exec.Command("git", "-C", bareRepoPath, "fetch", remoteName, commitInfo.Branch)
	}

	output, err := fetchCmd.CombinedOutput()
	if err != nil {
		// If unshallow fails, try regular fetch as fallback
		if isShallow {
			log.Printf("⚠️ Unshallow fetch failed, trying regular fetch: %s", string(output))
			fetchCmd = exec.Command("git", "-C", bareRepoPath, "fetch", remoteName, commitInfo.Branch)
			output, err = fetchCmd.CombinedOutput()
		}
		if err != nil {
			// Clean up the remote before returning error
			_ = removeRemoteCmd.Run()
			return fmt.Errorf("failed to fetch from worktree: %v\n%s", err, output)
		}
	}

	// Update the branch ref in the bare repository
	updateRefCmd := exec.Command("git", "-C", bareRepoPath, "update-ref",
		fmt.Sprintf("refs/heads/%s", commitInfo.Branch), commitInfo.CommitHash)
	updateOutput, err := updateRefCmd.CombinedOutput()
	if err != nil {
		// Clean up the remote before returning error
		_ = removeRemoteCmd.Run()
		return fmt.Errorf("failed to update branch ref: %v\n%s", err, updateOutput)
	}

	// Clean up the temporary remote
	_ = removeRemoteCmd.Run()

	return nil
}

// periodicSync performs periodic synchronization as a backup to filesystem monitoring
func (css *CommitSyncService) periodicSync() {
	ticker := time.NewTicker(css.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-css.stopChan:
			return
		case <-ticker.C:
			css.performPeriodicSync()
		}
	}
}

// PerformManualSync manually triggers a sync check (public method)
func (css *CommitSyncService) PerformManualSync() {
	css.performPeriodicSync()
}

// performPeriodicSync checks all worktrees for unsync'd commits
func (css *CommitSyncService) performPeriodicSync() {
	worktrees := css.gitService.ListWorktrees()

	for _, worktree := range worktrees {
		// Check if worktree has commits that aren't in the bare repo
		if css.hasUnsyncedCommits(worktree.Path) {
			commitInfo, err := css.getCommitInfo(worktree.Path)
			if err != nil {
				log.Printf("⚠️ Failed to get commit info during periodic sync for %s: %v", worktree.Path, err)
				continue
			}

			if err := css.syncCommitToBareRepo(commitInfo); err != nil {
				log.Printf("⚠️ Failed to sync commit during periodic sync: %v", err)
			} else {
				log.Printf("🔄 Periodic sync: synced commit %s", commitInfo.CommitHash[:8])
			}
		}
	}
}

// cleanupOrphanedRemotes removes any leftover sync remotes from previous runs
func (css *CommitSyncService) cleanupOrphanedRemotes() {
	status := css.gitService.GetStatus()
	if len(status.Repositories) == 0 {
		return
	}

	// Clean up remotes for all repositories
	for _, repo := range status.Repositories {
		css.cleanupOrphanedRemotesForRepo(repo.Path)
	}
}

// cleanupOrphanedRemotesForRepo removes orphaned remotes for a specific repository
func (css *CommitSyncService) cleanupOrphanedRemotesForRepo(bareRepoPath string) {

	// List all remotes
	cmd := exec.Command("git", "-C", bareRepoPath, "remote")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ Failed to list remotes for cleanup: %v", err)
		return
	}

	// Remove any remotes that start with "sync-" or "worktree-"
	remotes := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, remote := range remotes {
		remote = strings.TrimSpace(remote)
		if strings.HasPrefix(remote, "sync-") || strings.HasPrefix(remote, "worktree-") {
			log.Printf("🧹 Cleaning up orphaned remote: %s", remote)
			removeCmd := exec.Command("git", "-C", bareRepoPath, "remote", "remove", remote)
			_ = removeCmd.Run() // Ignore errors
		}
	}
}

// hasUnsyncedCommits checks if a worktree has commits not in the bare repository
func (css *CommitSyncService) hasUnsyncedCommits(worktreePath string) bool {
	// Find the repository for this worktree
	repo, err := css.findRepositoryForWorktree(worktreePath)
	if err != nil {
		return false
	}

	// Get worktree HEAD
	cmd := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD")
	worktreeHead, err := cmd.Output()
	if err != nil {
		return false
	}

	// Get branch name
	cmd = exec.Command("git", "-C", worktreePath, "branch", "--show-current")
	branchOutput, err := cmd.Output()
	if err != nil {
		return false
	}
	branch := strings.TrimSpace(string(branchOutput))

	// Get bare repo HEAD for this branch
	cmd = exec.Command("git", "-C", repo.Path, "rev-parse", fmt.Sprintf("refs/heads/%s", branch))
	bareHead, err := cmd.Output()
	if err != nil {
		// Branch doesn't exist in bare repo, so it's definitely unsynced
		return true
	}

	// Compare HEADs
	return strings.TrimSpace(string(worktreeHead)) != strings.TrimSpace(string(bareHead))
}
