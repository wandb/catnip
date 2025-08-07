package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/models"
)

// CommitSyncService monitors worktrees for commits and syncs them to the bare repository
type CommitSyncService struct {
	gitService   *GitService
	operations   git.Operations
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
		operations:   git.NewOperations(),
		syncInterval: 30 * time.Second, // Less aggressive - only syncing existing commits
		stopChan:     make(chan struct{}),
	}
}

// NewCommitSyncServiceWithOperations creates a new commit sync service with custom operations (for testing)
func NewCommitSyncServiceWithOperations(gitService *GitService, operations git.Operations) *CommitSyncService {
	return &CommitSyncService{
		gitService:   gitService,
		operations:   operations,
		syncInterval: 30 * time.Second, // Less aggressive - only syncing existing commits
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

	// For worktrees, check if .git is a file pointing to the main repo
	var mainRepoGitDir string
	if gitDirContent, err := os.ReadFile(gitDir); err == nil {
		// This is a worktree - .git file contains "gitdir: /path/to/main/repo/.git/worktrees/name"
		gitDirStr := string(gitDirContent)
		if strings.HasPrefix(gitDirStr, "gitdir: ") {
			worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirStr, "gitdir: "))
			// Extract main repo git directory: /path/to/main/repo/.git/worktrees/name -> /path/to/main/repo/.git
			parts := strings.Split(worktreeGitDir, string(filepath.Separator))
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i] == ".git" {
					mainRepoGitDir = strings.Join(parts[:i+1], string(filepath.Separator))
					break
				}
			}
		}
	}

	// Watch both the worktree's local refs and the main repo's custom refs
	var refsDirsToWatch []string

	if _, err := os.Stat(gitDir); err == nil {
		// For regular worktrees, watch local refs/heads
		localRefsDir := filepath.Join(gitDir, "refs", "heads")
		if _, err := os.Stat(localRefsDir); err == nil {
			refsDirsToWatch = append(refsDirsToWatch, localRefsDir)
		}
	}

	// For custom refs like refs/catnip/*, watch the main repository's refs/catnip directory
	if mainRepoGitDir != "" {
		catnipRefsDir := filepath.Join(mainRepoGitDir, "refs", "catnip")
		if _, err := os.Stat(catnipRefsDir); err == nil {
			refsDirsToWatch = append(refsDirsToWatch, catnipRefsDir)
		}
	}

	for _, refsDir := range refsDirsToWatch {
		if err := css.watcher.Add(refsDir); err != nil {
			log.Printf("⚠️ Failed to watch refs directory %s: %v", refsDir, err)
		} else {
			log.Printf("👀 Watching refs directory: %s", refsDir)
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
	// Look for writes to files in refs/heads or refs/catnip (branch updates)
	if event.Op&fsnotify.Write == fsnotify.Write {
		return strings.Contains(event.Name, "refs/heads/") || strings.Contains(event.Name, "refs/catnip/")
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
	// Handle two cases:
	// 1. Regular worktree refs: /workspace/repo/branch/.git/refs/heads/branchname -> /workspace/repo/branch
	// 2. Main repo custom refs: /live/repo/.git/refs/catnip/workspacename -> find corresponding worktree

	parts := strings.Split(refsPath, string(filepath.Separator))

	// Check if this is a custom ref from the main repository (refs/catnip/*)
	if strings.Contains(refsPath, "refs/catnip/") {
		// Extract workspace name from refs/catnip/workspacename
		for i, part := range parts {
			if part == "catnip" && i+1 < len(parts) {
				workspaceName := parts[i+1]
				// Find the corresponding worktree by checking all worktrees
				return css.findWorktreePathByName(workspaceName)
			}
		}
	}

	// Regular case: find .git directory and return path up to it
	for i, part := range parts {
		if part == ".git" && i > 0 {
			// Return path up to but not including .git
			return filepath.Join(parts[:i]...)
		}
	}

	return ""
}

// findWorktreePathByName finds the worktree path for a given workspace name
func (css *CommitSyncService) findWorktreePathByName(workspaceName string) string {
	// Get all worktrees from the git service
	worktrees := css.gitService.ListWorktrees()
	for _, worktree := range worktrees {
		if worktree.Name == workspaceName {
			return worktree.Path
		}
	}
	return ""
}

// getCommitInfo retrieves information about the latest commit in a worktree
func (css *CommitSyncService) getCommitInfo(worktreePath string) (*CommitInfo, error) {
	// Get commit hash
	commitHash, err := css.operations.RevParse(worktreePath, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hash: %v", err)
	}

	// Get full ref name (works for both regular branches and custom refs like refs/catnip/)
	var branch string
	refOutput, err := css.operations.ExecuteGit(worktreePath, "symbolic-ref", "HEAD")
	if err != nil {
		// Fallback to branch --show-current for detached HEAD or other cases
		branchOutput, err := css.operations.ExecuteGit(worktreePath, "branch", "--show-current")
		if err != nil {
			return nil, fmt.Errorf("failed to get branch/ref name: %v", err)
		}
		branch = strings.TrimSpace(string(branchOutput))
		if branch == "" {
			// Detached HEAD state
			branch = "HEAD"
		}
	} else {
		// We have a full ref, store it as is
		branch = strings.TrimSpace(string(refOutput))
	}

	// Get commit message
	messageOutput, err := css.operations.ExecuteGit(worktreePath, "log", "-1", "--pretty=format:%s")
	if err != nil {
		return nil, fmt.Errorf("failed to get commit message: %v", err)
	}
	message := strings.TrimSpace(string(messageOutput))

	// Get author
	authorOutput, err := css.operations.ExecuteGit(worktreePath, "log", "-1", "--pretty=format:%an <%ae>")
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
	// Find the repository for this worktree BEFORE acquiring the lock to avoid holding it during external calls
	repo, err := css.findRepositoryForWorktree(commitInfo.WorktreePath)
	if err != nil {
		return fmt.Errorf("failed to find repository for worktree: %v", err)
	}

	// Lock to prevent concurrent sync operations from interfering
	css.mu.Lock()
	defer css.mu.Unlock()

	bareRepoPath := repo.Path

	// Verify the commit exists in the worktree before trying to sync
	_, err = css.operations.ExecuteGit(commitInfo.WorktreePath, "cat-file", "-e", commitInfo.CommitHash)
	if err != nil {
		log.Printf("⚠️ Commit %s doesn't exist in worktree %s, skipping sync", commitInfo.CommitHash[:8], commitInfo.WorktreePath)
		return nil // Skip rather than error
	}

	// IMPORTANT: Also sync to the nice branch if this is a custom ref
	if strings.HasPrefix(commitInfo.Branch, "refs/catnip/") {
		if err := css.syncToNiceBranch(commitInfo); err != nil {
			log.Printf("⚠️ Failed to sync to nice branch: %v", err)
		}
	}

	// Check if commit already exists in bare repo
	_, err = css.operations.ExecuteGit(bareRepoPath, "cat-file", "-e", commitInfo.CommitHash)
	if err == nil {
		// Commit already exists, just update the ref
		// Handle both full refs (refs/catnip/name) and simple branch names
		refToUpdate := commitInfo.Branch
		if !strings.HasPrefix(refToUpdate, "refs/") {
			refToUpdate = fmt.Sprintf("refs/heads/%s", commitInfo.Branch)
		}
		_, err = css.operations.ExecuteGit(bareRepoPath, "update-ref", refToUpdate, commitInfo.CommitHash)
		if err != nil {
			return fmt.Errorf("failed to update branch ref: %v", err)
		}
		log.Printf("🔄 Updated branch ref %s to existing commit %s", refToUpdate, commitInfo.CommitHash[:8])
		return nil
	}

	// Fetch the commit from the worktree to the bare repository
	// Create unique remote name using repo ID to avoid conflicts between repositories
	repoID := strings.ReplaceAll(repo.ID, "/", "-")
	remoteName := fmt.Sprintf("sync-%s-%s", repoID, strings.ReplaceAll(commitInfo.Branch, "/", "-"))

	// Remove existing remote first to avoid conflicts
	_ = css.operations.RemoveRemote(bareRepoPath, remoteName) // Ignore error - remote might not exist

	// Add remote
	if err := css.operations.AddRemote(bareRepoPath, remoteName, commitInfo.WorktreePath); err != nil {
		return fmt.Errorf("failed to add remote: %v", err)
	}

	// Check if bare repo is shallow before deciding on fetch strategy
	isShallow := false
	if _, err := os.Stat(filepath.Join(bareRepoPath, "shallow")); err == nil {
		isShallow = true
	}

	// Fetch from the worktree - use unshallow only if repo is shallow
	// We need to fetch with the proper refspec for custom refs
	fetchRefspec := commitInfo.Branch
	// If it's a full ref (like refs/catnip/name), we need to fetch it properly
	if strings.HasPrefix(commitInfo.Branch, "refs/") {
		fetchRefspec = fmt.Sprintf("%s:%s", commitInfo.Branch, commitInfo.Branch)
	}

	var output []byte
	if isShallow {
		log.Printf("🔄 Bare repo is shallow, using --unshallow for %s", commitInfo.Branch)
		output, err = css.operations.ExecuteGit(bareRepoPath, "fetch", "--unshallow", remoteName, fetchRefspec)
	} else {
		output, err = css.operations.ExecuteGit(bareRepoPath, "fetch", remoteName, fetchRefspec)
	}

	if err != nil {
		// If unshallow fails, try regular fetch as fallback
		if isShallow {
			log.Printf("⚠️ Unshallow fetch failed, trying regular fetch: %s", string(output))
			output, err = css.operations.ExecuteGit(bareRepoPath, "fetch", remoteName, fetchRefspec)
		}
		if err != nil {
			// Clean up the remote before returning error
			_ = css.operations.RemoveRemote(bareRepoPath, remoteName)
			return fmt.Errorf("failed to fetch from worktree: %v\n%s", err, output)
		}
	}

	// Update the branch ref in the bare repository
	// Handle both full refs (refs/catnip/name) and simple branch names
	refToUpdate := commitInfo.Branch
	if !strings.HasPrefix(refToUpdate, "refs/") {
		refToUpdate = fmt.Sprintf("refs/heads/%s", commitInfo.Branch)
	}
	_, err = css.operations.ExecuteGit(bareRepoPath, "update-ref", refToUpdate, commitInfo.CommitHash)
	if err != nil {
		// Clean up the remote before returning error
		_ = css.operations.RemoveRemote(bareRepoPath, remoteName)
		return fmt.Errorf("failed to update branch ref: %v", err)
	}

	// Clean up the temporary remote
	_ = css.operations.RemoveRemote(bareRepoPath, remoteName)

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

// performPeriodicSync checks all worktrees for unsync'd commits (NO AUTO-COMMITS)
func (css *CommitSyncService) performPeriodicSync() {
	// Get worktrees WITHOUT holding the commit sync lock to avoid deadlocks
	worktrees := css.gitService.ListWorktrees()

	for _, worktree := range worktrees {
		// Only sync existing commits to bare repo (no auto-commits)
		// Let the session-aware CheckpointManager handle creating commits
		if css.hasUnsyncedCommits(worktree.Path) {
			commitInfo, err := css.getCommitInfo(worktree.Path)
			if err != nil {
				log.Printf("⚠️ Failed to get commit info during periodic sync for %s: %v", worktree.Path, err)
				continue
			}

			if err := css.syncCommitToBareRepo(commitInfo); err != nil {
				log.Printf("⚠️ Failed to sync commit during periodic sync: %v", err)
			} else {
				log.Printf("✅ Synced commit %s to bare repository", commitInfo.CommitHash[:8])
			}
		}

		// Check for custom refs that need nice branch syncing
		if strings.HasPrefix(worktree.Branch, "refs/catnip/") {
			// Get current commit info
			commitInfo, err := css.getCommitInfo(worktree.Path)
			if err != nil {
				log.Printf("⚠️ Failed to get commit info for nice branch sync for %s: %v", worktree.Path, err)
			} else {
				// Check if nice branch needs syncing
				if css.hasUnsyncedNiceBranch(commitInfo) {
					if err := css.syncToNiceBranch(commitInfo); err != nil {
						log.Printf("⚠️ Failed to sync to nice branch during periodic sync: %v", err)
					} else {
						log.Printf("✅ Synced nice branch for commit %s", commitInfo.CommitHash[:8])
					}
				}
			}

			// Check for bidirectional sync - external changes to nice branches
			if err := css.syncFromNiceBranch(worktree.Path, worktree.Branch); err != nil {
				log.Printf("⚠️ Failed to sync from nice branch for %s: %v", worktree.Path, err)
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
	// Get all remotes using the operations interface
	remotesMap, err := css.operations.GetRemotes(bareRepoPath)
	if err != nil {
		log.Printf("⚠️ Failed to list remotes for cleanup: %v", err)
		return
	}

	// Remove any remotes that start with "sync-" or "worktree-"
	for remoteName := range remotesMap {
		if strings.HasPrefix(remoteName, "sync-") || strings.HasPrefix(remoteName, "worktree-") {
			log.Printf("🧹 Cleaning up orphaned remote: %s", remoteName)
			_ = css.operations.RemoveRemote(bareRepoPath, remoteName) // Ignore errors
		}
	}
}

// syncToNiceBranch syncs commits from a custom ref to its corresponding nice branch
func (css *CommitSyncService) syncToNiceBranch(commitInfo *CommitInfo) error {
	// Get the nice branch name from git config
	configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(commitInfo.Branch, "/", "."))
	niceBranchOutput, err := css.operations.GetConfig(commitInfo.WorktreePath, configKey)
	if err != nil {
		// No mapping found, skip syncing
		return nil
	}
	niceBranch := strings.TrimSpace(niceBranchOutput)
	if niceBranch == "" {
		return nil
	}

	// Check if the nice branch exists
	if !css.operations.BranchExists(commitInfo.WorktreePath, niceBranch, false) {
		log.Printf("⚠️ Nice branch %s doesn't exist, skipping sync", niceBranch)
		return nil
	}

	// Update the nice branch to point to the same commit as the custom ref
	_, err = css.operations.ExecuteGit(commitInfo.WorktreePath, "branch", "-f", niceBranch, commitInfo.CommitHash)
	if err != nil {
		return fmt.Errorf("failed to update nice branch %s to commit %s: %v", niceBranch, commitInfo.CommitHash[:8], err)
	}

	log.Printf("🔄 Synced nice branch %s to commit %s from %s", niceBranch, commitInfo.CommitHash[:8], commitInfo.Branch)

	// For local repositories, also push the nice branch to the main repository
	repo, err := css.findRepositoryForWorktree(commitInfo.WorktreePath)
	if err == nil && strings.HasPrefix(repo.ID, "local/") {
		// Push the nice branch to the catnip-live remote (which points to the main repo)
		_, pushErr := css.operations.ExecuteGit(commitInfo.WorktreePath, "push", "catnip-live", fmt.Sprintf("%s:%s", niceBranch, niceBranch), "--force-with-lease")
		if pushErr != nil {
			log.Printf("⚠️ Failed to push nice branch to catnip-live remote: %v", pushErr)
		} else {
			log.Printf("✅ Pushed nice branch %s to catnip-live remote", niceBranch)
		}
	}

	return nil
}

// syncFromNiceBranch syncs commits from the nice branch back to the custom ref (bidirectional sync)
func (css *CommitSyncService) syncFromNiceBranch(worktreePath string, customRef string) error {
	// Get the nice branch name from git config
	configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(customRef, "/", "."))
	niceBranchOutput, err := css.operations.GetConfig(worktreePath, configKey)
	if err != nil {
		// No mapping found, skip syncing
		return nil
	}
	niceBranch := strings.TrimSpace(niceBranchOutput)
	if niceBranch == "" {
		return nil
	}

	// Check if the nice branch exists
	if !css.operations.BranchExists(worktreePath, niceBranch, false) {
		// Nice branch doesn't exist, nothing to sync
		return nil
	}

	// Get commit hashes for both branches
	customRefHash, err := css.operations.GetCommitHash(worktreePath, customRef)
	if err != nil {
		return fmt.Errorf("failed to get commit hash for custom ref %s: %v", customRef, err)
	}

	niceBranchHash, err := css.operations.GetCommitHash(worktreePath, niceBranch)
	if err != nil {
		return fmt.Errorf("failed to get commit hash for nice branch %s: %v", niceBranch, err)
	}

	// If they're the same, nothing to sync
	if customRefHash == niceBranchHash {
		return nil
	}

	// Check if nice branch is ahead of custom ref
	mergeBaseOutput, err := css.operations.ExecuteGit(worktreePath, "merge-base", customRef, niceBranch)
	if err != nil {
		log.Printf("⚠️ Failed to find merge base between %s and %s: %v", customRef, niceBranch, err)
		return nil
	}
	mergeBase := strings.TrimSpace(string(mergeBaseOutput))

	// If merge base equals custom ref hash, nice branch is ahead and can be fast-forwarded
	if mergeBase == customRefHash {
		log.Printf("🔄 Nice branch %s is ahead of %s, performing fast-forward sync", niceBranch, customRef)

		// Fast-forward the custom ref to the nice branch
		_, err = css.operations.ExecuteGit(worktreePath, "update-ref", customRef, niceBranchHash)
		if err != nil {
			return fmt.Errorf("failed to fast-forward %s to %s: %v", customRef, niceBranchHash[:8], err)
		}

		log.Printf("✅ Fast-forwarded %s to %s (%s)", customRef, niceBranchHash[:8], niceBranch)
		return nil
	}

	// If merge base equals nice branch hash, custom ref is ahead (already handled by normal sync)
	if mergeBase == niceBranchHash {
		return nil
	}

	// Branches have diverged - this requires a merge
	log.Printf("🔀 Branches have diverged: %s and %s. Attempting merge.", customRef, niceBranch)

	// For now, we'll create a merge commit on the custom ref
	// First, we need to temporarily switch to the custom ref to perform the merge
	currentHeadOutput, _ := css.operations.ExecuteGit(worktreePath, "symbolic-ref", "HEAD")
	currentHead := strings.TrimSpace(string(currentHeadOutput))

	// Switch to custom ref
	_, err = css.operations.ExecuteGit(worktreePath, "symbolic-ref", "HEAD", customRef)
	if err != nil {
		return fmt.Errorf("failed to switch to custom ref for merge: %v", err)
	}

	// Attempt merge
	_, mergeErr := css.operations.ExecuteGit(worktreePath, "merge", "--no-ff", "-m",
		fmt.Sprintf("Merge external changes from %s into %s", niceBranch, customRef), niceBranch)

	// Switch back to original HEAD
	if currentHead != "" {
		if _, err := css.operations.ExecuteGit(worktreePath, "symbolic-ref", "HEAD", currentHead); err != nil {
			log.Printf("⚠️ Failed to switch back to original HEAD: %v", err)
		}
	}

	if mergeErr != nil {
		log.Printf("⚠️ Failed to merge %s into %s: %v", niceBranch, customRef, mergeErr)
		log.Printf("💡 Manual intervention may be required to resolve conflicts")
		return fmt.Errorf("merge conflict: %v", mergeErr)
	}

	log.Printf("✅ Successfully merged external changes from %s into %s", niceBranch, customRef)
	return nil
}

// hasUnsyncedCommits checks if a worktree has commits not in the bare repository
func (css *CommitSyncService) hasUnsyncedCommits(worktreePath string) bool {
	// Find the repository for this worktree
	repo, err := css.findRepositoryForWorktree(worktreePath)
	if err != nil {
		return false
	}

	// Get worktree HEAD
	worktreeHead, err := css.operations.RevParse(worktreePath, "HEAD")
	if err != nil {
		return false
	}

	// Get full ref name (same logic as getCommitInfo)
	var branch string
	refOutput, err := css.operations.ExecuteGit(worktreePath, "symbolic-ref", "HEAD")
	if err != nil {
		// Fallback to branch --show-current for detached HEAD or other cases
		branchOutput, err := css.operations.ExecuteGit(worktreePath, "branch", "--show-current")
		if err != nil {
			return false
		}
		branch = strings.TrimSpace(string(branchOutput))
		if branch == "" {
			// Detached HEAD state
			return false
		}
		// Convert to full ref path for consistency
		branch = fmt.Sprintf("refs/heads/%s", branch)
	} else {
		// We have a full ref, store it as is
		branch = strings.TrimSpace(string(refOutput))
	}

	// Get bare repo HEAD for this ref
	bareHead, err := css.operations.RevParse(repo.Path, branch)
	if err != nil {
		// Branch doesn't exist in bare repo, so it's definitely unsynced
		return true
	}

	// Compare HEADs
	return strings.TrimSpace(worktreeHead) != strings.TrimSpace(bareHead)
}

// hasUnsyncedNiceBranch checks if a custom ref needs syncing to its nice branch
func (css *CommitSyncService) hasUnsyncedNiceBranch(commitInfo *CommitInfo) bool {
	// Only applies to custom refs
	if !strings.HasPrefix(commitInfo.Branch, "refs/catnip/") {
		return false
	}

	// Get the nice branch name from git config
	configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(commitInfo.Branch, "/", "."))
	niceBranchOutput, err := css.operations.GetConfig(commitInfo.WorktreePath, configKey)
	if err != nil {
		// No mapping found, skip syncing
		return false
	}
	niceBranch := strings.TrimSpace(niceBranchOutput)
	if niceBranch == "" {
		return false
	}

	// Check if the nice branch exists
	if !css.operations.BranchExists(commitInfo.WorktreePath, niceBranch, false) {
		return false
	}

	// Get commit hash of the nice branch
	niceBranchHash, err := css.operations.GetCommitHash(commitInfo.WorktreePath, niceBranch)
	if err != nil {
		return false
	}

	// Compare commit hashes - if they're different, nice branch needs syncing
	return strings.TrimSpace(commitInfo.CommitHash) != strings.TrimSpace(niceBranchHash)
}
