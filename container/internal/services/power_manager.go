package services

import (
	"context"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/macos"
	"github.com/vanpelt/catnip/internal/models"
)

// PowerManager manages macOS power assertions based on Claude session activity
type PowerManager struct {
	sessionService  *SessionService
	powerAssertion  *macos.PowerAssertion
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	deadManInterval time.Duration
	maxAssertionAge time.Duration
	assertionStart  time.Time
}

// NewPowerManager creates a new power manager that monitors Claude session activity
func NewPowerManager(sessionService *SessionService) *PowerManager {
	ctx, cancel := context.WithCancel(context.Background())

	pm := &PowerManager{
		sessionService:  sessionService,
		ctx:             ctx,
		cancel:          cancel,
		deadManInterval: 30 * time.Second, // Check every 30 seconds
		maxAssertionAge: 15 * time.Minute, // Maximum 15 minutes of continuous assertion
	}

	// Start monitoring goroutine
	go pm.monitor()

	logger.Infof("🔋 Power manager initialized with dead man switch (check: %v, max age: %v)",
		pm.deadManInterval, pm.maxAssertionAge)

	return pm
}

// Shutdown gracefully shuts down the power manager and releases any active assertions
func (pm *PowerManager) Shutdown() {
	pm.cancel()
	pm.releaseAssertion("shutdown")
	logger.Infof("🔋 Power manager shutdown complete")
}

// monitor runs the main monitoring loop
func (pm *PowerManager) monitor() {
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
func (pm *PowerManager) checkAndUpdateAssertion() {
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
		logger.Debugf("🔋 Power assertion active for %v (workspaces: %v)", age.Round(time.Second), activeWorkspaces)
	}
}

// shouldMaintainAssertion checks if any Claude sessions are in active or running state
func (pm *PowerManager) shouldMaintainAssertion() (bool, []string) {
	var activeWorkspaces []string

	// Get all active sessions from the session service
	activeSessions := pm.sessionService.GetAllActiveSessions()

	for workspacePath := range activeSessions {
		activityState := pm.sessionService.GetClaudeActivityState(workspacePath)

		// Keep machine awake only for Active state (recent activity <2 min)
		// Running state (PTY exists but no recent activity) allows machine to sleep
		if activityState == models.ClaudeActive {
			activeWorkspaces = append(activeWorkspaces, workspacePath)
		}
	}

	return len(activeWorkspaces) > 0, activeWorkspaces
}

// createAssertion creates a new power assertion
func (pm *PowerManager) createAssertion(activeWorkspaces []string) {
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
func (pm *PowerManager) releaseAssertion(reason string) {
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
func (pm *PowerManager) assertionExceedsMaxAge() bool {
	if pm.assertionStart.IsZero() {
		return false
	}
	return time.Since(pm.assertionStart) > pm.maxAssertionAge
}

// GetStatus returns the current power manager status for debugging
func (pm *PowerManager) GetStatus() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	status := map[string]interface{}{
		"assertion_active":  pm.powerAssertion != nil && pm.powerAssertion.IsActive(),
		"dead_man_interval": pm.deadManInterval.String(),
		"max_assertion_age": pm.maxAssertionAge.String(),
	}

	if pm.powerAssertion != nil && pm.powerAssertion.IsActive() {
		status["assertion_reason"] = pm.powerAssertion.GetReason()
		status["assertion_age"] = time.Since(pm.assertionStart).Round(time.Second).String()
	}

	shouldAssert, activeWorkspaces := pm.shouldMaintainAssertion()
	status["should_assert"] = shouldAssert
	status["active_workspaces"] = activeWorkspaces

	return status
}
