package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// GitOperations interface for branch renaming operations
type GitOperations interface {
	GetCommitHash(worktreePath, ref string) (string, error)
	CreateBranch(repoPath, branch, fromRef string) error
	BranchExists(repoPath, branch string, isRemote bool) bool
	SetConfig(repoPath, key, value string) error
	GetConfig(repoPath, key string) (string, error)
}

// WorktreeRestorer interface for recreating worktrees during state restoration
type WorktreeRestorer interface {
	RecreateWorktree(worktree *models.Worktree, repo *models.Repository) error
}

// WorktreeStateChange represents a change to worktree state
type WorktreeStateChange struct {
	Type       string // "created", "updated", "deleted"
	WorktreeID string
	Fields     map[string]interface{} // Changed fields for updates
	Worktree   *models.Worktree       // Full worktree for creates
}

// WorktreeStateManager manages all worktree state persistently
type WorktreeStateManager struct {
	mu               sync.RWMutex
	repositories     map[string]*models.Repository
	worktrees        map[string]*models.Worktree
	stateDir         string
	eventsEmitter    EventsEmitter
	sessionService   *SessionService
	worktreeRestorer WorktreeRestorer

	// Track field-level changes
	previousState map[string]worktreeFieldState

	// Periodic sync control
	stopChan chan struct{}
}

// worktreeFieldState tracks all fields we care about for change detection
type worktreeFieldState struct {
	ID                     string
	Name                   string
	Branch                 string
	SourceBranch           string
	CommitHash             string
	CommitCount            int
	CommitsBehind          int
	IsDirty                bool
	HasConflicts           bool
	PullRequestURL         string
	SessionTitle           *models.TitleEntry
	SessionTitleHistory    []models.TitleEntry
	HasActiveClaudeSession bool
	ClaudeActivityState    models.ClaudeActivityState
	Todos                  []models.Todo
	HasBeenRenamed         bool // Whether this worktree has had its branch renamed
}

// NewWorktreeStateManager creates a new centralized state manager
func NewWorktreeStateManager(stateDir string, eventsEmitter EventsEmitter) *WorktreeStateManager {
	wsm := &WorktreeStateManager{
		repositories:  make(map[string]*models.Repository),
		worktrees:     make(map[string]*models.Worktree),
		stateDir:      stateDir,
		eventsEmitter: eventsEmitter,
		previousState: make(map[string]worktreeFieldState),
		stopChan:      make(chan struct{}),
	}

	// Load existing state
	if err := wsm.loadState(); err != nil {
		logger.Warnf("⚠️ Failed to load state: %v", err)
	}

	return wsm
}

// SetSessionService sets the session service and starts periodic Claude activity state checking
func (wsm *WorktreeStateManager) SetSessionService(sessionService *SessionService) {
	wsm.mu.Lock()
	wsm.sessionService = sessionService
	wsm.mu.Unlock()

	// Start periodic Claude activity state checking
	go wsm.startClaudeActivitySync()
}

// SetWorktreeRestorer sets the worktree restorer for state restoration
func (wsm *WorktreeStateManager) SetWorktreeRestorer(restorer WorktreeRestorer) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	wsm.worktreeRestorer = restorer
}

// Stop stops the periodic syncing
func (wsm *WorktreeStateManager) Stop() {
	close(wsm.stopChan)
}

// GetRepository returns a repository by ID
func (wsm *WorktreeStateManager) GetRepository(repoID string) (*models.Repository, bool) {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()
	repo, exists := wsm.repositories[repoID]
	return repo, exists
}

// GetWorktree returns a worktree by ID
func (wsm *WorktreeStateManager) GetWorktree(worktreeID string) (*models.Worktree, bool) {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()
	wt, exists := wsm.worktrees[worktreeID]
	return wt, exists
}

// GetAllWorktrees returns all worktrees
func (wsm *WorktreeStateManager) GetAllWorktrees() map[string]*models.Worktree {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()

	// Return a copy to prevent external modifications
	result := make(map[string]*models.Worktree, len(wsm.worktrees))
	for id, wt := range wsm.worktrees {
		wtCopy := *wt
		result[id] = &wtCopy
	}
	return result
}

// GetAllRepositories returns all repositories
func (wsm *WorktreeStateManager) GetAllRepositories() map[string]*models.Repository {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()

	// Return a copy to prevent external modifications
	result := make(map[string]*models.Repository, len(wsm.repositories))
	for id, repo := range wsm.repositories {
		repoCopy := *repo
		result[id] = &repoCopy
	}
	return result
}

// AddRepository adds or updates a repository
func (wsm *WorktreeStateManager) AddRepository(repo *models.Repository) error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	// Ensure new repositories are marked as available by default
	if !repo.Available {
		repo.Available = true
	}

	wsm.repositories[repo.ID] = repo
	return wsm.saveStateInternal()
}

// IsRepositoryAvailable checks if a repository is available for operations
func (wsm *WorktreeStateManager) IsRepositoryAvailable(repoID string) bool {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()

	repo, exists := wsm.repositories[repoID]
	return exists && repo.Available
}

// AddWorktree adds a new worktree
func (wsm *WorktreeStateManager) AddWorktree(worktree *models.Worktree) error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	// Check if the associated repository is available
	repo, repoExists := wsm.repositories[worktree.RepoID]
	if !repoExists {
		return fmt.Errorf("repository %s not found", worktree.RepoID)
	}
	if !repo.Available {
		return fmt.Errorf("repository %s is not available", worktree.RepoID)
	}

	wsm.worktrees[worktree.ID] = worktree

	// Save state
	if err := wsm.saveStateInternal(); err != nil {
		return err
	}

	// Emit created event
	if wsm.eventsEmitter != nil {
		wsm.eventsEmitter.EmitWorktreeCreated(worktree)
	}

	return nil
}

// UpdateWorktree updates specific fields of a worktree
func (wsm *WorktreeStateManager) UpdateWorktree(worktreeID string, updates map[string]interface{}) error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	worktree, exists := wsm.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Apply updates based on field names
	for field, value := range updates {
		switch field {
		case "branch":
			if v, ok := value.(string); ok {
				worktree.Branch = v
			}
		case "source_branch":
			if v, ok := value.(string); ok {
				worktree.SourceBranch = v
			}
		case "commit_hash":
			if v, ok := value.(string); ok {
				worktree.CommitHash = v
			}
		case "commit_count":
			if v, ok := value.(int); ok {
				worktree.CommitCount = v
			}
		case "commits_behind":
			if v, ok := value.(int); ok {
				worktree.CommitsBehind = v
			}
		case "is_dirty":
			if v, ok := value.(bool); ok {
				worktree.IsDirty = v
			}
		case "has_conflicts":
			if v, ok := value.(bool); ok {
				worktree.HasConflicts = v
			}
		case "pull_request_url":
			if v, ok := value.(string); ok {
				worktree.PullRequestURL = v
			}
		case "pull_request_title":
			if v, ok := value.(string); ok {
				worktree.PullRequestTitle = v
			}
		case "pull_request_body":
			if v, ok := value.(string); ok {
				worktree.PullRequestBody = v
			}
		case "session_title":
			if v, ok := value.(*models.TitleEntry); ok {
				worktree.SessionTitle = v
			}
		case "session_title_history":
			if v, ok := value.([]models.TitleEntry); ok {
				worktree.SessionTitleHistory = v
			}
		case "has_active_claude_session":
			if v, ok := value.(bool); ok {
				worktree.HasActiveClaudeSession = v
			}
		case "claude_activity_state":
			if v, ok := value.(models.ClaudeActivityState); ok {
				worktree.ClaudeActivityState = v
			}
		case "last_accessed":
			if v, ok := value.(time.Time); ok {
				worktree.LastAccessed = v
			}
		case "todos":
			if v, ok := value.([]models.Todo); ok {
				worktree.Todos = v
			}
		case "has_been_renamed":
			if v, ok := value.(bool); ok {
				worktree.HasBeenRenamed = v
			}
		}
	}

	// Save state
	if err := wsm.saveStateInternal(); err != nil {
		return err
	}

	// Emit update event with only changed fields
	if wsm.eventsEmitter != nil {
		wsm.eventsEmitter.EmitWorktreeUpdated(worktreeID, updates)

		// Emit specific todos event if todos were updated
		if todosValue, hasTodos := updates["todos"]; hasTodos {
			if todos, ok := todosValue.([]models.Todo); ok {
				wsm.eventsEmitter.EmitWorktreeTodosUpdated(worktreeID, todos)
			}
		}
	}

	return nil
}

// UpdateWorktreeStatus updates status fields from cache
func (wsm *WorktreeStateManager) UpdateWorktreeStatus(worktreeID string, status *CachedWorktreeStatus) error {
	updates := make(map[string]interface{})

	// Convert cached status to update map
	if status.IsDirty != nil {
		updates["is_dirty"] = *status.IsDirty
	}
	if status.HasConflicts != nil {
		updates["has_conflicts"] = *status.HasConflicts
	}
	if status.CommitHash != "" {
		updates["commit_hash"] = status.CommitHash
	}
	if status.CommitCount != nil {
		updates["commit_count"] = *status.CommitCount
	}
	if status.CommitsBehind != nil {
		updates["commits_behind"] = *status.CommitsBehind
	}
	if status.Branch != "" {
		updates["branch"] = status.Branch
	}

	return wsm.UpdateWorktree(worktreeID, updates)
}

// DeleteWorktree removes a worktree
func (wsm *WorktreeStateManager) DeleteWorktree(worktreeID string) error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	worktree, exists := wsm.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Delete from state
	delete(wsm.worktrees, worktreeID)
	delete(wsm.previousState, worktreeID)

	// Save state
	if err := wsm.saveStateInternal(); err != nil {
		return err
	}

	// Emit deleted event
	if wsm.eventsEmitter != nil {
		wsm.eventsEmitter.EmitWorktreeDeleted(worktreeID, worktree.Name)
	}

	return nil
}

// BatchUpdateWorktrees applies updates to multiple worktrees at once
func (wsm *WorktreeStateManager) BatchUpdateWorktrees(updates map[string]map[string]interface{}) error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	// Apply all updates
	for worktreeID, worktreeUpdates := range updates {
		worktree, exists := wsm.worktrees[worktreeID]
		if !exists {
			continue
		}

		// Apply updates to this worktree
		for field, value := range worktreeUpdates {
			switch field {
			case "branch":
				if v, ok := value.(string); ok {
					worktree.Branch = v
				}
			case "commit_hash":
				if v, ok := value.(string); ok {
					worktree.CommitHash = v
				}
			case "commit_count":
				if v, ok := value.(int); ok {
					worktree.CommitCount = v
				}
			case "commits_behind":
				if v, ok := value.(int); ok {
					worktree.CommitsBehind = v
				}
			case "is_dirty":
				if v, ok := value.(bool); ok {
					worktree.IsDirty = v
				}
			case "has_conflicts":
				if v, ok := value.(bool); ok {
					worktree.HasConflicts = v
				}
			case "has_active_claude_session":
				if v, ok := value.(bool); ok {
					worktree.HasActiveClaudeSession = v
				}
			case "claude_activity_state":
				if v, ok := value.(models.ClaudeActivityState); ok {
					worktree.ClaudeActivityState = v
				}
			}
		}
	}

	// Save state once
	if err := wsm.saveStateInternal(); err != nil {
		return err
	}

	// Emit events
	if wsm.eventsEmitter != nil {
		// For git status updates, emit batch update
		cachedUpdates := make(map[string]*CachedWorktreeStatus)
		hasGitStatusUpdates := false

		for worktreeID, worktreeUpdates := range updates {
			cached := &CachedWorktreeStatus{
				WorktreeID:  worktreeID,
				LastUpdated: time.Now(),
			}

			// Convert updates to cached format
			if v, ok := worktreeUpdates["is_dirty"].(bool); ok {
				cached.IsDirty = &v
				hasGitStatusUpdates = true
			}
			if v, ok := worktreeUpdates["has_conflicts"].(bool); ok {
				cached.HasConflicts = &v
				hasGitStatusUpdates = true
			}
			if v, ok := worktreeUpdates["commit_hash"].(string); ok {
				cached.CommitHash = v
				hasGitStatusUpdates = true
			}
			if v, ok := worktreeUpdates["commit_count"].(int); ok {
				cached.CommitCount = &v
				hasGitStatusUpdates = true
			}
			if v, ok := worktreeUpdates["commits_behind"].(int); ok {
				cached.CommitsBehind = &v
				hasGitStatusUpdates = true
			}
			if v, ok := worktreeUpdates["branch"].(string); ok {
				cached.Branch = v
				hasGitStatusUpdates = true
			}

			cachedUpdates[worktreeID] = cached

			// For Claude activity state changes, emit individual worktree update events
			// This ensures the frontend receives proper SSE events with all field changes
			wsm.eventsEmitter.EmitWorktreeUpdated(worktreeID, worktreeUpdates)
		}

		// Only emit batch update if there were git status changes
		if hasGitStatusUpdates {
			wsm.eventsEmitter.EmitWorktreeBatchUpdated(cachedUpdates)
		}
	}

	return nil
}

// saveStateInternal saves state to disk (must be called with lock held)
func (wsm *WorktreeStateManager) saveStateInternal() error {
	state := map[string]interface{}{
		"repositories": wsm.repositories,
		"worktrees":    wsm.worktrees,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	stateFile := filepath.Join(wsm.stateDir, "state.json")
	if err := os.MkdirAll(wsm.stateDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(stateFile, data, 0644)
}

// loadState loads state from disk
func (wsm *WorktreeStateManager) loadState() error {
	stateFile := filepath.Join(wsm.stateDir, "state.json")
	data, err := os.ReadFile(stateFile)
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

	// Load repositories
	if reposData, exists := state["repositories"]; exists {
		var repos map[string]*models.Repository
		if err := json.Unmarshal(reposData, &repos); err == nil {
			wsm.repositories = repos
		}
	}

	// Load worktrees
	if worktreesData, exists := state["worktrees"]; exists {
		var worktrees map[string]*models.Worktree
		if err := json.Unmarshal(worktreesData, &worktrees); err == nil {
			wsm.worktrees = worktrees

			// Initialize previous state for change detection
			for id, wt := range worktrees {
				wsm.previousState[id] = wsm.captureFieldState(wt)
			}
		}
	}

	return nil
}

// RestoreState recreates worktrees from persisted state on boot
func (wsm *WorktreeStateManager) RestoreState() error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	if wsm.worktreeRestorer == nil {
		logger.Warn("⚠️ No worktree restorer set, skipping state restoration")
		return nil
	}

	logger.Debug("🔄 Starting state restoration...")

	// First, check repository availability
	for repoID, repo := range wsm.repositories {
		// Use the actual repo Path from the repository struct
		// This should already contain the correct path (either /volume/repos/... or /live/...)
		repoPath := repo.Path

		if _, err := os.Stat(repoPath); err != nil {
			logger.Warnf("⚠️ Repository %s not available at %s, marking as unavailable", repoID, repoPath)
			repo.Available = false
		} else {
			logger.Debugf("✅ Repository %s found at %s", repoID, repoPath)
			repo.Available = true
		}
	}

	// Track restoration stats
	restoredCount := 0
	skippedCount := 0
	failedCount := 0

	// Attempt to restore worktrees
	for _, worktree := range wsm.worktrees {
		logger.Debugf("🔍 Processing worktree %s (RepoID: %s)", worktree.Name, worktree.RepoID)

		// Check if the associated repository is available
		repo, repoExists := wsm.repositories[worktree.RepoID]
		if !repoExists {
			logger.Warnf("⚠️ Worktree %s references missing repository %s, skipping", worktree.Name, worktree.RepoID)
			skippedCount++
			continue
		}

		if !repo.Available {
			logger.Warnf("⚠️ Worktree %s belongs to unavailable repository %s, skipping", worktree.Name, worktree.RepoID)
			skippedCount++
			continue
		}

		// Check if worktree directory still exists
		if _, err := os.Stat(worktree.Path); err == nil {
			logger.Debugf("✅ Worktree %s already exists at %s, no restoration needed", worktree.Name, worktree.Path)
			restoredCount++
			continue
		}

		logger.Debugf("🔄 Attempting to restore worktree %s to %s (repo path: %s)", worktree.Name, worktree.Path, repo.Path)

		// Add debug check for worktree restorer
		if wsm.worktreeRestorer == nil {
			logger.Errorf("❌ ERROR: worktreeRestorer is nil when trying to restore %s", worktree.Name)
			failedCount++
			continue
		}

		// Attempt to recreate the worktree
		logger.Debugf("🔧 Calling RecreateWorktree for %s", worktree.Name)
		if err := wsm.worktreeRestorer.RecreateWorktree(worktree, repo); err != nil {
			logger.Errorf("❌ Failed to restore worktree %s: %v", worktree.Name, err)
			failedCount++
			continue
		}

		logger.Debugf("✅ Successfully restored worktree %s", worktree.Name)
		restoredCount++
	}

	// Save state to persist any availability changes
	if err := wsm.saveStateInternal(); err != nil {
		logger.Warnf("⚠️ Failed to save state after restoration: %v", err)
	}

	logger.Infof("🎉 State restoration completed: %d restored, %d skipped, %d failed",
		restoredCount, skippedCount, failedCount)

	return nil
}

// captureFieldState captures the current state of worktree fields
func (wsm *WorktreeStateManager) captureFieldState(wt *models.Worktree) worktreeFieldState {
	state := worktreeFieldState{
		ID:                     wt.ID,
		Name:                   wt.Name,
		Branch:                 wt.Branch,
		SourceBranch:           wt.SourceBranch,
		CommitHash:             wt.CommitHash,
		CommitCount:            wt.CommitCount,
		CommitsBehind:          wt.CommitsBehind,
		IsDirty:                wt.IsDirty,
		HasConflicts:           wt.HasConflicts,
		PullRequestURL:         wt.PullRequestURL,
		SessionTitle:           wt.SessionTitle,
		HasActiveClaudeSession: wt.HasActiveClaudeSession,
		ClaudeActivityState:    wt.ClaudeActivityState,
		HasBeenRenamed:         wt.HasBeenRenamed,
	}

	// Deep copy title history
	if wt.SessionTitleHistory != nil {
		state.SessionTitleHistory = make([]models.TitleEntry, len(wt.SessionTitleHistory))
		copy(state.SessionTitleHistory, wt.SessionTitleHistory)
	}

	// Deep copy todos
	if wt.Todos != nil {
		state.Todos = make([]models.Todo, len(wt.Todos))
		copy(state.Todos, wt.Todos)
	}

	return state
}

// SetEventsEmitter connects the state manager to an events emitter
func (wsm *WorktreeStateManager) SetEventsEmitter(emitter EventsEmitter) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	wsm.eventsEmitter = emitter
}

// startClaudeActivitySync periodically checks and updates Claude activity states
func (wsm *WorktreeStateManager) startClaudeActivitySync() {
	logger.Debug("🔄 Starting Claude activity state sync")
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-wsm.stopChan:
			logger.Debug("🛑 Stopping Claude activity state sync")
			return
		case <-ticker.C:
			wsm.syncClaudeActivityStates()
		}
	}
}

// TriggerClaudeActivitySync triggers an immediate Claude activity state sync
func (wsm *WorktreeStateManager) TriggerClaudeActivitySync() {
	wsm.syncClaudeActivityStates()
}

// syncClaudeActivityStates checks all worktrees for Claude activity state changes
func (wsm *WorktreeStateManager) syncClaudeActivityStates() {
	wsm.mu.RLock()
	sessionService := wsm.sessionService
	if sessionService == nil {
		wsm.mu.RUnlock()
		return
	}

	// Make a copy of worktrees to avoid holding lock during SessionService calls
	worktreeCopy := make(map[string]*models.Worktree)
	for id, wt := range wsm.worktrees {
		wtCopy := *wt
		worktreeCopy[id] = &wtCopy
	}
	wsm.mu.RUnlock()

	// Check each worktree for Claude activity state changes
	updates := make(map[string]map[string]interface{})

	for worktreeID, wt := range worktreeCopy {
		// Get current Claude activity state
		currentActivityState := sessionService.GetClaudeActivityState(wt.Path)

		// Check if activity state has changed
		if wt.ClaudeActivityState != currentActivityState {
			logger.Debugf("🔄 Claude activity state changed for %s: %s -> %s",
				wt.Name, wt.ClaudeActivityState, currentActivityState)

			if updates[worktreeID] == nil {
				updates[worktreeID] = make(map[string]interface{})
			}
			updates[worktreeID]["claude_activity_state"] = currentActivityState

			// Also update the backward compatibility field
			hasActiveSession := (currentActivityState == models.ClaudeActive || currentActivityState == models.ClaudeRunning)
			if wt.HasActiveClaudeSession != hasActiveSession {
				updates[worktreeID]["has_active_claude_session"] = hasActiveSession
			}
		}
	}

	// Apply any updates found
	if len(updates) > 0 {
		if err := wsm.BatchUpdateWorktrees(updates); err != nil {
			logger.Warnf("⚠️ Failed to update Claude activity states: %v", err)
		}
	}
}

// RenameWorktreeBranch is the centralized method for renaming catnip branches to nice names
// This is the ONLY place where branch renaming should happen
func (wsm *WorktreeStateManager) RenameWorktreeBranch(worktreeID, niceBranchName string, gitOperations GitOperations) error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	worktree, exists := wsm.worktrees[worktreeID]
	if !exists {
		return fmt.Errorf("worktree %s not found", worktreeID)
	}

	// Check if branch has already been renamed
	if worktree.HasBeenRenamed {
		logger.Debugf("🔍 Branch for worktree %s already renamed to %q, skipping", worktreeID, worktree.Branch)
		return nil
	}

	// Only rename catnip branches
	originalBranch := worktree.Branch
	if !strings.HasPrefix(originalBranch, "refs/catnip/") {
		logger.Debugf("🔍 Branch %s is not a catnip branch, skipping rename", originalBranch)
		return nil
	}

	logger.Debugf("🔄 Creating nice branch %s for %s", niceBranchName, originalBranch)

	// Create the nice branch using git operations (this can be done without holding the lock)
	currentCommit, err := gitOperations.GetCommitHash(worktree.Path, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current commit: %v", err)
	}

	if err := gitOperations.CreateBranch(worktree.Path, niceBranchName, currentCommit); err != nil {
		return fmt.Errorf("failed to create nice branch %q: %v", niceBranchName, err)
	}

	// Store the branch mapping in git config for external tools (PRs, etc)
	configKey := fmt.Sprintf("catnip.branch-map.%s", strings.ReplaceAll(originalBranch, "/", "."))
	if err := gitOperations.SetConfig(worktree.Path, configKey, niceBranchName); err != nil {
		logger.Warnf("⚠️ Failed to store branch mapping in git config: %v", err)
		// Don't fail the operation for this
	}

	// Update the worktree state:
	// - Branch field shows the nice name for UI display
	// - The actual git HEAD stays on the catnip ref
	// - has_been_renamed prevents future rename attempts
	logger.Debugf("🔄 Updating worktree state: Branch %s -> %s (git HEAD stays on %s)", worktree.Branch, niceBranchName, originalBranch)
	worktree.Branch = niceBranchName // This is what the UI displays
	worktree.HasBeenRenamed = true   // This prevents further renames

	// Save state directly
	if err := wsm.saveStateInternal(); err != nil {
		return fmt.Errorf("failed to save worktree state: %v", err)
	}

	// Emit events manually since we bypassed UpdateWorktree
	if wsm.eventsEmitter != nil {
		updates := map[string]interface{}{
			"branch":           niceBranchName,
			"has_been_renamed": true,
		}
		// No need to filter here since we're explicitly setting the nice branch name
		wsm.eventsEmitter.EmitWorktreeUpdated(worktreeID, updates)
	}

	logger.Infof("✅ Successfully renamed branch display: %s -> %q for worktree %s (git HEAD remains on %s)",
		originalBranch, niceBranchName, worktreeID, originalBranch)
	return nil
}

// ShouldRenameBranch checks if a worktree branch should be renamed (centralized check)
func (wsm *WorktreeStateManager) ShouldRenameBranch(worktreeID string) bool {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()

	worktree, exists := wsm.worktrees[worktreeID]
	if !exists {
		return false
	}

	// Don't rename if already renamed
	if worktree.HasBeenRenamed {
		logger.Debugf("🔍 ShouldRenameBranch: %s already renamed (has_been_renamed=true)", worktreeID)
		return false
	}

	// Check if this is a catnip branch that needs renaming
	// After renaming, Branch field shows nice name, so we need to check git HEAD directly
	if strings.HasPrefix(worktree.Branch, "refs/catnip/") {
		logger.Debugf("🔍 ShouldRenameBranch: %s is catnip branch %s, should rename", worktreeID, worktree.Branch)
		return true
	}

	// If Branch field doesn't start with refs/catnip/, we still need to check git HEAD
	// in case the worktree was already renamed but git HEAD is still on catnip ref
	logger.Debugf("🔍 ShouldRenameBranch: %s Branch=%s (not catnip format), checking if already processed", worktreeID, worktree.Branch)
	return false
}
