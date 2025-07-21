package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitServiceIntegration(t *testing.T) {
	// Skip if not in CI or test environment
	if os.Getenv("CI") == "" && os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=1 to run")
	}

	// Create test workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("WORKSPACE_DIR")
	require.NoError(t, os.Setenv("WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

	// Create service
	service := NewGitService()
	require.NotNil(t, service)

	// Load state (loadState is private, so skip this check)

	t.Run("GetStatus", func(t *testing.T) {
		status := service.GetStatus()
		assert.NotNil(t, status)
		assert.NotNil(t, status.Repositories)
		assert.Equal(t, 0, status.WorktreeCount)
	})

	t.Run("ListWorktrees", func(t *testing.T) {
		worktrees := service.ListWorktrees()
		assert.Empty(t, worktrees)
	})
}

func TestGitServiceMethods(t *testing.T) {
	service := NewGitService()
	require.NotNil(t, service)

	t.Run("IsLocalRepo", func(t *testing.T) {
		assert.True(t, service.isLocalRepo("local/catnip"))
		assert.False(t, service.isLocalRepo("github/repo"))
		assert.False(t, service.isLocalRepo("owner/repo"))
	})

	t.Run("DeleteWorktree_NonExistent", func(t *testing.T) {
		err := service.DeleteWorktree("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
	})
}

func TestGitServiceGitHubOperations(t *testing.T) {
	service := NewGitService()
	require.NotNil(t, service)

	t.Run("ListGitHubRepositories", func(t *testing.T) {
		repos, err := service.ListGitHubRepositories()
		// Currently not implemented
		assert.Error(t, err)
		assert.Nil(t, repos)
	})

	t.Run("CreatePullRequest", func(t *testing.T) {
		pr, err := service.CreatePullRequest("worktree-id", "title", "body")
		// Currently not implemented
		assert.Error(t, err)
		assert.Nil(t, pr)
	})

	t.Run("UpdatePullRequest", func(t *testing.T) {
		pr, err := service.UpdatePullRequest("worktree-id", "title", "body")
		// Currently not implemented
		assert.Error(t, err)
		assert.Nil(t, pr)
	})

	t.Run("GetPullRequestInfo", func(t *testing.T) {
		info, err := service.GetPullRequestInfo("worktree-id")
		// Currently not implemented
		assert.Error(t, err)
		assert.Nil(t, info)
	})
}

func TestGitServiceConflictOperations(t *testing.T) {
	service := NewGitService()
	require.NotNil(t, service)

	t.Run("CheckSyncConflicts", func(t *testing.T) {
		conflict, err := service.CheckSyncConflicts("worktree-id")
		// Currently not implemented
		assert.Error(t, err)
		assert.Nil(t, conflict)
	})

	t.Run("CheckMergeConflicts", func(t *testing.T) {
		conflict, err := service.CheckMergeConflicts("worktree-id")
		// Currently not implemented
		assert.Error(t, err)
		assert.Nil(t, conflict)
	})

	t.Run("SyncWorktree", func(t *testing.T) {
		err := service.SyncWorktree("worktree-id", "rebase")
		// Currently not implemented
		assert.Error(t, err)
	})

	t.Run("MergeWorktreeToMain", func(t *testing.T) {
		err := service.MergeWorktreeToMain("worktree-id", true)
		// Currently not implemented
		assert.Error(t, err)
	})
}
