package tui

import (
	"context"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/macos"
	"github.com/vanpelt/catnip/internal/models"
)

// HostPowerManager manages macOS power assertions based on Claude session activity
// This runs on the host machine, not in the container
type HostPowerManager struct {
	powerAssertion  *macos.PowerAssertion
	worktreeStates  map[string]models.ClaudeActivityState // worktree path -> activity state
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	deadManInterval time.Duration
	maxAssertionAge time.Duration
	assertionStart  time.Time
}

// NewHostPowerManager creates a new power manager for the host
func NewHostPowerManager() *HostPowerManager {
	ctx, cancel := context.WithCancel(context.Background())

	pm := &HostPowerManager{
		worktreeStates:  make(map[string]models.ClaudeActivityState),
		ctx:             ctx,
		cancel:          cancel,
		deadManInterval: 30 * time.Second, // Check every 30 seconds
		maxAssertionAge: 15 * time.Minute, // Maximum 15 minutes of continuous assertion
	}

	// Start monitoring goroutine
	go pm.monitor()

	debugLog("🔋 Host power manager initialized (check: %v, max age: %v)",
		pm.deadManInterval, pm.maxAssertionAge)

	return pm
}

// UpdateWorktreeState updates the activity state for a worktree
func (pm *HostPowerManager) UpdateWorktreeState(worktreePath string, state models.ClaudeActivityState) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	oldState, existed := pm.worktreeStates[worktreePath]
	pm.worktreeStates[worktreePath] = state

	if !existed || oldState != state {
		debugLog("🔋 Worktree state updated: %s -> %s", worktreePath, state)
		// Immediately check if we need to update assertion
		go pm.checkAndUpdateAssertion()
	}
}

// UpdateWorktreeBatch updates multiple worktree states at once
func (pm *HostPowerManager) UpdateWorktreeBatch(worktrees []WorktreeInfo) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	changed := false
	for _, wt := range worktrees {
		oldState, existed := pm.worktreeStates[wt.Path]
		if !existed || oldState != wt.ClaudeActivityState {
			pm.worktreeStates[wt.Path] = wt.ClaudeActivityState
			changed = true
			debugLog("🔋 Worktree state updated: %s -> %s", wt.Path, wt.ClaudeActivityState)
		}
	}

	if changed {
		// Immediately check if we need to update assertion
		go pm.checkAndUpdateAssertion()
	}
}

// Shutdown gracefully shuts down the power manager and releases any active assertions
func (pm *HostPowerManager) Shutdown() {
	pm.cancel()
	pm.releaseAssertion("shutdown")
	debugLog("🔋 Host power manager shutdown complete")
}

// monitor runs the main monitoring loop
func (pm *HostPowerManager) monitor() {
	ticker := time.NewTicker(pm.deadManInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.checkAndUpdateAssertion()
		}
	}
}

// checkAndUpdateAssertion checks current session states and updates power assertion accordingly
func (pm *HostPowerManager) checkAndUpdateAssertion() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	shouldAssert, activeWorkspaces := pm.shouldMaintainAssertion()
	currentlyAsserted := pm.powerAssertion != nil && pm.powerAssertion.IsActive()

	// Dead man switch: Release assertion if it's been active too long
	if currentlyAsserted && pm.assertionExceedsMaxAge() {
		logger.Warnf("🔋 Dead man switch triggered: power assertion exceeded max age (%v), releasing", pm.maxAssertionAge)
		pm.releaseAssertion("dead man switch")
		return
	}

	if shouldAssert && !currentlyAsserted {
		pm.createAssertion(activeWorkspaces)
	} else if !shouldAssert && currentlyAsserted {
		pm.releaseAssertion("no active Claude sessions")
	} else if currentlyAsserted {
		// Log periodic status while assertion is active
		age := time.Since(pm.assertionStart)
		debugLog("🔋 Power assertion active for %v (workspaces: %v)", age.Round(time.Second), activeWorkspaces)
	}
}

// shouldMaintainAssertion checks if any Claude sessions are in active state
func (pm *HostPowerManager) shouldMaintainAssertion() (bool, []string) {
	var activeWorkspaces []string

	for path, state := range pm.worktreeStates {
		// Keep machine awake only for Active state (recent activity <2 min)
		// Running state (PTY exists but no recent activity) allows machine to sleep
		if state == models.ClaudeActive {
			activeWorkspaces = append(activeWorkspaces, path)
		}
	}

	return len(activeWorkspaces) > 0, activeWorkspaces
}

// createAssertion creates a new power assertion
func (pm *HostPowerManager) createAssertion(activeWorkspaces []string) {
	if pm.powerAssertion != nil && pm.powerAssertion.IsActive() {
		return // Already have an active assertion
	}

	reason := "Catnip: Claude Code sessions active"
	if len(activeWorkspaces) == 1 {
		reason = "Catnip: Claude Code session active"
	}

	assertion, err := macos.NewPowerAssertion(reason)
	if err != nil {
		logger.Errorf("🔋 Failed to create power assertion: %v", err)
		return
	}

	pm.powerAssertion = assertion
	pm.assertionStart = time.Now()

	logger.Infof("🔋 Created power assertion for %d active Claude workspace(s): %v",
		len(activeWorkspaces), activeWorkspaces)
}

// releaseAssertion releases the current power assertion
func (pm *HostPowerManager) releaseAssertion(reason string) {
	if pm.powerAssertion == nil {
		return
	}

	err := pm.powerAssertion.Release()
	if err != nil {
		logger.Errorf("🔋 Failed to release power assertion: %v", err)
	} else {
		duration := time.Since(pm.assertionStart)
		logger.Infof("🔋 Released power assertion after %v (reason: %s)",
			duration.Round(time.Second), reason)
	}

	pm.powerAssertion = nil
	pm.assertionStart = time.Time{}
}

// assertionExceedsMaxAge checks if the current assertion has been active too long
func (pm *HostPowerManager) assertionExceedsMaxAge() bool {
	if pm.assertionStart.IsZero() {
		return false
	}
	return time.Since(pm.assertionStart) > pm.maxAssertionAge
}

// WorktreeInfo contains basic worktree information needed for power management
type WorktreeInfo struct {
	Path                string
	ClaudeActivityState models.ClaudeActivityState
}
