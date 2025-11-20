package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/git/templates"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/recovery"
)

// getWorkspaceDir returns the workspace directory for the current runtime
func getWorkspaceDir() string {
	if dir := os.Getenv("CATNIP_WORKSPACE_DIR"); dir != "" {
		return dir
	}
	return config.Runtime.WorkspaceDir
}

// getGitStateDir returns the git state directory based on volume dir
func getGitStateDir() string {
	return config.Runtime.VolumeDir
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
	logger.Debug("🧹 Starting cleanup of unused catnip branches...")

	s.mu.RLock()
	reposMap := s.stateManager.GetAllRepositories()
	s.mu.RUnlock()

	totalDeleted := 0

	for _, repo := range reposMap {
		// Skip unavailable repositories to prevent boot failures
		if !repo.Available {
			logger.Debugf("🔍 Skipping cleanup for unavailable repository %s", repo.ID)
			continue
		}

		// Check if repository path exists before trying to clean it up
		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			// Mark repository as unavailable and skip cleanup
			logger.Warnf("⚠️ Repository %s not available at %s, marking as unavailable", repo.ID, repo.Path)
			repo.Available = false
			continue
		}

		// List all branches in the bare repository
		branches, err := s.operations.ListBranches(repo.Path, git.ListBranchesOptions{All: true})
		if err != nil {
			logger.Warnf("⚠️  Failed to list branches for %s: %v", repo.ID, err)
			// Check if it's a directory access issue and mark repo as unavailable
			if strings.Contains(err.Error(), "cannot change to") || strings.Contains(err.Error(), "No such file or directory") {
				logger.Warnf("⚠️ Repository %s appears to be inaccessible, marking as unavailable", repo.ID)
				repo.Available = false
			}
			continue
		}
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
				if err := s.operations.ShowRef(repo.Path, ref, git.ShowRefOptions{Verify: true, Quiet: true}); err == nil {
					baseRef = ref
					break
				}
			}

			if baseRef == "" {
				continue // Skip if we can't find a base branch
			}

			// Check if branch exists locally
			if !s.operations.BranchExists(repo.Path, branchName, false) {
				continue // Branch doesn't exist locally
			}

			// Count commits ahead of base
			commitCount, err := s.operations.GetCommitCount(repo.Path, baseRef, branchName)
			if err != nil || commitCount > 0 {
				continue // Skip if there are commits or error parsing
			}

			// Also check if there's an active worktree using this branch
			worktrees, err := s.operations.ListWorktrees(repo.Path)
			if err == nil {
				var skipBranch bool
				for _, wt := range worktrees {
					if wt.Branch == branchName {
						skipBranch = true
						break
					}
				}
				if skipBranch {
					continue // Skip if branch is currently checked out in a worktree
				}
			}

			// Delete the branch (local)
			if err := s.operations.DeleteBranch(repo.Path, branchName, true); err == nil {
				deletedInRepo++
				totalDeleted++
				logger.Debugf("🗑️  Deleted unused branch: %s in %s", branchName, repo.ID)
			}
		}

		if deletedInRepo > 0 {
			logger.Infof("✅ Cleaned up %d unused branches in %s", deletedInRepo, repo.ID)
		}
	}

	if totalDeleted > 0 {
		logger.Infof("🧹 Cleanup complete: removed %d unused catnip branches", totalDeleted)
	} else {
		logger.Debug("✅ No unused catnip branches found")
	}
}

// cleanupCatnipRefs provides comprehensive cleanup of refs/catnip/ namespace, checking against state.json
func (s *GitService) cleanupCatnipRefs() {
	logger.Debug("🧹 Starting cleanup of catnip refs namespace...")

	s.mu.RLock()
	reposMap := s.stateManager.GetAllRepositories()
	worktreesMap := s.stateManager.GetAllWorktrees()
	s.mu.RUnlock()

	// Build a set of workspace names that should be preserved
	preservedWorkspaces := make(map[string]bool)
	for _, worktree := range worktreesMap {
		// Extract workspace name from display name (e.g., "catnip/mini-milo" -> "mini-milo")
		if parts := strings.Split(worktree.Name, "/"); len(parts) >= 2 {
			workspaceName := parts[len(parts)-1]
			preservedWorkspaces[workspaceName] = true
		}

		// Also preserve if the branch is already a catnip ref
		if strings.HasPrefix(worktree.Branch, "refs/catnip/") {
			refName := strings.TrimPrefix(worktree.Branch, "refs/catnip/")
			preservedWorkspaces[refName] = true
		}
	}

	logger.Debugf("🔍 Preserving %d workspace refs: %v", len(preservedWorkspaces), preservedWorkspaces)

	totalDeleted := 0

	for _, repo := range reposMap {
		// Skip unavailable repositories to prevent boot failures
		if !repo.Available {
			logger.Debugf("🔍 Skipping catnip refs cleanup for unavailable repository %s", repo.ID)
			continue
		}

		// Check if repository path exists before trying to list refs
		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			// Mark repository as unavailable and skip cleanup
			logger.Warnf("⚠️ Repository %s not available at %s, marking as unavailable", repo.ID, repo.Path)
			repo.Available = false
			continue
		}

		// Use git for-each-ref to list all refs/catnip/ references
		output, err := s.operations.ExecuteGit(repo.Path, "for-each-ref", "--format=%(refname)", "refs/catnip/")
		if err != nil {
			logger.Warnf("⚠️  Failed to list catnip refs for %s: %v", repo.ID, err)
			// Check if it's a directory access issue and mark repo as unavailable
			if strings.Contains(err.Error(), "cannot change to") || strings.Contains(err.Error(), "No such file or directory") {
				logger.Warnf("⚠️ Repository %s appears to be inaccessible, marking as unavailable", repo.ID)
				repo.Available = false
			}
			continue
		}

		if strings.TrimSpace(string(output)) == "" {
			continue // No catnip refs to clean up
		}

		deletedInRepo := 0
		refs := strings.Split(strings.TrimSpace(string(output)), "\n")

		for _, ref := range refs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}

			// Extract workspace name from ref (refs/catnip/workspace-name)
			refWorkspace := strings.TrimPrefix(ref, "refs/catnip/")

			// Check if this workspace is tracked in state.json
			if preservedWorkspaces[refWorkspace] {
				logger.Debugf("🔒 Preserving tracked ref: %s", ref)
				continue
			}

			// Double-check if there's an active worktree using this ref (fallback safety)
			worktrees, err := s.operations.ListWorktrees(repo.Path)
			if err == nil {
				var skipRef bool
				for _, wt := range worktrees {
					if wt.Branch == ref {
						skipRef = true
						logger.Debugf("🔒 Preserving ref with active worktree: %s", ref)
						break
					}
				}
				if skipRef {
					continue
				}
			}

			// Delete the orphaned ref using update-ref
			if _, err := s.operations.ExecuteGit(repo.Path, "update-ref", "-d", ref); err == nil {
				deletedInRepo++
				totalDeleted++
				logger.Debugf("🗑️  Deleted orphaned catnip ref: %s in %s", ref, repo.ID)

				// Also clean up the git config mapping for this ref
				configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(ref, "/", "."))
				if configErr := s.operations.UnsetConfig(repo.Path, configKey); configErr != nil {
					// Don't log as error since config might not exist - this is cleanup
					logger.Debugf("🧹 Config mapping %s didn't exist or was already clean", configKey)
				} else {
					logger.Debugf("🧹 Cleaned up config mapping: %s", configKey)
				}
			} else {
				logger.Warnf("⚠️  Failed to delete catnip ref %s: %v", ref, err)
			}
		}

		if deletedInRepo > 0 {
			logger.Infof("✅ Cleaned up %d orphaned catnip refs in %s", deletedInRepo, repo.ID)
			// Run garbage collection to clean up unreachable objects
			if err := s.operations.GarbageCollect(repo.Path); err != nil {
				logger.Warnf("⚠️ Failed to run garbage collection for %s: %v", repo.ID, err)
			}
		}
	}

	if totalDeleted > 0 {
		logger.Infof("🧹 Catnip refs cleanup complete: removed %d orphaned refs", totalDeleted)
	} else {
		logger.Debug("✅ No orphaned catnip refs found")
	}

	// Also clean up orphaned config mappings (even when no refs were deleted)
	s.cleanupOrphanedConfigMappings()
}

// CleanupAllCatnipRefs provides a comprehensive cleanup that handles both legacy catnip/ branches and new refs/catnip/ refs
func (s *GitService) CleanupAllCatnipRefs() {
	logger.Debug("🧹 Starting comprehensive catnip cleanup...")

	// Clean up legacy catnip/ branches first
	s.cleanupUnusedBranches()

	// Then clean up new refs/catnip/ namespace
	s.cleanupCatnipRefs()

	logger.Debug("✅ Comprehensive catnip cleanup complete")
}

// cleanupOrphanedConfigMappings removes git config mappings for refs that no longer exist
func (s *GitService) cleanupOrphanedConfigMappings() {
	logger.Debug("🧹 Starting cleanup of orphaned git config mappings...")

	s.mu.RLock()
	reposMap := s.stateManager.GetAllRepositories()
	s.mu.RUnlock()

	totalCleaned := 0

	for _, repo := range reposMap {
		// Skip unavailable repositories to prevent boot failures
		if !repo.Available {
			logger.Debugf("🔍 Skipping config cleanup for unavailable repository %s", repo.ID)
			continue
		}

		// Check if repository path exists before trying to clean it up
		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			// Mark repository as unavailable and skip cleanup
			logger.Warnf("⚠️ Repository %s not available at %s, marking as unavailable", repo.ID, repo.Path)
			repo.Available = false
			continue
		}

		// Get all existing refs/catnip/ refs
		existingRefs := make(map[string]bool)
		output, err := s.operations.ExecuteGit(repo.Path, "for-each-ref", "--format=%(refname)", "refs/catnip/")
		if err == nil && strings.TrimSpace(string(output)) != "" {
			refs := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, ref := range refs {
				ref = strings.TrimSpace(ref)
				if ref != "" {
					existingRefs[ref] = true
				}
			}
		}

		// Get all catnip.branch-map config entries
		configOutput, err := s.operations.ExecuteGit(repo.Path, "config", "--get-regexp", "catnip\\.branch-map\\.")
		if err != nil {
			continue // No config mappings or error
		}

		cleanedInRepo := 0
		lines := strings.Split(strings.TrimSpace(string(configOutput)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Parse config line: "catnip.branch-map.refs.catnip.name value"
			parts := strings.SplitN(line, " ", 2)
			if len(parts) != 2 {
				continue
			}

			configKey := parts[0]
			// Extract ref from config key: catnip.branch-map.refs.catnip.name -> refs/catnip/name
			refName := strings.ReplaceAll(strings.TrimPrefix(configKey, "catnip.branch-map."), ".", "/")

			// Check if this ref still exists
			if !existingRefs[refName] {
				// This config mapping is orphaned, remove it
				if err := s.operations.UnsetConfig(repo.Path, configKey); err != nil {
					logger.Debugf("⚠️ Failed to unset config %s: %v", configKey, err)
				} else {
					cleanedInRepo++
					totalCleaned++
					logger.Debugf("🧹 Cleaned up orphaned config mapping: %s", configKey)
				}
			}
		}

		if cleanedInRepo > 0 {
			logger.Infof("✅ Cleaned up %d orphaned config mappings in %s", cleanedInRepo, repo.ID)
		}
	}

	if totalCleaned > 0 {
		logger.Infof("🧹 Config mappings cleanup complete: removed %d orphaned mappings", totalCleaned)
	} else {
		logger.Debug("✅ No orphaned config mappings found")
	}
}

// SetupExecutor interface for executing setup.sh scripts in worktrees
type SetupExecutor interface {
	ExecuteSetupScript(worktreePath string)
}

// EventsEmitter interface for emitting worktree status events
type EventsEmitter interface {
	EmitWorktreeStatusUpdated(worktreeID string, status *CachedWorktreeStatus)
	EmitWorktreeBatchUpdated(updates map[string]*CachedWorktreeStatus)
	EmitWorktreeDirty(worktreeID, worktreeName string, files []string)
	EmitWorktreeClean(worktreeID, worktreeName string)
	EmitWorktreeUpdated(worktreeID string, updates map[string]interface{})
	EmitWorktreeCreated(worktree *models.Worktree)
	EmitWorktreeDeleted(worktreeID, worktreeName string)
	EmitWorktreeTodosUpdated(worktreeID string, todos []models.Todo)
	EmitSessionTitleUpdated(workspaceDir, worktreeID string, sessionTitle *models.TitleEntry, sessionTitleHistory []models.TitleEntry)
}

type GitService struct {
	stateManager        *WorktreeStateManager // Centralized state management
	operations          git.Operations        // All git operations through this interface
	gitWorktreeManager  *git.WorktreeManager  // Git layer worktree operations
	conflictResolver    *git.ConflictResolver // Handles conflict detection/resolution
	githubManager       *git.GitHubManager    // Handles all GitHub CLI operations
	localRepoManager    *LocalRepoManager     // Handles local repository detection
	commitSync          *CommitSyncService    // Handles automatic checkpointing and commit sync
	setupExecutor       SetupExecutor         // Handles setup.sh execution in PTY sessions
	worktreeCache       *WorktreeStatusCache  // Handles worktree status caching with event updates
	eventsEmitter       EventsEmitter         // Handles emitting events to connected clients
	claudeMonitor       *ClaudeMonitorService // Handles Claude session monitoring
	mu                  sync.RWMutex
	lastFetchTimes      map[string]time.Time // Track last fetch time per repo path
	lastFetchMu         sync.RWMutex         // Protect lastFetchTimes map
	fetchThrottlePeriod time.Duration        // How long to wait between fetches for same repo
}

// Helper functions for standardized command execution

// SetSetupExecutor sets the setup executor for executing setup.sh scripts
func (s *GitService) SetSetupExecutor(executor SetupExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setupExecutor = executor
}

// SetClaudeMonitor sets the claude monitor service
func (s *GitService) SetClaudeMonitor(monitor *ClaudeMonitorService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claudeMonitor = monitor
}

// SetEventsEmitter connects the events emitter to the state manager
func (s *GitService) SetEventsEmitter(emitter EventsEmitter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventsEmitter = emitter
	s.stateManager.SetEventsEmitter(emitter)
}

// SetSessionService connects the session service to enable Claude activity state tracking
func (s *GitService) SetSessionService(sessionService *SessionService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateManager.SetSessionService(sessionService)
}

// TriggerClaudeActivitySync triggers an immediate Claude activity state sync
func (s *GitService) TriggerClaudeActivitySync() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.stateManager.TriggerClaudeActivitySync()
}

// InitializeLocalRepos detects and loads any local repositories in /live
// This should be called after SetSetupExecutor to ensure setup.sh execution works
func (s *GitService) InitializeLocalRepos() {
	logger.Debug("🔍 Initializing local repositories with setup executor configured")
	s.detectLocalRepos()
}

// Repository type detection helpers
func (s *GitService) isLocalRepo(repoID string) bool {
	return strings.HasPrefix(repoID, "local/")
}

// Helper methods for command execution - using operations interface where possible
func (s *GitService) execCommand(command string, args ...string) *exec.Cmd {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+config.Runtime.HomeDir,
		"USER="+os.Getenv("USER"),
	)
	return cmd
}

func (s *GitService) runGitCommand(workingDir string, args ...string) ([]byte, error) {
	return s.operations.ExecuteGit(workingDir, args...)
}

// getSourceRef returns the appropriate source reference for a worktree
func (s *GitService) getSourceRef(worktree *models.Worktree) string {
	if s.isLocalRepo(worktree.RepoID) {
		// For local repos, use the local branch directly since it's the source of truth
		// The live remote can become stale and doesn't represent the current state
		return worktree.SourceBranch
	}

	// Check if origin remote exists and is valid in the worktree
	remotes, err := s.operations.GetRemotes(worktree.Path)
	if err != nil || remotes["origin"] == "" {
		// No valid origin remote, use local branch
		return worktree.SourceBranch
	}

	// Check if origin points to a temp directory (template repos)
	originURL := remotes["origin"]
	if strings.HasPrefix(originURL, "/tmp/template-") {
		// Template repo with invalid temp directory origin, use local branch
		return worktree.SourceBranch
	}

	return fmt.Sprintf("origin/%s", worktree.SourceBranch)
}

// Removed RemoteURLManager - functionality moved to git.URLManager

// PushStrategy defines the strategy for pushing branches (DEPRECATED: use git.PushStrategy)
type PushStrategy struct {
	Branch       string // Branch to push (defaults to worktree.Branch)
	Remote       string // Remote name (defaults to "origin")
	RemoteURL    string // Remote URL (optional, for local repos)
	SyncOnFail   bool   // Whether to sync with upstream on push failure
	SetUpstream  bool   // Whether to set upstream (-u flag)
	ConvertHTTPS bool   // Whether to convert SSH URLs to HTTPS
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

	// Execute push using operations
	err := s.operations.PushBranch(worktree.Path, gitStrategy)

	// Handle push failure with sync retry (if requested)
	if err != nil && strategy.SyncOnFail && git.IsPushRejected(err, err.Error()) {
		logger.Debug("🔄 Push rejected due to upstream changes, syncing and retrying")

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

// branchExists checks if a branch exists in a repository with configurable options
func (s *GitService) branchExists(repoPath, branch string, isRemote bool) bool {
	return s.operations.BranchExists(repoPath, branch, isRemote)
}

// getRemoteURL gets the remote URL for a repository
func (s *GitService) getRemoteURL(repoPath string) (string, error) {
	return s.operations.GetRemoteURL(repoPath)
}

// getDefaultBranch gets the default branch from a repository
func (s *GitService) getDefaultBranch(repoPath string) (string, error) {
	return s.operations.GetDefaultBranch(repoPath)
}

// fetchBranch unified fetch method with strategy pattern
func (s *GitService) fetchBranch(repoPath string, strategy git.FetchStrategy) error {
	return s.operations.FetchBranch(repoPath, strategy)
}

// NewGitService creates a new Git service instance
func NewGitService() *GitService {
	return NewGitServiceWithOperations(git.NewOperations())
}

// NewGitServiceWithOperations creates a new Git service instance with injectable git operations
func NewGitServiceWithOperations(operations git.Operations) *GitService {
	return NewGitServiceWithStateDir(operations, getGitStateDir())
}

// NewGitServiceWithStateDir creates a new Git service instance with custom state directory (for testing)
func NewGitServiceWithStateDir(operations git.Operations, stateDir string) *GitService {
	// Create state manager first (it will be connected to events handler later)
	stateManager := NewWorktreeStateManager(stateDir, nil)

	s := &GitService{
		stateManager:        stateManager,
		operations:          operations,
		gitWorktreeManager:  git.NewWorktreeManager(operations),
		conflictResolver:    git.NewConflictResolver(operations),
		githubManager:       git.NewGitHubManager(operations),
		localRepoManager:    NewLocalRepoManager(operations),
		lastFetchTimes:      make(map[string]time.Time),
		fetchThrottlePeriod: 5 * time.Second, // Throttle fetches to once per 5 seconds per repo
	}

	// Initialize CommitSync service
	s.commitSync = NewCommitSyncServiceWithOperations(s, operations)

	// Initialize worktree cache with state manager
	s.worktreeCache = NewWorktreeStatusCache(operations, stateManager)

	// Connect cache to worktree resolution using state manager
	s.worktreeCache.SetWorktreePathResolver(func(worktreeID string) (string, *models.Worktree) {
		worktree, exists := s.stateManager.GetWorktree(worktreeID)
		if !exists {
			return "", nil
		}
		return worktree.Path, worktree
	})

	// Ensure workspace directory exists
	_ = os.MkdirAll(getWorkspaceDir(), 0755)
	_ = os.MkdirAll(getGitStateDir(), 0755)

	// Configure Git to use gh as credential helper if available (containerized mode only)
	if config.Runtime.IsContainerized() {
		s.configureGitCredentials()
	} else {
		logger.Info("ℹ️ Running in native mode - respecting existing git configuration")
	}

	// State is already loaded by the state manager

	// Note: detectLocalRepos() will be called after setupExecutor is configured

	// Clean up unused catnip branches (skip in dev mode to avoid deleting active branches)
	if os.Getenv("CATNIP_DEV") != "true" {
		s.cleanupUnusedBranches()
	} else {
		logger.Debug("🔧 Skipping branch cleanup in dev mode")
	}

	// Always clean up orphaned catnip refs and config mappings (safe in both dev and prod)
	s.cleanupCatnipRefs()

	// Start CommitSync service for automatic checkpointing
	if err := s.commitSync.Start(); err != nil {
		logger.Warnf("⚠️ Failed to start CommitSync service: %v", err)
	}

	// Set up GitService as the WorktreeRestorer for state restoration
	stateManager.SetWorktreeRestorer(s)

	// Initialize and start PR sync manager
	prSyncManager := GetPRSyncManager(stateManager)
	prSyncManager.Start()

	return s
}

// Stop properly shuts down the git service and its components
func (s *GitService) Stop() {
	// Stop CommitSync service
	if s.commitSync != nil {
		s.commitSync.Stop()
	}

	// Stop worktree cache
	if s.worktreeCache != nil {
		s.worktreeCache.Stop()
	}

	// Stop state manager
	if s.stateManager != nil {
		s.stateManager.Stop()
	}

	// Stop PR sync manager
	if prSyncManager := GetPRSyncManager(nil); prSyncManager != nil {
		prSyncManager.Stop()
	}
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

	var repoURL string
	if os.Getenv("CATNIP_TEST_MODE") == "1" {
		// In test mode, use local test repositories
		repoURL = filepath.Join("/tmp", "test-repos", repo)
	} else {
		// In production, use GitHub URLs
		repoURL = fmt.Sprintf("https://github.com/%s/%s.git", org, repo)
	}

	repoName := strings.ReplaceAll(repo, "/", "-")
	reposDir := filepath.Join(config.Runtime.VolumeDir, "repos")

	// Ensure repos directory exists
	if err := os.MkdirAll(reposDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create repos directory: %v", err)
	}

	barePath := filepath.Join(reposDir, fmt.Sprintf("%s.git", repoName))

	// Check if a directory is already mounted at the repo location
	if s.isRepoMounted(getWorkspaceDir(), repoName) {
		return nil, nil, fmt.Errorf("a repository already exists at %s (possibly mounted)",
			filepath.Join(getWorkspaceDir(), repoName))
	}

	// Check if repository already exists in our map
	if existingRepo, exists := s.stateManager.GetRepository(repoID); exists {
		logger.Debugf("🔄 Repository already loaded, creating new worktree: %s", repoID)
		return s.createWorktreeForExistingRepo(existingRepo, branch)
	}

	// Check if bare repository already exists on disk
	if _, err := os.Stat(barePath); err == nil {
		logger.Debugf("🔄 Found existing bare repository, loading and creating new worktree: %s", repoID)
		return s.handleExistingRepository(repoID, repoURL, barePath, branch)
	}

	logger.Debugf("🔄 Cloning new repository: %s", repoID)
	return s.cloneNewRepository(repoID, repoURL, barePath, branch)
}

// isRepoMounted checks if a repo directory is already mounted
func (s *GitService) isRepoMounted(workspaceDir, repoName string) bool {
	potentialMountPath := filepath.Join(workspaceDir, repoName)
	if info, err := os.Stat(potentialMountPath); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(potentialMountPath, ".git")); err == nil {
			logger.Warnf("⚠️ Found existing Git repository at %s, skipping checkout", potentialMountPath)
			return true
		}
	}
	return false
}

// handleExistingRepository handles checkout when bare repo already exists
func (s *GitService) handleExistingRepository(repoID, repoURL, barePath, branch string) (*models.Repository, *models.Worktree, error) {
	// Load existing repository if we have state
	var repo *models.Repository
	if existingRepo, exists := s.stateManager.GetRepository(repoID); exists {
		logger.Debugf("📦 Repository already loaded: %s", repoID)
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
		if err := s.stateManager.AddRepository(repo); err != nil {
			logger.Warnf("⚠️ Failed to add repository to state: %v", err)
		}
	}

	// If no branch specified, use default
	if branch == "" {
		branch = repo.DefaultBranch
	}

	// Check if the requested branch exists in the bare repo
	if !s.branchExists(barePath, branch, true) {
		logger.Infof("🔄 Branch %s not found, fetching from remote", branch)
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

	// State persistence handled by state manager
	logger.Infof("✅ Worktree created from existing repository: %s", repoID)
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

	if err := s.stateManager.AddRepository(repository); err != nil {
		logger.Warnf("⚠️ Failed to add repository to state: %v", err)
	}

	// Start background unshallow process for the requested branch
	go s.unshallowRepository(barePath, branch)

	// Create initial worktree with fun name to avoid conflicts with local branches
	funName := s.generateUniqueSessionName(repository.Path)
	worktree, err := s.createWorktreeInternalForRepo(repository, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create initial worktree: %v", err)
	}

	// State persistence handled by state manager
	logger.Infof("✅ Repository cloned successfully: %s", repository.ID)
	return repository, worktree, nil
}

// ListWorktrees returns all worktrees with fast cache-enhanced responses
func (s *GitService) ListWorktrees() []*models.Worktree {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allWorktrees := s.stateManager.GetAllWorktrees()
	worktrees := make([]*models.Worktree, 0, len(allWorktrees))

	for _, wt := range allWorktrees {
		// Create a copy to avoid modifying the original
		worktreeCopy := *wt

		// Enhance with cached status (this is extremely fast - O(1) lookup)
		s.worktreeCache.EnhanceWorktreeWithCache(&worktreeCopy)

		// Enhance with PR state information if available
		s.enhanceWorktreeWithPRState(&worktreeCopy)

		worktrees = append(worktrees, &worktreeCopy)
	}

	return worktrees
}

// enhanceWorktreeWithPRState adds PR state information to a worktree if available
func (s *GitService) enhanceWorktreeWithPRState(wt *models.Worktree) {
	// Only enhance if the worktree has a PR URL
	if wt.PullRequestURL == "" {
		return
	}

	// Extract repo ID and PR number from the PR URL
	prPattern := regexp.MustCompile(`github\.com/([^/]+/[^/]+)/pull/(\d+)`)
	matches := prPattern.FindStringSubmatch(wt.PullRequestURL)
	if len(matches) != 3 {
		return
	}

	repoID := matches[1]
	prNumber, err := strconv.Atoi(matches[2])
	if err != nil {
		return
	}

	// Get PR state from the sync manager
	if prSyncManager := GetPRSyncManager(nil); prSyncManager != nil {
		if prState := prSyncManager.GetPRState(repoID, prNumber); prState != nil {
			wt.PullRequestState = prState.State
			wt.PullRequestLastSynced = &prState.LastSynced
		}
	}

	// Check if we have commits ahead of the remote branch (without fetching)
	// Note: checkHasCommitsAheadOfRemote does fetch, but that's okay for worktree enhancement
	// since we want accurate data. For the PR endpoint, we use it after fetching anyway.
	if ahead, err := s.checkHasCommitsAheadOfRemote(wt); err == nil {
		wt.HasCommitsAheadOfRemote = ahead
	}
}

// GetStatus returns the current Git status
func (s *GitService) GetStatus() *models.GitStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos := make(map[string]*models.Repository)
	for _, repo := range s.stateManager.GetAllRepositories() {
		repos[repo.ID] = repo
	}

	return &models.GitStatus{
		Repositories:  repos, // All repositories
		WorktreeCount: len(s.stateManager.GetAllWorktrees()),
	}
}

// UpdateWorktreeFields updates specific fields of a worktree
func (s *GitService) UpdateWorktreeFields(worktreeID string, updates map[string]interface{}) error {
	return s.stateManager.UpdateWorktree(worktreeID, updates)
}

// GetWorktree returns a worktree by ID
func (s *GitService) GetWorktree(worktreeID string) (*models.Worktree, bool) {
	return s.stateManager.GetWorktree(worktreeID)
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

// Snapshot-related code removed - change detection is now handled by WorktreeStateManager

// saveState and loadState methods removed - state persistence is now handled by WorktreeStateManager

// GetDefaultWorktreePath returns the path to the most recently accessed worktree
func (s *GitService) GetDefaultWorktreePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find most recently accessed worktree
	var mostRecentWorktree *models.Worktree
	for _, wt := range s.stateManager.GetAllWorktrees() {
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
	if err := s.githubManager.ConfigureGitCredentials(); err != nil {
		logger.Warnf("❌ Failed to configure Git credential helper: %v", err)
	} else {
		logger.Infof("✅ Git credential helper configured successfully")
	}
}

// ListGitHubRepositories returns a list of GitHub repositories accessible to the user
func (s *GitService) ListGitHubRepositories() ([]map[string]interface{}, error) {
	var repos []map[string]interface{}

	// Add all local repositories
	s.mu.RLock()
	for _, repo := range s.stateManager.GetAllRepositories() {
		repoID := repo.ID
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
	githubRepos, err := s.githubManager.ListRepositories()
	if err != nil {
		// If GitHub CLI fails, still return dev repo if it exists
		if len(repos) > 0 {
			return repos, nil
		}
		return nil, fmt.Errorf("failed to list GitHub repositories: %w", err)
	}

	// Transform the GitHub data to match frontend expectations
	for _, repo := range githubRepos {
		repoMap := map[string]interface{}{
			"name":        repo.Name,
			"url":         repo.URL,
			"private":     repo.IsPrivate,
			"description": repo.Description,
		}

		// Add full name for display
		if login, ok := repo.Owner["login"].(string); ok {
			repoMap["fullName"] = fmt.Sprintf("%s/%s", login, repo.Name)
		}

		repos = append(repos, repoMap)
	}

	return repos, nil
}

// detectLocalRepos delegates to LocalRepoManager for detecting local repositories
func (s *GitService) detectLocalRepos() {
	repos := s.localRepoManager.DetectLocalRepos()

	// Add detected repos to our repository map via state manager
	for repoID, repo := range repos {
		// Check if repository already exists in state and update fields if needed
		if existingRepo, exists := s.stateManager.GetRepository(repoID); exists {
			// Always update these fields from fresh detection
			existingRepo.DefaultBranch = repo.DefaultBranch
			existingRepo.LastAccessed = repo.LastAccessed
			existingRepo.HasGitHubRemote = repo.HasGitHubRemote
			existingRepo.RemoteOrigin = repo.RemoteOrigin

			// Log if GitHub remote detection changed
			if existingRepo.HasGitHubRemote != repo.HasGitHubRemote {
				logger.Infof("🔄 Updating GitHub remote status for %s: %v -> %v", repoID, existingRepo.HasGitHubRemote, repo.HasGitHubRemote)
			}

			repo = existingRepo // Use the existing repo with updated fields
		}

		if err := s.stateManager.AddRepository(repo); err != nil {
			logger.Warnf("⚠️ Failed to add repository %s to state: %v", repoID, err)
			continue
		}

		// Check if any worktrees exist for this repo
		if s.shouldCreateInitialWorktree(repoID) {
			logger.Infof("🌱 Creating initial worktree for %s", repoID)

			// For shallow clones or when on a non-default branch, we need to ensure
			// the default branch is fetched before we can create a worktree from it
			defaultBranch := repo.DefaultBranch

			// Check if the default branch exists locally
			if !s.branchExists(repo.Path, defaultBranch, false) {
				logger.Infof("📥 Default branch '%s' not found locally, fetching from origin...", defaultBranch)

				// Fetch the default branch in the background
				// Use FetchBranchFast for speed (shallow fetch with depth=1)
				// FetchBranchFast will also create the local branch ref from the remote tracking branch
				if err := s.operations.FetchBranchFast(repo.Path, defaultBranch); err != nil {
					logger.Warnf("⚠️  Failed to fetch default branch '%s': %v", defaultBranch, err)
					logger.Infof("🔄 Attempting to determine and fetch the correct default branch from remote...")

					// Try to get the actual default branch from the remote
					if remoteBranch, err := s.operations.GetRemoteDefaultBranch(repo.Path); err == nil && remoteBranch != "" {
						defaultBranch = remoteBranch
						logger.Infof("🔍 Remote default branch detected: %s", defaultBranch)

						// Try fetching the actual default branch
						if err := s.operations.FetchBranchFast(repo.Path, defaultBranch); err != nil {
							logger.Warnf("⚠️  Failed to fetch remote default branch '%s': %v", defaultBranch, err)
						} else {
							logger.Infof("✅ Successfully fetched default branch '%s'", defaultBranch)
							// Update the repository's default branch if it was detected differently
							repo.DefaultBranch = defaultBranch
							if err := s.stateManager.AddRepository(repo); err != nil {
								logger.Warnf("⚠️ Failed to update repository default branch in state: %v", err)
							}
						}
					}
				} else {
					logger.Infof("✅ Successfully fetched default branch '%s'", defaultBranch)
				}

				// If the local branch still doesn't exist (even after fetch), fall back to using any available local branch
				// Note: A successful fetch might only update the remote tracking branch without creating the local branch
				if !s.branchExists(repo.Path, defaultBranch, false) {
					logger.Warnf("⚠️  Default branch '%s' not available locally, checking for available local branches...", defaultBranch)

					// Get list of local branches
					if localBranches, err := s.operations.GetLocalBranches(repo.Path); err == nil && len(localBranches) > 0 {
						// Try common branch names in order
						commonBranches := []string{"main", "master", "develop"}
						fallbackBranch := ""

						for _, commonBranch := range commonBranches {
							for _, localBranch := range localBranches {
								if localBranch == commonBranch {
									fallbackBranch = commonBranch
									break
								}
							}
							if fallbackBranch != "" {
								break
							}
						}

						// If no common branch found, use the first available branch
						if fallbackBranch == "" {
							fallbackBranch = localBranches[0]
						}

						logger.Warnf("⚠️  Using fallback branch '%s' instead of configured default '%s'", fallbackBranch, defaultBranch)
						defaultBranch = fallbackBranch

						// Update the repository's default branch
						repo.DefaultBranch = defaultBranch
						if err := s.stateManager.AddRepository(repo); err != nil {
							logger.Warnf("⚠️ Failed to update repository default branch in state: %v", err)
						}
					} else {
						logger.Warnf("⚠️  No local branches found, will attempt to create worktree anyway")
					}
				}
			}

			// Don't proactively prune during runtime - it can delete workspaces being restored
			// Pruning should only happen on explicit user request or during shutdown
			// if pruneErr := s.operations.PruneWorktrees(repo.Path); pruneErr != nil {
			// 	logger.Warnf("⚠️  Failed to prune worktrees for %s: %v", repoID, pruneErr)
			// }

			if _, worktree, err := s.handleLocalRepoWorktree(repoID, defaultBranch); err != nil {
				logger.Warnf("❌ Failed to create initial worktree for %s: %v", repoID, err)
			} else {
				logger.Infof("✅ Initial worktree created: %s", worktree.Name)
			}
		}
	}

	// Check and update any stale catnip-live remotes in existing worktrees
	s.updateStaleRemotes()
}

// updateStaleRemotes checks all existing worktrees for stale catnip-live remotes and updates them
func (s *GitService) updateStaleRemotes() {
	logger.Debug("🔍 Checking for stale catnip-live remotes in existing worktrees...")

	allWorktrees := s.stateManager.GetAllWorktrees()
	for _, worktree := range allWorktrees {
		// Only check local repo worktrees
		if !s.isLocalRepo(worktree.RepoID) {
			continue
		}

		// Get the repository for this worktree
		repo, exists := s.stateManager.GetRepository(worktree.RepoID)
		if !exists {
			continue
		}

		// Check if worktree path exists
		if _, err := os.Stat(worktree.Path); os.IsNotExist(err) {
			continue
		}

		// Get existing remotes
		remotes, err := s.operations.GetRemotes(worktree.Path)
		if err != nil {
			continue
		}

		// Check if catnip-live remote exists and points to correct path
		if existingURL, exists := remotes["catnip-live"]; exists {
			if existingURL != repo.Path {
				logger.Infof("🔄 Updating stale 'catnip-live' remote in %s from %s to %s",
					worktree.Name, existingURL, repo.Path)
				if err := s.operations.SetRemoteURL(worktree.Path, "catnip-live", repo.Path); err != nil {
					logger.Warnf("⚠️ Failed to update catnip-live remote in %s: %v", worktree.Name, err)
				} else {
					logger.Infof("✅ Updated 'catnip-live' remote in %s", worktree.Name)
				}
			}
		}
	}
}

// UpdateAllStaleRemotes is a public method that can be called to manually check and update all stale catnip-live remotes
func (s *GitService) UpdateAllStaleRemotes() {
	s.mu.Lock()
	defer s.mu.Unlock()
	logger.Info("🔄 Manually checking and updating all stale catnip-live remotes...")
	s.updateStaleRemotes()
	logger.Info("✅ Manual stale remote update completed")
}

// shouldCreateInitialWorktree checks if we should create an initial worktree for a repo
func (s *GitService) shouldCreateInitialWorktree(repoID string) bool {
	// First check if worktrees exist in state manager (for restore scenario)
	allWorktrees := s.stateManager.GetAllWorktrees()
	for _, worktree := range allWorktrees {
		if worktree.RepoID == repoID {
			logger.Debugf("🔍 Found existing worktree in state for %s: %s", repoID, worktree.Name)
			return false
		}
	}

	// Check if any worktrees exist for this repo in /workspace
	dirName := filepath.Base(strings.TrimPrefix(repoID, "local/"))
	repoWorkspaceDir := filepath.Join(getWorkspaceDir(), dirName)

	// Check if the repo workspace directory exists and has any worktrees
	if entries, err := os.ReadDir(repoWorkspaceDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				// Check if this directory is a valid git worktree
				if _, err := os.Stat(filepath.Join(repoWorkspaceDir, entry.Name(), ".git")); err == nil {
					logger.Debugf("🔍 Found existing worktree for %s: %s", repoID, entry.Name())
					return false
				}
			}
		}
	}

	logger.Debugf("🔍 No existing worktrees found for %s, will create initial worktree", repoID)
	return true
}

// getLocalRepoDefaultBranch delegates to git helper for determining the actual default branch
func (s *GitService) getLocalRepoDefaultBranch(repoPath string) string {
	// Use the shared git helper function to determine the default branch
	// This ensures consistent logic across the codebase
	defaultBranch := git.GetDefaultBranch(s.operations, repoPath)
	logger.Debugf("🔍 Determined default branch for %s: %s", repoPath, defaultBranch)
	return defaultBranch
}

// handleLocalRepoWorktree creates a worktree for any local repo
func (s *GitService) handleLocalRepoWorktree(repoID, branch string) (*models.Repository, *models.Worktree, error) {
	// Get the local repo from repositories map
	localRepo, exists := s.stateManager.GetRepository(repoID)
	if !exists {
		return nil, nil, fmt.Errorf("local repository %s not found - it may not be mounted", repoID)
	}

	// If no branch specified, use repository's default branch
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
	// State persistence handled by state manager

	logger.Infof("✅ Local repo worktree created: %s from branch %s", worktree.Name, worktree.SourceBranch)
	return localRepo, worktree, nil
}

// createLocalRepoWorktree creates a worktree for any local repo
func (s *GitService) createLocalRepoWorktree(repo *models.Repository, branch, name string) (*models.Worktree, error) {
	// Use git WorktreeManager to create the local worktree
	worktree, err := s.gitWorktreeManager.CreateLocalWorktree(git.CreateWorktreeRequest{
		Repository:   repo,
		SourceBranch: branch,
		BranchName:   name,
		WorkspaceDir: getWorkspaceDir(),
	})
	if err != nil {
		return nil, err
	}

	// Store worktree in service map
	if err := s.stateManager.AddWorktree(worktree); err != nil {
		logger.Warnf("⚠️ Failed to add worktree to state: %v", err)
	}

	// Notify ClaudeMonitor service about the new worktree
	if s.claudeMonitor != nil {
		s.claudeMonitor.OnWorktreeCreated(worktree.ID, worktree.Path)
	}

	// Update current symlink to point to this worktree if it's the first one
	if len(s.stateManager.GetAllWorktrees()) == 1 {
		_ = s.updateCurrentSymlink(worktree.Path)
	}

	// Execute setup.sh if it exists in the newly created worktree
	if s.setupExecutor != nil {
		logger.Infof("🚀 Scheduling setup.sh execution for local worktree: %s", worktree.Path)
		// Run setup.sh execution in a goroutine to avoid blocking worktree creation
		recovery.SafeGo("setup-script-local-"+worktree.Path, func() {
			// Wait a moment to ensure the worktree is fully ready
			time.Sleep(2 * time.Second)
			logger.Infof("⏰ Starting setup.sh execution for local worktree: %s", worktree.Path)
			s.setupExecutor.ExecuteSetupScript(worktree.Path)
		})
	} else {
		logger.Warnf("⚠️ No setup executor configured, skipping setup.sh execution for local worktree: %s", worktree.Path)
	}

	return worktree, nil
}

// getLocalRepoBranches returns the local branches for a local repository
func (s *GitService) getLocalRepoBranches(repoPath string) ([]string, error) {
	return s.operations.GetLocalBranches(repoPath)
}

// GetRepositoryBranches returns the remote branches for a repository
func (s *GitService) GetRepositoryBranches(repoID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.stateManager.GetRepository(repoID)
	if !exists {
		// For remote GitHub repos that haven't been checked out yet,
		// we can still fetch branches using git ls-remote
		if !strings.HasPrefix(repoID, "local/") {
			// Convert repoID (e.g., "vanpelt/vllmulator") to GitHub URL
			remoteURL := fmt.Sprintf("https://github.com/%s.git", repoID)
			return s.operations.GetRemoteBranchesFromURL(remoteURL)
		}
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	// Check if repository is available for local repos only
	// Remote repos can still be queried even if not locally available
	if s.isLocalRepo(repoID) && !repo.Available {
		return nil, fmt.Errorf("repository %s is not available", repoID)
	}

	// Handle local repos specially - only use local branches to avoid network complexity
	if s.isLocalRepo(repoID) {
		// Get local branches only - no remote fetching to avoid network issues
		localBranches, err := s.operations.GetLocalBranches(repo.Path)
		if err != nil {
			logger.Warnf("Warning: failed to get local branches for %s: %v", repoID, err)
			// Fallback to default branch if we have it
			if repo.DefaultBranch != "" {
				return []string{repo.DefaultBranch}, nil
			}
			return []string{"main"}, nil // final fallback
		}

		// Return local branches if we have them
		if len(localBranches) > 0 {
			return localBranches, nil
		}

		// No local branches found - return sensible fallback
		logger.Warnf("Warning: no local branches found for %s", repoID)
		if repo.DefaultBranch != "" {
			return []string{repo.DefaultBranch}, nil
		}
		return []string{"main"}, nil // final fallback
	}

	// For remote repos, use the operations interface
	return s.operations.GetRemoteBranches(repo.Path, repo.DefaultBranch)
}

// DeleteWorktree removes a worktree and returns a channel that signals when cleanup is complete
// Callers can ignore the channel for async behavior, or wait on it for sync behavior
func (s *GitService) DeleteWorktree(worktreeID string) (<-chan error, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Get repository for worktree deletion
	repo, exists := s.stateManager.GetRepository(worktree.RepoID)
	if !exists {
		return nil, fmt.Errorf("repository %s not found", worktree.RepoID)
	}

	// SAFETY CHECK: Refuse to delete worktrees outside our managed workspace directory
	// This protects against accidentally deleting external repository paths
	// Exception: Allow deletion during tests (temp directories on Linux/macOS)
	workspaceDir := config.Runtime.WorkspaceDir
	isTestPath := s.isTemporaryPath(worktree.Path)
	if workspaceDir != "" && !strings.HasPrefix(worktree.Path, workspaceDir+"/") && !isTestPath {
		return nil, fmt.Errorf("cannot delete worktree %s: path %s is outside managed workspace directory %s", worktree.Name, worktree.Path, workspaceDir)
	}

	// Clean up any active PTY sessions for this worktree (service-specific)
	s.cleanupActiveSessions(worktree.Path)

	// Remove from cache immediately (for fast UI response)
	s.worktreeCache.RemoveWorktree(worktreeID, worktree.Path)

	// Remove from service memory immediately
	if err := s.stateManager.DeleteWorktree(worktreeID); err != nil {
		logger.Warnf("⚠️ Failed to delete worktree from state: %v", err)
	}

	// Notify Claude monitor service to clean up checkpoint managers and todo monitors immediately
	if s.claudeMonitor != nil {
		s.claudeMonitor.OnWorktreeDeleted(worktreeID, worktree.Path)
	}

	// Create a channel to signal completion
	done := make(chan error, 1)

	// For test environments, run cleanup synchronously to avoid hanging in CI
	// Note: isTestPath was already declared above for the safety check
	if isTestPath {
		logger.Debugf("🧪 Running synchronous cleanup for test worktree %s", worktree.Name)
		cleanupStart := time.Now()

		if err := s.gitWorktreeManager.DeleteWorktree(worktree, repo); err != nil {
			logger.Warnf("⚠️ Synchronous git cleanup failed for worktree %s: %v", worktree.Name, err)
			done <- err
		} else {
			cleanupDuration := time.Since(cleanupStart)
			logger.Debugf("✅ Synchronous git cleanup completed for worktree %s in %v", worktree.Name, cleanupDuration)
			done <- nil
		}
		close(done)
	} else {
		// For production, perform comprehensive git cleanup in background (non-blocking)
		go func() {
			logger.Debugf("🗑️ Starting background git cleanup for worktree %s", worktree.Name)
			cleanupStart := time.Now()

			if err := s.gitWorktreeManager.DeleteWorktree(worktree, repo); err != nil {
				logger.Warnf("⚠️ Background git cleanup failed for worktree %s: %v", worktree.Name, err)
				done <- err
			} else {
				cleanupDuration := time.Since(cleanupStart)
				logger.Debugf("✅ Background git cleanup completed for worktree %s in %v", worktree.Name, cleanupDuration)
				done <- nil
			}
			close(done)
		}()
	}

	// Save state
	// State persistence handled by state manager

	return done, nil
}

// UpdateWorktreeBranchName updates the stored branch name for a worktree after a git branch rename
func (s *GitService) UpdateWorktreeBranchName(worktreePath, newBranchName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find worktree by path
	var targetWorktree *models.Worktree
	for _, worktree := range s.stateManager.GetAllWorktrees() {
		if worktree.Path == worktreePath {
			targetWorktree = worktree
			break
		}
	}

	if targetWorktree == nil {
		return fmt.Errorf("worktree not found for path: %s", worktreePath)
	}

	// Update the branch name
	oldBranchName := targetWorktree.Branch

	// Update via state manager to ensure persistence and event emission
	updates := map[string]interface{}{
		"branch": newBranchName,
	}

	if err := s.stateManager.UpdateWorktree(targetWorktree.ID, updates); err != nil {
		return fmt.Errorf("failed to update worktree branch: %v", err)
	}

	logger.Infof("✅ Updated worktree %s branch name: %s -> %s", targetWorktree.Name, oldBranchName, newBranchName)

	return nil
}

// CleanupMergedWorktrees removes worktrees that have been fully merged into their source branch
func (s *GitService) CleanupMergedWorktrees() (int, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var cleanedUp []string
	var errors []error

	logger.Infof("🧹 Starting cleanup of merged worktrees, checking %d worktrees", len(s.stateManager.GetAllWorktrees()))

	for _, worktree := range s.stateManager.GetAllWorktrees() {
		logger.Debugf("🔍 Checking worktree %s: dirty=%v, conflicts=%v, commits_ahead=%d, source=%s",
			worktree.Name, worktree.IsDirty, worktree.HasConflicts, worktree.CommitCount, worktree.SourceBranch)

		// Skip if worktree has uncommitted changes or conflicts
		if worktree.IsDirty {
			logger.Warnf("⏭️  Skipping cleanup of dirty worktree: %s", worktree.Name)
			continue
		}
		if worktree.HasConflicts {
			logger.Warnf("⏭️  Skipping cleanup of conflicted worktree: %s", worktree.Name)
			continue
		}

		// Skip if worktree has commits ahead of source
		if worktree.CommitCount > 0 {
			logger.Warnf("⏭️  Skipping cleanup of worktree with %d commits ahead: %s", worktree.CommitCount, worktree.Name)
			continue
		}

		// Check if the worktree branch exists in the source repo
		repo, exists := s.stateManager.GetRepository(worktree.RepoID)
		if !exists {
			continue
		}

		// For local repos, check if the worktree branch no longer exists or if it matches the source branch
		isLocal := s.isLocalRepo(worktree.RepoID)
		var isMerged bool

		if isLocal {
			logger.Debugf("🔍 Checking local worktree %s: branch=%s, source=%s", worktree.Name, worktree.Branch, worktree.SourceBranch)

			// For local repos, check if the branch exists in the main repo
			// If it doesn't exist, it was likely deleted after merge
			branchExists := s.operations.BranchExists(repo.Path, worktree.Branch, false)

			if !branchExists {
				logger.Infof("✅ Branch %s no longer exists in main repo (likely merged and deleted)", worktree.Branch)
				isMerged = true
			} else {
				// Branch still exists, check if it's merged
				branches, err := s.operations.ListBranches(repo.Path, git.ListBranchesOptions{Merged: worktree.SourceBranch})
				if err != nil {
					logger.Warnf("⚠️ Failed to check merged status for %s: %v", worktree.Name, err)
					continue
				}

				for _, branch := range branches {
					branch = git.CleanBranchName(branch)
					if branch == worktree.Branch {
						isMerged = true
						logger.Infof("✅ Found %s in merged branches list", worktree.Branch)
						break
					}
				}
			}
		} else {
			// Regular repo logic (existing code)
			logger.Debugf("🔍 Checking if branch %s is merged into %s in repo %s", worktree.Branch, worktree.SourceBranch, repo.Path)
			branches, err := s.operations.ListBranches(repo.Path, git.ListBranchesOptions{Merged: worktree.SourceBranch})
			if err != nil {
				logger.Warnf("⚠️ Failed to check merged status for %s: %v", worktree.Name, err)
				continue
			}

			// Check if our branch appears in the merged branches list
			logger.Infof("📋 Merged branches into %s: %d branches found", worktree.SourceBranch, len(branches))

			for _, branch := range branches {
				// Handle both regular branches and worktree branches (marked with +)
				branch = git.CleanBranchName(branch)
				if branch == worktree.Branch {
					isMerged = true
					logger.Infof("✅ Found %s in merged branches list", worktree.Branch)
					break
				}
			}
		}

		if !isMerged {
			logger.Debugf("❌ Branch %s not eligible for cleanup", worktree.Branch)
		}

		if isMerged {
			logger.Infof("🧹 Found merged worktree to cleanup: %s", worktree.Name)

			// Use the existing deletion logic but don't hold the mutex
			s.mu.Unlock()
			if done, cleanupErr := s.DeleteWorktree(worktree.ID); cleanupErr != nil {
				errors = append(errors, fmt.Errorf("failed to cleanup worktree %s: %v", worktree.Name, cleanupErr))
			} else {
				// Wait for cleanup to complete
				if waitErr := <-done; waitErr != nil {
					errors = append(errors, fmt.Errorf("failed to complete cleanup for worktree %s: %v", worktree.Name, waitErr))
				} else {
					cleanedUp = append(cleanedUp, worktree.Name)
				}
			}
			s.mu.Lock()
		}
	}

	if len(cleanedUp) > 0 {
		logger.Infof("✅ Cleaned up %d merged worktrees: %s", len(cleanedUp), strings.Join(cleanedUp, ", "))
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
		logger.Infof("ℹ️ No active processes found for worktree path: %s", worktreePath)
	} else {
		logger.Infof("✅ Terminated processes for worktree: %s", worktreePath)
	}

	// Also try to cleanup any session directories that might exist
	// Session IDs are typically derived from worktree names
	workspaceDir := getWorkspaceDir()
	parts := strings.Split(strings.TrimPrefix(worktreePath, workspaceDir+"/"), "/")
	if len(parts) >= 2 {
		sessionID := fmt.Sprintf("%s/%s", parts[0], parts[1])
		sessionWorkDir := filepath.Join(workspaceDir, sessionID)

		// If there's a session directory different from the worktree, clean it up too
		if sessionWorkDir != worktreePath {
			if _, err := os.Stat(sessionWorkDir); err == nil {
				if removeErr := os.RemoveAll(sessionWorkDir); removeErr != nil {
					logger.Warnf("⚠️ Failed to remove session directory %s: %v", sessionWorkDir, removeErr)
				} else {
					logger.Infof("✅ Removed session directory: %s", sessionWorkDir)
				}
			}
		}
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
		// Local repos: No fetching needed since worktrees share the same .git repository
		// The source branch is already available locally
		return
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
	return s.operations.FetchBranchFast(repoPath, branch)
}

// fetchBranchFull performs a full fetch for operations that need complete history
func (s *GitService) fetchBranchFull(repoPath, branch string) error {
	return s.operations.FetchBranchFull(repoPath, branch)
}

// These fetchLocalBranch functions have been removed as they used the deprecated "live" remote approach.
// Local repos now work directly with the shared git repository without needing separate remotes.

// SyncWorktree syncs a worktree with its source branch
func (s *GitService) SyncWorktree(worktreeID string, strategy string) error {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
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
	getSourceRef := func(w *models.Worktree) string {
		if s.isLocalRepo(w.RepoID) {
			return w.SourceBranch // Local repos use branch directly
		} else {
			return fmt.Sprintf("origin/%s", w.SourceBranch) // Remote repos use origin prefix
		}
	}
	s.gitWorktreeManager.UpdateWorktreeStatus(worktree, getSourceRef)

	logger.Infof("✅ Synced worktree %s with %s strategy", worktree.Name, strategy)
	return nil
}

// applySyncStrategy applies merge or rebase strategy
func (s *GitService) applySyncStrategy(worktree *models.Worktree, strategy, sourceRef string) error {
	var err error

	switch strategy {
	case "merge":
		err = s.operations.Merge(worktree.Path, sourceRef)
	case "rebase":
		err = s.operations.Rebase(worktree.Path, sourceRef)
	default:
		return fmt.Errorf("unknown sync strategy: %s", strategy)
	}

	if err != nil {
		// Check if this is an uncommitted changes error (not a conflict)
		if s.isUncommittedChangesError(err.Error()) {
			return fmt.Errorf("cannot %s: worktree has staged changes. Please commit or unstage your changes first", strategy)
		}

		// Check if this is a merge conflict
		if s.isMergeConflict(worktree.Path, err.Error()) {
			return s.createMergeConflictError("sync", worktree, err.Error())
		}
		return fmt.Errorf("failed to %s: %v", strategy, err)
	}

	return nil
}

// MergeWorktreeToMain merges a local repo worktree's changes back to the main repository
func (s *GitService) MergeWorktreeToMain(worktreeID string, squash bool) error {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Only works for local repos
	if !s.isLocalRepo(worktree.RepoID) {
		return fmt.Errorf("merge to main only supported for local repositories")
	}

	// Get the local repo
	repo, exists := s.stateManager.GetRepository(worktree.RepoID)
	if !exists {
		return fmt.Errorf("local repository %s not found", worktree.RepoID)
	}

	logger.Infof("🔄 Merging worktree %s back to main repository", worktree.Name)

	// Ensure we have full history for merge operations
	s.fetchFullHistory(worktree)

	// First, push the worktree branch to the main repo
	output, err := s.runGitCommand(worktree.Path, "push", repo.Path, fmt.Sprintf("%s:%s", worktree.Branch, worktree.Branch))
	if err != nil {
		return fmt.Errorf("failed to push worktree branch to main repo: %v\n%s", err, output)
	}

	// Switch to the source branch in main repo and merge
	output, err = s.runGitCommand(repo.Path, "checkout", worktree.SourceBranch)
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
	output, err = s.runGitCommand(repo.Path, mergeArgs...)
	if err != nil {
		// Check if this is a merge conflict
		if s.isMergeConflict(repo.Path, string(output)) {
			return s.createMergeConflictError("merge", worktree, string(output))
		}
		return fmt.Errorf("failed to merge worktree branch: %v\n%s", err, output)
	}

	// For squash merges, we need to commit the staged changes
	if squash {
		_, err = s.runGitCommitWithGPGFallback(repo.Path, "commit", "-m", fmt.Sprintf("Squash merge branch '%s' from worktree", worktree.Branch))
		if err != nil {
			return fmt.Errorf("failed to commit squash merge: %v", err)
		}
	}

	// Delete the feature branch from main repo (cleanup)
	_ = s.operations.DeleteBranch(repo.Path, worktree.Branch, false) // Ignore errors - branch might be in use

	// Get the new commit hash from the main branch after merge
	if newCommitHash, err := s.operations.GetCommitHash(repo.Path, "HEAD"); err != nil {
		logger.Warnf("⚠️  Failed to get new commit hash after merge: %v", err)
	} else {
		// Update the worktree's commit hash to the new merge point
		s.mu.Lock()
		worktree.CommitHash = newCommitHash
		s.mu.Unlock()
		logger.Warnf("📝 Updated worktree %s CommitHash to %s", worktree.Name, newCommitHash)
	}

	logger.Infof("✅ Merged worktree %s to main repository", worktree.Name)
	return nil
}

// CreateWorktreePreview creates a preview branch in the main repo for viewing changes outside container
func (s *GitService) CreateWorktreePreview(worktreeID string) error {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Only works for local repos
	if !s.isLocalRepo(worktree.RepoID) {
		return fmt.Errorf("preview only supported for local repositories")
	}

	// Get the local repo
	repo, exists := s.stateManager.GetRepository(worktree.RepoID)
	if !exists {
		return fmt.Errorf("local repository %s not found", worktree.RepoID)
	}

	// Check if repository is available
	if !repo.Available {
		return fmt.Errorf("repository %s is not available", worktree.RepoID)
	}

	previewBranchName := fmt.Sprintf("catnip/%s", git.ExtractWorkspaceName(worktree.Branch))
	logger.Debugf("🔍 Creating preview branch %s for worktree %s", previewBranchName, worktree.Name)

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
				_, _ = s.runGitCommand(worktree.Path, "reset", "--mixed", "HEAD~1")
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
		logger.Infof("🔄 Updating existing preview branch %s", previewBranchName)
	}
	pushArgs = append(pushArgs, repo.Path, fmt.Sprintf("%s:refs/heads/%s", worktree.Branch, previewBranchName))

	output, err := s.runGitCommand(worktree.Path, pushArgs...)
	if err != nil {
		return fmt.Errorf("failed to create preview branch: %v\n%s", err, output)
	}

	action := "created"
	if shouldForceUpdate {
		action = "updated"
	}

	if hasUncommittedChanges {
		logger.Infof("✅ Preview branch %s %s with uncommitted changes - you can now checkout this branch outside the container", previewBranchName, action)
	} else {
		logger.Infof("✅ Preview branch %s %s - you can now checkout this branch outside the container", previewBranchName, action)
	}
	return nil
}

// shouldForceUpdatePreviewBranch determines if we should force-update an existing preview branch
func (s *GitService) shouldForceUpdatePreviewBranch(repoPath, previewBranchName string) (bool, error) {
	// Check if the preview branch exists
	if _, err := s.runGitCommand(repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", previewBranchName)); err != nil {
		// Branch doesn't exist, safe to create
		return false, nil
	}

	// Branch exists - always force update preview branches since they should reflect latest worktree state
	output, err := s.runGitCommand(repoPath, "log", "-1", "--pretty=format:%s", previewBranchName)
	if err != nil {
		return false, fmt.Errorf("failed to get last commit message: %v", err)
	}

	lastCommitMessage := strings.TrimSpace(string(output))
	logger.Infof("🔄 Found existing preview branch %s with commit: '%s' - will force update", previewBranchName, lastCommitMessage)
	return true, nil
}

// hasUncommittedChanges checks if the worktree has any uncommitted changes
func (s *GitService) hasUncommittedChanges(worktreePath string) (bool, error) {
	return s.operations.HasUncommittedChanges(worktreePath)
}

// createTemporaryCommit creates a temporary commit with all uncommitted changes
func (s *GitService) createTemporaryCommit(worktreePath string) (string, error) {
	// Add all changes (staged, unstaged, and untracked)
	if output, err := s.runGitCommand(worktreePath, "add", "."); err != nil {
		return "", fmt.Errorf("failed to stage changes: %v\n%s", err, output)
	}

	// Create the commit
	if _, err := s.runGitCommitWithGPGFallback(worktreePath, "commit", "-m", "Preview: Include all uncommitted changes"); err != nil {
		return "", fmt.Errorf("failed to create temporary commit: %v", err)
	}

	// Get the commit hash
	commitHash, err := s.operations.GetCommitHash(worktreePath, "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %v", err)
	}
	logger.Warnf("📝 Created temporary commit %s with uncommitted changes", commitHash[:8])
	return commitHash, nil
}

// revertTemporaryCommit reverts a temporary commit by resetting HEAD~1
func (s *GitService) revertTemporaryCommit(worktreePath, commitHash string) {
	if commitHash != "" {
		_ = s.operations.ResetMixed(worktreePath, "HEAD~1")
	}
}

// isMergeConflict checks if the git command output indicates a merge conflict
func (s *GitService) isMergeConflict(repoPath, output string) bool {
	return s.conflictResolver.IsMergeConflict(repoPath, output)
}

// isUncommittedChangesError checks if the error is due to staged/uncommitted changes
func (s *GitService) isUncommittedChangesError(output string) bool {
	uncommittedIndicators := []string{
		"Your index contains uncommitted changes",
		"cannot rebase: Your index contains uncommitted changes",
		"Please commit or stash them",
	}

	for _, indicator := range uncommittedIndicators {
		if strings.Contains(output, indicator) {
			return true
		}
	}
	return false
}

// createMergeConflictError creates a detailed merge conflict error
func (s *GitService) createMergeConflictError(operation string, worktree *models.Worktree, output string) *models.MergeConflictError {
	return s.conflictResolver.CreateMergeConflictError(operation, worktree.Name, worktree.Path, output)
}

// CheckSyncConflicts checks if syncing a worktree would cause merge conflicts
func (s *GitService) CheckSyncConflicts(worktreeID string) (*models.MergeConflictError, error) {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Ensure we have full history for accurate conflict detection
	s.fetchFullHistory(worktree)

	// Get the appropriate source reference
	sourceRef := s.getSourceRef(worktree)

	return s.conflictResolver.CheckSyncConflicts(worktree.Path, sourceRef)
}

// CheckMergeConflicts checks if merging a worktree to main would cause conflicts
func (s *GitService) CheckMergeConflicts(worktreeID string) (*models.MergeConflictError, error) {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Only works for local repos
	if !s.isLocalRepo(worktree.RepoID) {
		return nil, fmt.Errorf("merge conflict check only supported for local repositories")
	}

	// Get the local repo
	repo, exists := s.stateManager.GetRepository(worktree.RepoID)
	if !exists {
		return nil, fmt.Errorf("local repository %s not found", worktree.RepoID)
	}

	return s.conflictResolver.CheckMergeConflicts(repo.Path, worktree.Path, worktree.Branch, worktree.SourceBranch, worktree.Name)
}

// GetStateManager returns the worktree state manager
func (s *GitService) GetStateManager() *WorktreeStateManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stateManager
}

// GetCommitSyncService returns the commit sync service
func (s *GitService) GetCommitSyncService() *CommitSyncService {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.commitSync
}

// RenameBranch renames a branch in the given repository
func (s *GitService) RenameBranch(repoPath, oldBranch, newBranch string) error {
	return s.operations.RenameBranch(repoPath, oldBranch, newBranch)
}

// ExecuteGit executes a git command in the given working directory (public wrapper)
func (s *GitService) ExecuteGit(workingDir string, args ...string) ([]byte, error) {
	return s.operations.ExecuteGit(workingDir, args...)
}

// BranchExists checks if a branch exists in the repository (public wrapper)
func (s *GitService) BranchExists(repoPath, branch string, isRemote bool) bool {
	return s.operations.BranchExists(repoPath, branch, isRemote)
}

// RefreshWorktreeStatus triggers an immediate refresh of worktree status cache
func (s *GitService) RefreshWorktreeStatus(workDir string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find worktree by path
	for _, worktree := range s.stateManager.GetAllWorktrees() {
		if worktree.Path == workDir {
			// Trigger cache refresh if available
			if s.worktreeCache != nil {
				s.worktreeCache.ForceRefresh(worktree.ID)
				logger.Infof("🔄 Triggered worktree status refresh for %s", worktree.Name)
			}
			return nil
		}
	}

	return fmt.Errorf("worktree not found for path: %s", workDir)
}

// GitAddCommitGetHash performs git add, commit, and returns the commit hash
// Returns empty string if not a git repository or no changes to commit
func (s *GitService) GitAddCommitGetHash(workspaceDir, message string) (string, error) {
	// Check if it's a git repository
	if !s.operations.IsGitRepository(workspaceDir) {
		logger.Warnf("📂 Not a git repository, skipping git operations for: %s", workspaceDir)
		return "", nil
	}

	// Stage all changes
	if output, err := s.runGitCommand(workspaceDir, "add", "."); err != nil {
		return "", fmt.Errorf("git add failed: %v, output: %s", err, string(output))
	}

	// Check if there are staged changes to commit
	if _, err := s.runGitCommand(workspaceDir, "diff", "--cached", "--quiet"); err == nil {
		return "", nil
	}

	// Commit with the message (with GPG error handling)
	if _, err := s.runGitCommitWithGPGFallback(workspaceDir, "commit", "-m", message, "-n"); err != nil {
		return "", fmt.Errorf("git commit failed: %v", err)
	}

	// Get the commit hash
	output, err := s.runGitCommand(workspaceDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %v", err)
	}

	hash := strings.TrimSpace(string(output))
	return hash, nil
}

// isGPGSigningError checks if the error output indicates a GPG signing failure
func (s *GitService) isGPGSigningError(output string) bool {
	// Check for common GPG signing error patterns
	gpgErrorPatterns := []string{
		"error signing commit",
		"failed to write commit object",
		"unsupported protocol scheme",
		"vscs_internal/commit/sign",
		"gpg failed to sign the data",
		"no default signing key configured",
	}

	outputLower := strings.ToLower(output)
	for _, pattern := range gpgErrorPatterns {
		if strings.Contains(outputLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// disableGPGSigning disables GPG signing for the given repository
func (s *GitService) disableGPGSigning(workspaceDir string) error {
	logger.Infof("🔧 Disabling commit.gpgsign for repository: %s", workspaceDir)

	// Set commit.gpgsign to false for this repository
	_, err := s.runGitCommand(workspaceDir, "config", "--bool", "commit.gpgsign", "false")
	if err != nil {
		return fmt.Errorf("failed to disable commit.gpgsign: %v", err)
	}

	logger.Infof("✅ Successfully disabled GPG signing for repository")
	return nil
}

// runGitCommitWithGPGFallback runs a git commit command with automatic GPG error handling
func (s *GitService) runGitCommitWithGPGFallback(workspaceDir string, args ...string) ([]byte, error) {
	output, err := s.runGitCommand(workspaceDir, args...)
	if err != nil {
		// Check both the output (stdout) and error message (which includes stderr) for GPG errors
		outputStr := string(output)
		errorStr := err.Error()

		if s.isGPGSigningError(outputStr) || s.isGPGSigningError(errorStr) {
			logger.Warnf("🔐 Detected GPG signing error, disabling commit.gpgsign for repository: %s", workspaceDir)
			logger.Debugf("🔍 GPG error detected in: output=%q, error=%q", outputStr, errorStr)

			if disableErr := s.disableGPGSigning(workspaceDir); disableErr != nil {
				logger.Errorf("❌ Failed to disable GPG signing: %v", disableErr)
				return output, err
			}

			// Retry the commit after disabling GPG signing
			logger.Infof("🔄 Retrying commit after disabling GPG signing...")
			retryOutput, retryErr := s.runGitCommand(workspaceDir, args...)
			if retryErr != nil {
				return retryOutput, fmt.Errorf("git commit failed even after disabling GPG: %v", retryErr)
			}
			logger.Infof("✅ Successfully committed after disabling GPG signing")
			return retryOutput, nil
		}
		return output, err
	}
	return output, nil
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
	logger.Infof("🔄 Fetching latest state for branch %s", branch)
	if err := s.fetchBranch(repo.Path, git.FetchStrategy{
		Branch:         branch,
		UpdateLocalRef: true,
	}); err != nil {
		// If fetch fails, check if branch exists locally and proceed if so
		if !s.branchExists(repo.Path, branch, true) {
			return nil, nil, fmt.Errorf("failed to fetch branch %s: %v", branch, err)
		}
		logger.Warnf("⚠️ Fetch failed but branch exists locally, proceeding with checkout")
	}

	// Create new worktree with fun name
	funName := s.generateUniqueSessionName(repo.Path)
	// Creating worktree
	worktree, err := s.createWorktreeInternalForRepo(repo, branch, funName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worktree: %v", err)
	}

	// Save state
	// State persistence handled by state manager

	logger.Infof("✅ Worktree created for existing repository: %s", repo.ID)
	return repo, worktree, nil
}

// createWorktreeInternalForRepo creates a worktree for a specific repository
func (s *GitService) createWorktreeInternalForRepo(repo *models.Repository, source, name string, isInitial bool) (*models.Worktree, error) {
	return s.createWorktreeInternalForRepoWithOptions(repo, source, name, isInitial, true)
}

// createWorktreeInternalForRepoWithOptions creates a worktree with option to skip Claude cleanup (for restoration)
func (s *GitService) createWorktreeInternalForRepoWithOptions(repo *models.Repository, source, name string, isInitial bool, shouldCleanupClaude bool) (*models.Worktree, error) {
	// Use git WorktreeManager to create the worktree
	worktree, err := s.gitWorktreeManager.CreateWorktree(git.CreateWorktreeRequest{
		Repository:   repo,
		SourceBranch: source,
		BranchName:   name,
		WorkspaceDir: getWorkspaceDir(),
		IsInitial:    isInitial,
	})
	if err != nil {
		// Check if the error is because branch already exists or worktree registration conflict
		if strings.Contains(err.Error(), "already exists") {
			logger.Warnf("⚠️  Branch %s already exists, trying a new name...", name)
			// Generate a unique name that doesn't already exist
			newName := s.generateUniqueSessionName(repo.Path)
			return s.createWorktreeInternalForRepoWithOptions(repo, source, newName, isInitial, shouldCleanupClaude)
		} else if strings.Contains(err.Error(), "missing but already registered worktree") {
			logger.Warnf("⚠️  Worktree registration conflict for %s, trying a new name...", name)
			// Generate a unique name that doesn't already exist
			newName := s.generateUniqueSessionName(repo.Path)
			return s.createWorktreeInternalForRepoWithOptions(repo, source, newName, isInitial, shouldCleanupClaude)
		} else if strings.Contains(err.Error(), "worktree creation failed even after cleanup") {
			logger.Warnf("⚠️  Worktree creation failed even after cleanup for %s, trying a new name...", name)
			// Generate a unique name that doesn't already exist
			newName := s.generateUniqueSessionName(repo.Path)
			return s.createWorktreeInternalForRepoWithOptions(repo, source, newName, isInitial, shouldCleanupClaude)
		}
		return nil, err
	}

	// CRITICAL: Clean up any existing Claude session files for this worktree path BEFORE any other initialization
	// This prevents race conditions where the PTY connects and finds old session files
	// Only cleanup for fresh creations, NOT during restoration
	if shouldCleanupClaude && s.claudeMonitor != nil && s.claudeMonitor.claudeService != nil {
		if err := s.claudeMonitor.claudeService.CleanupWorktreeClaudeFiles(worktree.Path); err != nil {
			logger.Warnf("⚠️ Failed to cleanup existing Claude files for new worktree %s: %v", worktree.Path, err)
			// Don't fail the worktree creation, just log the warning
		}
	}

	// Store worktree in service map
	if err := s.stateManager.AddWorktree(worktree); err != nil {
		logger.Warnf("⚠️ Failed to add worktree to state: %v", err)
	}

	// Add to cache and start watching
	s.worktreeCache.AddWorktree(worktree.ID, worktree.Path)

	// Notify CommitSync service about the new worktree
	if s.commitSync != nil {
		s.commitSync.AddWorktreeWatcher(worktree.Path)
	}

	// Notify ClaudeMonitor service about the new worktree
	if s.claudeMonitor != nil {
		s.claudeMonitor.OnWorktreeCreated(worktree.ID, worktree.Path)
	}

	if isInitial || len(s.stateManager.GetAllWorktrees()) == 1 {
		// Update current symlink to point to the first/initial worktree
		_ = s.updateCurrentSymlink(worktree.Path)
	}

	// Execute setup.sh if it exists in the newly created worktree
	if s.setupExecutor != nil {
		logger.Infof("🚀 Scheduling setup.sh execution for worktree: %s", worktree.Path)
		// Run setup.sh execution in a goroutine to avoid blocking worktree creation
		recovery.SafeGo("setup-script-"+worktree.Path, func() {
			// Wait a moment to ensure the worktree is fully ready
			time.Sleep(2 * time.Second)
			logger.Infof("⏰ Starting setup.sh execution for worktree: %s", worktree.Path)
			s.setupExecutor.ExecuteSetupScript(worktree.Path)
		})
	} else {
		logger.Warnf("⚠️ No setup executor configured, skipping setup.sh execution for worktree: %s", worktree.Path)
	}

	return worktree, nil
}

// unshallowRepository unshallows a specific branch in the background
func (s *GitService) unshallowRepository(barePath, branch string) {
	// Wait a bit before starting to avoid interfering with initial setup
	time.Sleep(5 * time.Second)

	// Only fetch the specific branch to be more efficient
	if output, err := s.runGitCommand(barePath, "fetch", "origin", "--unshallow", branch); err != nil {
		// Silent failure - unshallow is optional optimization
		_ = output // Avoid unused variable
		_ = err
	}
}

// GetRepositoryByID returns a repository by its ID
func (s *GitService) GetRepositoryByID(repoID string) *models.Repository {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, _ := s.stateManager.GetRepository(repoID)
	return repo
}

// ListRepositories returns all loaded repositories
func (s *GitService) ListRepositories() []*models.Repository {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reposMap := s.stateManager.GetAllRepositories()
	repos := make([]*models.Repository, 0, len(reposMap))
	for _, repo := range reposMap {
		repos = append(repos, repo)
	}
	return repos
}

// GetWorktreeDiff returns the diff for a worktree against its source branch
func (s *GitService) GetWorktreeDiff(worktreeID string) (*git.WorktreeDiffResponse, error) {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree not found: %s", worktreeID)
	}

	// Get source reference and delegate to WorktreeManager
	sourceRef := s.getSourceRef(worktree)

	// Create fetch function that the WorktreeManager can call if needed
	fetchLatestRef := func(w *models.Worktree) error {
		s.fetchLatestReference(w)
		return nil
	}

	result, err := s.gitWorktreeManager.GetWorktreeDiff(worktree, sourceRef, fetchLatestRef)
	if err != nil {
		return nil, err
	}

	// Set the worktreeID since git WorktreeManager doesn't have access to it
	result.WorktreeID = worktreeID
	return result, nil
}

// CreatePullRequest creates a pull request for a worktree branch
func (s *GitService) CreatePullRequest(worktreeID, title, body string, forcePush bool) (*models.PullRequestResponse, error) {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	if !exists {
		s.mu.RUnlock()
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	repo, exists := s.stateManager.GetRepository(worktree.RepoID)
	if !exists {
		s.mu.RUnlock()
		return nil, fmt.Errorf("repository %s not found", worktree.RepoID)
	}
	s.mu.RUnlock()

	// Check if this is a local repository without a GitHub remote
	if strings.HasPrefix(worktree.RepoID, "local/") && !repo.HasGitHubRemote {
		return nil, fmt.Errorf("NO_GITHUB_REMOTE: this local repository does not have a GitHub remote configured. Please create a GitHub repository first")
	}

	logger.Infof("🔄 Creating pull request for worktree %s", worktree.Name)

	// Check if base branch exists on remote and push if needed
	if err := s.ensureBaseBranchOnRemote(worktree, repo); err != nil {
		return nil, fmt.Errorf("failed to ensure base branch exists on remote: %v", err)
	}

	pr, err := s.githubManager.CreatePullRequest(git.CreatePullRequestRequest{
		Worktree:         worktree,
		Repository:       repo,
		Title:            title,
		Body:             body,
		IsUpdate:         false,
		ForcePush:        forcePush,
		FetchFullHistory: s.fetchFullHistory,
		CreateTempCommit: s.createTemporaryCommit,
		RevertTempCommit: s.revertTemporaryCommit,
	})

	if err != nil {
		return nil, err
	}

	// Save PR metadata to worktree state and emit events
	s.mu.Lock()
	updates := map[string]interface{}{
		"pull_request_url":   pr.URL,
		"pull_request_title": title,
		"pull_request_body":  body,
	}
	if err := s.stateManager.UpdateWorktree(worktreeID, updates); err != nil {
		logger.Warnf("Failed to update worktree %s with PR metadata: %v", worktreeID, err)
	}
	s.mu.Unlock()

	return pr, nil
}

// UpdatePullRequest updates an existing pull request for a worktree branch
func (s *GitService) UpdatePullRequest(worktreeID, title, body string, forcePush bool) (*models.PullRequestResponse, error) {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	if !exists {
		s.mu.RUnlock()
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	repo, exists := s.stateManager.GetRepository(worktree.RepoID)
	if !exists {
		s.mu.RUnlock()
		return nil, fmt.Errorf("repository %s not found", worktree.RepoID)
	}
	s.mu.RUnlock()

	logger.Infof("🔄 Updating pull request for worktree %s", worktree.Name)

	// Check if base branch exists on remote and push if needed
	if err := s.ensureBaseBranchOnRemote(worktree, repo); err != nil {
		return nil, fmt.Errorf("failed to ensure base branch exists on remote: %v", err)
	}

	pr, err := s.githubManager.CreatePullRequest(git.CreatePullRequestRequest{
		Worktree:         worktree,
		Repository:       repo,
		Title:            title,
		Body:             body,
		IsUpdate:         true,
		ForcePush:        forcePush,
		FetchFullHistory: s.fetchFullHistory,
		CreateTempCommit: s.createTemporaryCommit,
		RevertTempCommit: s.revertTemporaryCommit,
	})

	if err != nil {
		return nil, err
	}

	// Save PR metadata to worktree state (in case it changed) and emit events
	s.mu.Lock()
	updates := map[string]interface{}{
		"pull_request_url":   pr.URL,
		"pull_request_title": title,
		"pull_request_body":  body,
	}
	if err := s.stateManager.UpdateWorktree(worktreeID, updates); err != nil {
		logger.Warnf("Failed to update worktree %s with PR metadata: %v", worktreeID, err)
	}
	s.mu.Unlock()

	return pr, nil
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
				logger.Warnf("⚠️ No remote configured for local repo %s, skipping base branch check", worktree.RepoID)
				return nil
			}
		}

		// Check if base branch exists on remote
		if err := s.checkBaseBranchOnRemote(worktree, remoteURL); err != nil {
			logger.Infof("🔄 Base branch %s not found on remote, pushing it", worktree.SourceBranch)
			if err := s.pushBaseBranchToRemote(worktree, repo, remoteURL); err != nil {
				return fmt.Errorf("failed to push base branch to remote: %v", err)
			}
		}
	} else {
		// For remote repositories, ensure we have the latest base branch
		if err := s.fetchBaseBranchFromOrigin(worktree); err != nil {
			logger.Warnf("⚠️ Could not fetch base branch from origin: %v", err)
			// This is not a fatal error, continue with PR creation
		}
	}

	return nil
}

// checkBaseBranchOnRemote checks if the base branch exists on the remote repository
func (s *GitService) checkBaseBranchOnRemote(worktree *models.Worktree, remoteURL string) error {
	// Convert SSH URLs to HTTPS to avoid authentication issues
	httpsURL := git.ConvertSSHToHTTPS(remoteURL)
	logger.Debugf("🔍 Checking base branch on remote: %s -> %s", remoteURL, httpsURL)

	// Use git ls-remote to check if the base branch exists on remote
	output, err := s.runGitCommand("", "ls-remote", "--heads", httpsURL, worktree.SourceBranch)
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
	logger.Infof("🔄 Syncing branch %s with upstream due to push failure", worktree.Branch)

	// First, fetch the latest changes from remote
	if err := s.fetchBranch(worktree.Path, git.FetchStrategy{
		Branch: worktree.Branch,
	}); err != nil {
		// If fetch fails, the branch might not exist on remote yet - that's OK
		logger.Warnf("⚠️ Could not fetch remote branch %s (might not exist yet): %v", worktree.Branch, err)
		return nil
	}

	// Check if we're behind the remote branch
	output, err := s.runGitCommand(worktree.Path, "rev-list", "--count", fmt.Sprintf("HEAD..origin/%s", worktree.Branch))
	if err != nil {
		// If this fails, assume we're not behind
		return nil
	}

	behindCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || behindCount == 0 {
		// We're not behind, no need to sync
		return nil
	}

	logger.Infof("🔄 Branch %s is %d commits behind remote, syncing", worktree.Branch, behindCount)

	// Rebase our changes on top of the remote branch
	output, err = s.runGitCommand(worktree.Path, "rebase", fmt.Sprintf("origin/%s", worktree.Branch))
	if err != nil {
		// Check if this is a rebase conflict
		if strings.Contains(string(output), "CONFLICT") {
			return fmt.Errorf("rebase conflict occurred while syncing with upstream. Please resolve conflicts manually in the terminal")
		}
		return fmt.Errorf("failed to rebase on upstream: %v\n%s", err, output)
	}

	logger.Infof("✅ Successfully synced branch %s with upstream", worktree.Branch)
	return nil
}

// Removed setupRemoteOrigin - remote setup is now handled by URL manager with .insteadOf

// GetPullRequestInfo gets information about an existing pull request for a worktree
func (s *GitService) GetPullRequestInfo(worktreeID string) (*models.PullRequestInfo, error) {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Get the repository
	repo, exists := s.stateManager.GetRepository(worktree.RepoID)
	if !exists {
		return nil, fmt.Errorf("repository %s not found", worktree.RepoID)
	}

	// Check if branch has commits ahead of the base branch
	hasCommitsAhead, err := s.checkHasCommitsAhead(worktree)
	if err != nil {
		logger.Warnf("⚠️ Could not check commits ahead: %v", err)
		hasCommitsAhead = false // Default to false if we can't determine
	}

	// Initialize PR info with commits ahead status
	prInfo := &models.PullRequestInfo{
		HasCommitsAhead: hasCommitsAhead,
		Exists:          false,
	}

	// GitHubManager handles URL parsing and PR checking internally

	// Get PR info from GitHub manager (already handles checking existing PR)
	if ghPrInfo, err := s.githubManager.GetPullRequestInfo(worktree, repo); err != nil {
		logger.Warnf("⚠️ Could not check for existing PR: %v", err)
	} else {
		prInfo = ghPrInfo
		// Override HasCommitsAhead with our more specific check (local vs remote branch)
		// GitHubManager just checks if commit count > 0 (ahead of base)
		if ahead, err := s.checkHasCommitsAheadOfRemote(worktree); err == nil {
			prInfo.HasCommitsAhead = ahead
		} else {
			logger.Warnf("⚠️ Failed to check commits ahead of remote: %v", err)
			// Fallback to generic check if remote check fails
			prInfo.HasCommitsAhead = hasCommitsAhead
		}
	}

	// Override with persisted PR data if available (gives precedence to locally stored data)
	if worktree.PullRequestURL != "" {
		prInfo.Exists = true
		prInfo.URL = worktree.PullRequestURL

		// Use persisted title and body if available (for updates)
		if worktree.PullRequestTitle != "" {
			prInfo.Title = worktree.PullRequestTitle
		}
		if worktree.PullRequestBody != "" {
			prInfo.Body = worktree.PullRequestBody
		}
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
		if _, err := s.runGitCommand(worktree.Path, "fetch", "origin", worktree.SourceBranch); err != nil {
			logger.Warnf("⚠️ Could not fetch base branch %s: %v", worktree.SourceBranch, err)
		}
		baseRef = fmt.Sprintf("origin/%s", worktree.SourceBranch)
	}

	// Count commits ahead of base branch
	output, err := s.runGitCommand(worktree.Path, "rev-list", "--count", fmt.Sprintf("%s..HEAD", baseRef))
	if err != nil {
		return false, fmt.Errorf("failed to count commits ahead: %v", err)
	}

	commitCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return false, fmt.Errorf("failed to parse commit count: %v", err)
	}

	return commitCount > 0, nil
}

// checkHasCommitsAheadOfRemote checks if the worktree branch has commits ahead of the remote branch
func (s *GitService) checkHasCommitsAheadOfRemote(worktree *models.Worktree) (bool, error) {
	// We want to check if we have commits that are not yet on the remote branch
	// This is effectively: git rev-list --count origin/<branch>..HEAD

	// Throttle fetches: only fetch if we haven't fetched this repo recently
	s.lastFetchMu.Lock()
	lastFetch, exists := s.lastFetchTimes[worktree.Path]
	shouldFetch := !exists || time.Since(lastFetch) > s.fetchThrottlePeriod
	if shouldFetch {
		s.lastFetchTimes[worktree.Path] = time.Now()
	}
	s.lastFetchMu.Unlock()

	// First, fetch origin/<branch> to be sure we have the latest state (if not throttled)
	if shouldFetch {
		if _, err := s.runGitCommand(worktree.Path, "fetch", "origin", worktree.Branch); err != nil {
			// If fetch fails, it might be because the remote branch doesn't exist yet.
			// In that case, we'll fall back to checking if we have any commits at all.
			logger.Debugf("⚠️ Could not fetch remote branch %s: %v", worktree.Branch, err)
		}
	} else {
		logger.Debugf("⏭️ Skipping fetch for %s (last fetched %v ago)", worktree.Path, time.Since(lastFetch))
	}

	remoteRef := fmt.Sprintf("origin/%s", worktree.Branch)

	// Check if remote ref exists
	if !s.branchExists(worktree.Path, remoteRef, true) {
		// Remote branch doesn't exist.
		// If we have any commits, we are technically "ahead" (all commits are new).
		// However, for "Update PR", we usually care about updating an EXISTING PR.
		// If the PR exists, the remote branch SHOULD exist.
		// If we return true here, the UI might show "Update PR" but the push might fail if PR doesn't exist?
		// Actually, if PR doesn't exist, GetPullRequestInfo will return Exists=false anyway.
		// So returning true here is safe/correct: we have unpushed commits.
		return worktree.CommitCount > 0, nil
	}

	// Count commits ahead of remote branch
	output, err := s.runGitCommand(worktree.Path, "rev-list", "--count", fmt.Sprintf("%s..HEAD", remoteRef))
	if err != nil {
		return false, fmt.Errorf("failed to count commits ahead of remote: %v", err)
	}

	commitCount, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return false, fmt.Errorf("failed to parse commit count: %v", err)
	}

	return commitCount > 0, nil
}

// SetEventsHandler is deprecated - use SetEventsEmitter instead
func (s *GitService) SetEventsHandler(eventsHandler EventsEmitter) {
	s.SetEventsEmitter(eventsHandler)
}

// IsWorktreeStatusCached returns true if we have cached status for a worktree
func (s *GitService) IsWorktreeStatusCached(worktreeID string) bool {
	if s.worktreeCache == nil {
		return false
	}
	return s.worktreeCache.IsStatusCached(worktreeID)
}

// RefreshWorktreeStatusByID forces an immediate refresh of a worktree's status by ID
func (s *GitService) RefreshWorktreeStatusByID(worktreeID string) error {
	s.mu.RLock()
	worktree, exists := s.stateManager.GetWorktree(worktreeID)
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Create a function that provides the source reference
	getSourceRefFunc := func(w *models.Worktree) string {
		return s.getSourceRef(w)
	}

	// Force update the worktree status using the WorktreeManager
	s.gitWorktreeManager.UpdateWorktreeStatus(worktree, getSourceRefFunc)

	// Create updates map for the state manager
	updates := map[string]interface{}{
		"commit_hash":    worktree.CommitHash,
		"commit_count":   worktree.CommitCount,
		"commits_behind": worktree.CommitsBehind,
		"is_dirty":       worktree.IsDirty,
		"has_conflicts":  worktree.HasConflicts,
	}

	// Update the state manager with the new values
	if err := s.stateManager.UpdateWorktree(worktreeID, updates); err != nil {
		return fmt.Errorf("failed to update worktree state: %v", err)
	}

	logger.Infof("✅ Force refreshed worktree %s status: %d commits ahead", worktree.Name, worktree.CommitCount)
	return nil
}

// CreateFromTemplate creates a new project from a template using bare repository approach
func (s *GitService) CreateFromTemplate(templateID, projectName string) (*models.Repository, *models.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate project name
	if projectName == "" {
		return nil, nil, fmt.Errorf("project name is required")
	}

	// Get GitHub username from git config for template repos
	// This distinguishes them from local clones of live repos
	var repoID string
	usernameOutput, err := s.operations.ExecuteGit(".", "config", "--global", "user.name")
	if err != nil || strings.TrimSpace(string(usernameOutput)) == "" {
		// Fallback to "template" if no username is configured
		logger.Warnf("⚠️ No git user.name configured, using 'template' prefix for repository")
		repoID = fmt.Sprintf("template/%s", projectName)
	} else {
		username := strings.TrimSpace(string(usernameOutput))
		repoID = fmt.Sprintf("%s/%s", username, projectName)
	}

	// Check if repository already exists in our state
	if _, exists := s.stateManager.GetRepository(repoID); exists {
		return nil, nil, fmt.Errorf("project %s already exists", projectName)
	}

	// Set up bare repository path in /volume/repos (persistent)
	reposDir := filepath.Join(config.Runtime.VolumeDir, "repos")
	if err := os.MkdirAll(reposDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create repos directory: %v", err)
	}

	barePath := filepath.Join(reposDir, fmt.Sprintf("%s.git", projectName))

	// Check if bare repository already exists on disk
	if _, err := os.Stat(barePath); err == nil {
		return nil, nil, fmt.Errorf("bare repository already exists at %s", barePath)
	}

	// Create temporary directory for template setup
	tempDir := filepath.Join("/tmp", fmt.Sprintf("template-%s-%d", projectName, time.Now().Unix()))
	defer os.RemoveAll(tempDir)

	// Create the temp directory first
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create temp directory: %v", err)
	}

	projectPath := filepath.Join(tempDir, projectName)

	// Create the project based on template type
	logger.Infof("🏗️ Creating project from template %s at %s", templateID, projectPath)

	var cmd *exec.Cmd
	switch templateID {
	case "react-vite":
		cmd = exec.Command("pnpm", "create", "vite", projectName, "--template", "react-ts", "--yes")
		cmd.Dir = tempDir
	case "basic":
		// For basic template, we create the directory manually and populate it
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create project directory: %v", err)
		}
	case "vue-vite":
		cmd = exec.Command("pnpm", "create", "vite", projectName, "--template", "vue-ts", "--yes")
		cmd.Dir = tempDir
	case "nextjs-app":
		cmd = exec.Command("pnpm", "create", "next-app", projectName, "--typescript", "--tailwind", "--app", "--no-eslint", "--yes")
		cmd.Dir = tempDir
	case "node-express", "python-fastapi":
		// For these, we create the directory manually and populate it
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create project directory: %v", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported template: %s", templateID)
	}

	// Execute the creation command if one was set
	if cmd != nil {
		logger.Infof("🏗️ Running command: %s", strings.Join(cmd.Args, " "))
		output, err := cmd.CombinedOutput()
		logger.Debugf("📄 Command output: %s", string(output))
		if err != nil {
			logger.Warnf("❌ Command failed: %v", err)
			return nil, nil, fmt.Errorf("failed to create project: %v\nOutput: %s", err, string(output))
		}
		logger.Infof("✅ Command completed successfully")
	}

	// Verify the project directory was created
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		logger.Warnf("❌ Project directory %s does not exist after command execution", projectPath)
		return nil, nil, fmt.Errorf("project directory %s was not created by template command", projectPath)
	}
	logger.Infof("✅ Project directory verified: %s", projectPath)

	// Create README.md for basic template
	if templateID == "basic" {
		readmePath := filepath.Join(projectPath, "README.md")
		readmeContent := "# Hello World\n\nThis is a basic project created with Catnip.\n"
		if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to create README.md: %v", err)
		}
		logger.Infof("✅ Created README.md for basic template")
	}

	// For templates that just create directories, we need to set up the files manually
	supportedTemplates := templates.GetSupportedTemplates()
	isSupported := false
	for _, supported := range supportedTemplates {
		if templateID == supported {
			isSupported = true
			break
		}
	}
	if isSupported {
		if err := templates.SetupTemplateFiles(templateID, projectPath); err != nil {
			return nil, nil, fmt.Errorf("failed to setup template files: %v", err)
		}
	}

	// Initialize git repository in temp directory
	if output, err := s.runGitCommand(projectPath, "init"); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize git repo: %v\nOutput: %s", err, string(output))
	}

	// Configure git user for the repo (needed for commits)
	_, _ = s.runGitCommand(projectPath, "config", "user.email", "user@catnip.local")
	_, _ = s.runGitCommand(projectPath, "config", "user.name", "Catnip User")

	// Add all files and make initial commit
	if output, err := s.runGitCommand(projectPath, "add", "."); err != nil {
		logger.Warnf("⚠️ Failed to add files to git: %v\nOutput: %s", err, string(output))
	}

	commitMsg := fmt.Sprintf("Initial commit from %s template", templateID)
	if _, err := s.runGitCommitWithGPGFallback(projectPath, "commit", "-m", commitMsg); err != nil {
		logger.Warnf("⚠️ Failed to make initial commit: %v", err)
	}

	// Clone the temporary repository as a bare repository to the persistent location
	if output, err := s.runGitCommand("", "clone", "--bare", projectPath, barePath); err != nil {
		return nil, nil, fmt.Errorf("failed to create bare repository: %v\nOutput: %s", err, string(output))
	}

	// Get the default branch from the bare repository
	defaultBranch, err := s.getDefaultBranch(barePath)
	if err != nil {
		// Clean up on failure
		os.RemoveAll(barePath)
		return nil, nil, fmt.Errorf("failed to get default branch: %v", err)
	}

	// Create repository object pointing to the bare repository
	repo := &models.Repository{
		ID:            repoID,
		URL:           fmt.Sprintf("file://%s", barePath), // Use file URL to indicate local bare repo
		Path:          barePath,
		DefaultBranch: defaultBranch,
		Description:   fmt.Sprintf("Created from %s template", templateID),
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
		Available:     true,
	}

	// Add repository to state
	if err := s.stateManager.AddRepository(repo); err != nil {
		logger.Warnf("⚠️ Failed to add repository to state: %v", err)
	}

	// Create an initial worktree for the template project so the user can immediately start working
	logger.Infof("🌱 Creating initial worktree for template project %s", projectName)

	// Generate a unique session name for the initial worktree
	funName := s.generateUniqueSessionName(repo.Path)

	// Create worktree using the bare repository (similar to remote repos)
	worktree, err := s.createWorktreeInternalForRepo(repo, defaultBranch, funName, true)
	if err != nil {
		logger.Warnf("⚠️ Failed to create initial worktree for template project: %v", err)
		// Still return success since the repository was created successfully
		// The user can create worktrees manually later
		return repo, nil, nil
	}

	logger.Infof("✅ Successfully created project %s from template %s with bare repository at %s and initial worktree %s",
		projectName, templateID, barePath, worktree.Name)
	return repo, worktree, nil
}

// RecreateWorktree implements the WorktreeRestorer interface
// This method manually restores worktrees by leveraging existing git metadata
// instead of using `git worktree add` which fails due to registration conflicts
func (s *GitService) RecreateWorktree(worktree *models.Worktree, repo *models.Repository) error {
	logger.Infof("🔄 Manually restoring worktree %s at %s (from repo %s)", worktree.Name, worktree.Path, repo.Path)

	// Step 1: Create the workspace directory
	if err := os.MkdirAll(worktree.Path, 0755); err != nil {
		logger.Warnf("❌ Failed to create workspace directory %s: %v", worktree.Path, err)
		return fmt.Errorf("failed to create workspace directory %s: %v", worktree.Path, err)
	}
	logger.Infof("✅ Created workspace directory: %s", worktree.Path)

	// Step 2: Determine the correct worktree metadata path
	// Extract workspace name from the worktree path
	workspaceName := filepath.Base(worktree.Path)
	// Handle different repo types:
	// - Local repos (e.g., /live/catnip): metadata at /live/catnip/.git/worktrees/coal
	// - Remote repos (e.g., /volume/repos/slide.git): metadata at /volume/repos/slide.git/worktrees/buddy
	var worktreeMetadataPath string
	if strings.HasSuffix(repo.Path, ".git") {
		// Bare repository (remote repo) - worktrees directly under repo path
		worktreeMetadataPath = filepath.Join(repo.Path, "worktrees", workspaceName)
	} else {
		// Regular repository (local repo) - worktrees under .git subdirectory
		worktreeMetadataPath = filepath.Join(repo.Path, ".git", "worktrees", workspaceName)
	}

	// Check if worktree metadata exists
	if _, err := os.Stat(worktreeMetadataPath); os.IsNotExist(err) {
		logger.Warnf("⚠️ Worktree metadata not found at %s - falling back to fresh worktree creation", worktreeMetadataPath)

		// For renamed branches, we need to find the original catnip branch reference
		branchRef := worktree.Branch
		if worktree.HasBeenRenamed && !strings.HasPrefix(worktree.Branch, "refs/catnip/") {
			parts := strings.Split(worktree.Name, "/")
			workspaceName := parts[len(parts)-1]
			branchRef = fmt.Sprintf("refs/catnip/%s", workspaceName)
			logger.Debugf("🔍 Using catnip ref %s for recreating renamed worktree %s", branchRef, worktree.Name)
		}

		// Use internal worktree creation logic WITHOUT Claude cleanup (restoration context)
		logger.Warnf("🔧 Creating fresh worktree during restoration (no Claude cleanup): repo=%s, sourceBranch=%s, branchName=%s",
			repo.Path, worktree.SourceBranch, branchRef)

		_, err := s.createWorktreeInternalForRepoWithOptions(repo, worktree.SourceBranch, branchRef, false, false)

		if err != nil {
			logger.Warnf("❌ Fresh worktree creation failed for %s: %v", worktree.Name, err)
			return fmt.Errorf("failed to create fresh worktree: %v", err)
		}

		logger.Infof("✅ Successfully created fresh worktree %s", worktree.Name)
		return nil
	}
	logger.Infof("✅ Found worktree metadata at: %s", worktreeMetadataPath)

	// Step 3: Create the .git file pointing to the worktree metadata
	gitFilePath := filepath.Join(worktree.Path, ".git")
	gitFileContent := fmt.Sprintf("gitdir: %s", worktreeMetadataPath)
	if err := os.WriteFile(gitFilePath, []byte(gitFileContent), 0644); err != nil {
		logger.Warnf("❌ Failed to create .git file at %s: %v", gitFilePath, err)
		return fmt.Errorf("failed to create .git file: %v", err)
	}
	logger.Infof("✅ Created .git file pointing to metadata: %s", gitFilePath)

	// Step 4: Restore files from git index
	logger.Infof("🔄 Restoring files from git index...")
	restoreCmd := []string{"restore", "."}
	if _, err := s.operations.ExecuteGit(worktree.Path, restoreCmd...); err != nil {
		logger.Warnf("❌ Failed to restore files in %s: %v", worktree.Path, err)

		// Check if it's an index.lock issue and try to recover
		if strings.Contains(err.Error(), "index.lock") {
			logger.Infof("🔧 Detected index.lock issue, attempting recovery...")

			// Find the index.lock file path
			worktreeMetadataPath := filepath.Join(repo.Path, "worktrees", filepath.Base(worktree.Path))
			if !strings.HasSuffix(repo.Path, ".git") {
				worktreeMetadataPath = filepath.Join(repo.Path, ".git", "worktrees", filepath.Base(worktree.Path))
			}
			indexLockPath := filepath.Join(worktreeMetadataPath, "index.lock")

			// Remove stale index.lock file
			if err := os.Remove(indexLockPath); err != nil {
				logger.Warnf("⚠️ Failed to remove stale index.lock file %s: %v", indexLockPath, err)
			} else {
				logger.Infof("✅ Removed stale index.lock file: %s", indexLockPath)

				// Retry the restore operation
				logger.Infof("🔄 Retrying file restoration...")
				if _, retryErr := s.operations.ExecuteGit(worktree.Path, restoreCmd...); retryErr != nil {
					logger.Warnf("❌ Retry failed: %v", retryErr)
					return fmt.Errorf("failed to restore files after index.lock recovery: %v", retryErr)
				}
				logger.Infof("✅ Successfully restored files after index.lock recovery")
			}
		} else {
			return fmt.Errorf("failed to restore files: %v", err)
		}
	} else {
		logger.Infof("✅ Restored files from git index")
	}

	// Step 5: Verify the restoration
	statusCmd := []string{"status", "--porcelain"}
	if output, err := s.operations.ExecuteGit(worktree.Path, statusCmd...); err != nil {
		logger.Warnf("⚠️ Could not verify git status after restoration: %v", err)
	} else if strings.TrimSpace(string(output)) == "" {
		logger.Infof("✅ Worktree restoration verified - working tree is clean")
	} else {
		logger.Warnf("⚠️ Worktree may have uncommitted changes after restoration")
	}

	// Step 6: Recreate nice branch name for renamed worktrees
	if worktree.HasBeenRenamed {
		logger.Infof("🔄 Worktree has been renamed, recreating nice branch name...")

		// Get current branch (should be refs/catnip/workspacename)
		currentBranchOutput, err := s.operations.ExecuteGit(worktree.Path, "rev-parse", "--symbolic-full-name", "HEAD")
		if err != nil {
			logger.Warnf("⚠️ Failed to get current branch for renamed worktree %s: %v", worktree.Name, err)
		} else {
			currentBranch := strings.TrimSpace(string(currentBranchOutput))
			logger.Debugf("🔍 Current branch: %s", currentBranch)

			// Look up the nice branch name from git config
			configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(currentBranch, "/", "."))
			niceBranchName, err := s.operations.GetConfig(worktree.Path, configKey)
			if err == nil && strings.TrimSpace(niceBranchName) != "" {
				niceBranchName = strings.TrimSpace(niceBranchName)
				logger.Debugf("🔍 Found nice branch mapping: %s -> %s", currentBranch, niceBranchName)

				// Get current commit hash
				currentCommit, err := s.operations.GetCommitHash(worktree.Path, "HEAD")
				if err != nil {
					logger.Warnf("⚠️ Failed to get current commit for branch recreation: %v", err)
				} else {
					// Create the nice branch pointing to the same commit
					if err := s.operations.CreateBranch(worktree.Path, niceBranchName, currentCommit); err != nil {
						logger.Warnf("⚠️ Failed to recreate nice branch %q: %v", niceBranchName, err)
					} else {
						logger.Infof("✅ Successfully recreated nice branch %q pointing to %s", niceBranchName, currentCommit[:8])
					}
				}
			} else {
				logger.Warnf("⚠️ No nice branch mapping found for %s in git config", currentBranch)
			}
		}
	}

	logger.Infof("✅ Successfully restored worktree %s using manual restoration", worktree.Name)
	return nil
}

// RestoreState restores worktree state from persistent storage
func (s *GitService) RestoreState() error {
	return s.stateManager.RestoreState()
}

// CreateGitHubRepositoryAndSetOrigin creates a GitHub repository and sets it as origin for a local repo
func (s *GitService) CreateGitHubRepositoryAndSetOrigin(repoID, name, description string, isPrivate bool) (string, error) {
	logger.Infof("🔍 Looking up repository with ID: '%s'", repoID)

	s.mu.RLock()
	repo, exists := s.stateManager.GetRepository(repoID)
	if !exists {
		s.mu.RUnlock()
		logger.Errorf("❌ Repository '%s' not found in state manager", repoID)

		// Debug: List all available repositories
		s.mu.RLock()
		allRepos := s.stateManager.GetAllRepositories()
		s.mu.RUnlock()
		logger.Infof("🔍 Available repositories:")
		for id := range allRepos {
			logger.Infof("  - '%s'", id)
		}

		return "", fmt.Errorf("repository %s not found", repoID)
	}
	s.mu.RUnlock()

	logger.Infof("✅ Found repository: %s (path: %s)", repoID, repo.Path)
	logger.Infof("🚀 Creating GitHub repository %s for repo %s", name, repoID)

	// Create the GitHub repository
	repoURL, err := s.githubManager.CreateRepository(name, description, isPrivate)
	if err != nil {
		return "", fmt.Errorf("failed to create GitHub repository: %v", err)
	}

	logger.Infof("✅ Created GitHub repository: %s", repoURL)

	// Update the local repository's origin
	gitURL := strings.Replace(repoURL, "https://github.com/", "git@github.com:", 1) + ".git"
	if err := s.operations.SetRemoteURL(repo.Path, "origin", gitURL); err != nil {
		logger.Warnf("⚠️ Failed to set remote origin to %s: %v", gitURL, err)
		// Try HTTPS format as fallback
		httpsURL := repoURL + ".git"
		if err := s.operations.SetRemoteURL(repo.Path, "origin", httpsURL); err != nil {
			return "", fmt.Errorf("failed to set remote origin: %v", err)
		}
		logger.Infof("✅ Set remote origin to %s (HTTPS)", httpsURL)
	} else {
		logger.Infof("✅ Set remote origin to %s (SSH)", gitURL)
	}

	// Update repository state with new remote information
	s.mu.Lock()
	repo.RemoteOrigin = repoURL + ".git"
	repo.HasGitHubRemote = true
	repo.URL = repoURL
	if err := s.stateManager.AddRepository(repo); err != nil {
		logger.Warnf("Failed to update repository %s with remote info: %v", repoID, err)
	}
	s.mu.Unlock()

	// Push the main branch to the newly created GitHub repository
	// This must happen after updating the repository state so operations can see the new remote
	logger.Infof("📤 Pushing main branch to GitHub repository...")
	pushStrategy := git.PushStrategy{
		Branch:      repo.DefaultBranch,
		Remote:      "origin",
		SetUpstream: true, // Set upstream for the first push
	}
	if err := s.operations.PushBranch(repo.Path, pushStrategy); err != nil {
		logger.Warnf("⚠️ Failed to push %s branch to origin: %v", repo.DefaultBranch, err)
		// Don't fail the entire operation if push fails - the repo is created and origin is set
	} else {
		logger.Infof("✅ Successfully pushed %s branch to GitHub", repo.DefaultBranch)
	}

	return repoURL, nil
}

// DeleteRepository removes a repository and all its worktrees from disk and state management
func (s *GitService) DeleteRepository(repoID string) error {
	logger.Infof("🗑️  Delete repository request: %s", repoID)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get the repository
	repo, exists := s.stateManager.GetRepository(repoID)
	if !exists {
		return fmt.Errorf("repository not found: %s", repoID)
	}

	// Get all worktrees for this repository
	allWorktrees := s.stateManager.GetAllWorktrees()
	var repoWorktrees []*models.Worktree
	for _, worktree := range allWorktrees {
		if worktree.RepoID == repoID {
			repoWorktrees = append(repoWorktrees, worktree)
		}
	}

	// Delete all worktrees first
	for _, worktree := range repoWorktrees {
		logger.Infof("🗑️  Deleting worktree %s (%s)", worktree.Name, worktree.ID)

		// Remove worktree directory from disk
		if _, err := os.Stat(worktree.Path); err == nil {
			if err := os.RemoveAll(worktree.Path); err != nil {
				logger.Warnf("⚠️  Failed to remove worktree directory %s: %v", worktree.Path, err)
				// Continue with deletion even if directory removal fails
			}
		}

		// Remove from state management
		if err := s.stateManager.DeleteWorktree(worktree.ID); err != nil {
			logger.Warnf("⚠️  Failed to remove worktree from state: %v", err)
		}
	}

	// Remove repository directory from disk
	if _, err := os.Stat(repo.Path); err == nil {
		if err := os.RemoveAll(repo.Path); err != nil {
			logger.Warnf("⚠️  Failed to remove repository directory %s: %v", repo.Path, err)
			// Don't fail the entire operation if directory removal fails
		} else {
			logger.Infof("✅ Removed repository directory: %s", repo.Path)
		}
	}

	// Remove from state management
	if err := s.stateManager.DeleteRepository(repoID); err != nil {
		return fmt.Errorf("failed to remove repository from state: %v", err)
	}

	// Clear any cached status for all worktrees
	for _, worktree := range repoWorktrees {
		s.worktreeCache.RemoveWorktree(worktree.ID, worktree.Path)
	}

	logger.Infof("✅ Successfully deleted repository %s and %d worktrees", repoID, len(repoWorktrees))
	return nil
}

// isTemporaryPath checks if a path is in a temporary directory (for tests)
// Handles both Linux (/tmp/) and macOS (/var/folders/) temporary paths
func (s *GitService) isTemporaryPath(path string) bool {
	// Linux temporary directory
	if strings.Contains(path, "/tmp/") {
		return true
	}

	// macOS temporary directory pattern: /var/folders/xx/xxxxxxxxx/T/
	if strings.Contains(path, "/var/folders/") && strings.Contains(path, "/T/") {
		return true
	}

	return false
}
