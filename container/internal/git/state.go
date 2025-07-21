package git

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanpelt/catnip/internal/models"
)

// StateManager handles persistence of Git state
type StateManager struct {
	stateFile string
}

// NewStateManager creates a new state manager
func NewStateManager(stateFile string) *StateManager {
	return &StateManager{
		stateFile: stateFile,
	}
}

// SaveState saves the current state to disk
func (sm *StateManager) SaveState(repositories map[string]*models.Repository, worktrees map[string]*models.Worktree) error {
	state := models.GitState{
		Repositories: make(map[string]*models.Repository),
		Worktrees:    make(map[string]*models.Worktree),
	}

	// Copy repositories
	for k, v := range repositories {
		state.Repositories[k] = v
	}

	// Copy worktrees
	for k, v := range worktrees {
		state.Worktrees[k] = v
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(sm.stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(sm.stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadState loads the state from disk
func (sm *StateManager) LoadState() (map[string]*models.Repository, map[string]*models.Worktree, error) {
	// Check if file exists
	if _, err := os.Stat(sm.stateFile); os.IsNotExist(err) {
		// Return empty maps if file doesn't exist
		return make(map[string]*models.Repository), make(map[string]*models.Worktree), nil
	}

	// Read file
	data, err := os.ReadFile(sm.stateFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Unmarshal JSON
	var state models.GitState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// Ensure maps are not nil
	if state.Repositories == nil {
		state.Repositories = make(map[string]*models.Repository)
	}
	if state.Worktrees == nil {
		state.Worktrees = make(map[string]*models.Worktree)
	}

	return state.Repositories, state.Worktrees, nil
}
