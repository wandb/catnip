package services

import (
	"testing"

	"github.com/vanpelt/catnip/internal/git"
)

// createTestGitService creates a GitService with isolated state for testing
func createTestGitService(t *testing.T) *GitService {
	stateDir := t.TempDir()
	return NewGitServiceWithStateDir(git.NewOperations(), stateDir)
}
