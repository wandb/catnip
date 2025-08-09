package git

import (
	"fmt"

	"github.com/vanpelt/catnip/internal/git/executor"
	"github.com/vanpelt/catnip/internal/logger"
)

// URLManager handles remote URL operations with conversion and restoration
type URLManager struct {
	executor         executor.CommandExecutor
	originalURLCache map[string]string // Cache original URLs by worktree-remote key
}

// NewURLManager creates a new URL manager
func NewURLManager(executor executor.CommandExecutor) *URLManager {
	return &URLManager{
		executor:         executor,
		originalURLCache: make(map[string]string),
	}
}

// SetupRemoteURL sets up or updates the remote URL, optionally converting SSH to HTTPS
func (m *URLManager) SetupRemoteURL(worktreePath, remoteName, targetURL string) error {
	if remoteName == "" {
		remoteName = "origin"
	}

	cacheKey := fmt.Sprintf("%s:%s", worktreePath, remoteName)

	// Store original URL for restoration if not already cached
	if _, exists := m.originalURLCache[cacheKey]; !exists {
		branchOps := NewBranchOperations(m.executor)
		if originalURL, err := branchOps.GetRemoteURL(worktreePath); err == nil {
			m.originalURLCache[cacheKey] = originalURL
		}
	}

	// Set the new remote URL
	_, err := m.executor.ExecuteGitWithWorkingDir(worktreePath, "remote", "set-url", remoteName, targetURL)
	if err != nil {
		return fmt.Errorf("failed to set remote URL: %v", err)
	}

	logger.Debugf("🔗 Updated remote %s URL to: %s", remoteName, targetURL)
	return nil
}

// RestoreOriginalURL restores the original remote URL if it was changed
func (m *URLManager) RestoreOriginalURL(worktreePath, remoteName string) error {
	if remoteName == "" {
		remoteName = "origin"
	}

	cacheKey := fmt.Sprintf("%s:%s", worktreePath, remoteName)
	originalURL, exists := m.originalURLCache[cacheKey]

	if !exists || originalURL == "" {
		return nil // No original URL to restore
	}

	_, err := m.executor.ExecuteGitWithWorkingDir(worktreePath, "remote", "set-url", remoteName, originalURL)
	if err != nil {
		logger.Debugf("⚠️ Failed to restore original remote URL %s: %v", originalURL, err)
		return err
	}

	logger.Debugf("✅ Restored original remote URL: %s", originalURL)

	// Clear from cache after successful restoration
	delete(m.originalURLCache, cacheKey)

	return nil
}

// GetCurrentRemoteURL gets the current remote URL
func (m *URLManager) GetCurrentRemoteURL(worktreePath, remoteName string) (string, error) {
	// Note: remoteName parameter currently unused, but kept for API consistency
	// All operations use "origin" by default through GetRemoteURL
	_ = remoteName // Explicitly mark as unused

	branchOps := NewBranchOperations(m.executor)
	return branchOps.GetRemoteURL(worktreePath)
}
