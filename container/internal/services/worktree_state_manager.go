package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// WorktreeStateChange represents a change to worktree state
type WorktreeStateChange struct {
	Type       string // "created", "updated", "deleted"
	WorktreeID string
	Fields     map[string]interface{} // Changed fields for updates
	Worktree   *models.Worktree       // Full worktree for creates
}

// WorktreeStateManager manages all worktree state persistently
type WorktreeStateManager struct {
	mu            sync.RWMutex
	repositories  map[string]*models.Repository
	worktrees     map[string]*models.Worktree
	stateDir      string
	eventsEmitter EventsEmitter

	// Track field-level changes
	previousState map[string]worktreeFieldState
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
}

// NewWorktreeStateManager creates a new centralized state manager
func NewWorktreeStateManager(stateDir string, eventsEmitter EventsEmitter) *WorktreeStateManager {
	wsm := &WorktreeStateManager{
		repositories:  make(map[string]*models.Repository),
		worktrees:     make(map[string]*models.Worktree),
		stateDir:      stateDir,
		eventsEmitter: eventsEmitter,
		previousState: make(map[string]worktreeFieldState),
	}

	// Load existing state
	if err := wsm.loadState(); err != nil {
		log.Printf("⚠️ Failed to load state: %v", err)
	}

	return wsm
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

	wsm.repositories[repo.ID] = repo
	return wsm.saveStateInternal()
}

// AddWorktree adds a new worktree
func (wsm *WorktreeStateManager) AddWorktree(worktree *models.Worktree) error {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

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
		case "last_accessed":
			if v, ok := value.(time.Time); ok {
				worktree.LastAccessed = v
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
			}
		}
	}

	// Save state once
	if err := wsm.saveStateInternal(); err != nil {
		return err
	}

	// Emit batch update event
	if wsm.eventsEmitter != nil {
		// Convert to cached status format for compatibility
		cachedUpdates := make(map[string]*CachedWorktreeStatus)
		for worktreeID, worktreeUpdates := range updates {
			cached := &CachedWorktreeStatus{
				WorktreeID:  worktreeID,
				LastUpdated: time.Now(),
			}

			// Convert updates to cached format
			if v, ok := worktreeUpdates["is_dirty"].(bool); ok {
				cached.IsDirty = &v
			}
			if v, ok := worktreeUpdates["has_conflicts"].(bool); ok {
				cached.HasConflicts = &v
			}
			if v, ok := worktreeUpdates["commit_hash"].(string); ok {
				cached.CommitHash = v
			}
			if v, ok := worktreeUpdates["commit_count"].(int); ok {
				cached.CommitCount = &v
			}
			if v, ok := worktreeUpdates["commits_behind"].(int); ok {
				cached.CommitsBehind = &v
			}
			if v, ok := worktreeUpdates["branch"].(string); ok {
				cached.Branch = v
			}

			cachedUpdates[worktreeID] = cached
		}

		wsm.eventsEmitter.EmitWorktreeBatchUpdated(cachedUpdates)
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
	}

	// Deep copy title history
	if wt.SessionTitleHistory != nil {
		state.SessionTitleHistory = make([]models.TitleEntry, len(wt.SessionTitleHistory))
		copy(state.SessionTitleHistory, wt.SessionTitleHistory)
	}

	return state
}

// SetEventsEmitter connects the state manager to an events emitter
func (wsm *WorktreeStateManager) SetEventsEmitter(emitter EventsEmitter) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	wsm.eventsEmitter = emitter
}
