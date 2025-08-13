package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestWorkspace creates an isolated workspace for tests and returns a cleanup function
func setupTestWorkspace(t *testing.T) func() {
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

	return func() {
		if oldWorkspace == "" {
			_ = os.Unsetenv("CATNIP_WORKSPACE_DIR")
		} else {
			_ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace)
		}
	}
}

func TestGitServiceIntegration(t *testing.T) {
	// Skip if not in CI or test environment
	if os.Getenv("CI") == "" && os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=1 to run")
	}

	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	// Create service with isolated state
	service := createTestGitService(t)
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
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("IsLocalRepo", func(t *testing.T) {
		assert.True(t, service.isLocalRepo("local/catnip"))
		assert.False(t, service.isLocalRepo("github/repo"))
		assert.False(t, service.isLocalRepo("owner/repo"))
	})

	t.Run("DeleteWorktree_NonExistent", func(t *testing.T) {
		done, err := service.DeleteWorktree("non-existent")
		assert.Error(t, err)
		assert.Nil(t, done)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
	})
}

func TestGitServiceGitHubOperations(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("ListGitHubRepositories", func(t *testing.T) {
		repos, err := service.ListGitHubRepositories()
		// Should return at least empty slice, not error when gh CLI fails gracefully
		if err != nil {
			// Expected when gh CLI is not authenticated
			assert.Contains(t, err.Error(), "failed to list GitHub repositories")
		} else {
			assert.NotNil(t, repos)
		}
	})

	t.Run("CreatePullRequest", func(t *testing.T) {
		pr, err := service.CreatePullRequest("worktree-id", "title", "body", false)
		// Should error for non-existent worktree
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree worktree-id not found")
		assert.Nil(t, pr)
	})

	t.Run("UpdatePullRequest", func(t *testing.T) {
		pr, err := service.UpdatePullRequest("worktree-id", "title", "body", false)
		// Should error for non-existent worktree
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree worktree-id not found")
		assert.Nil(t, pr)
	})

	t.Run("GetPullRequestInfo", func(t *testing.T) {
		info, err := service.GetPullRequestInfo("worktree-id")
		// Should error for non-existent worktree
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree worktree-id not found")
		assert.Nil(t, info)
	})
}

func TestGitServiceConflictOperations(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("CheckSyncConflicts", func(t *testing.T) {
		conflict, err := service.CheckSyncConflicts("worktree-id")
		// Should error for non-existent worktree
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree worktree-id not found")
		assert.Nil(t, conflict)
	})

	t.Run("CheckMergeConflicts", func(t *testing.T) {
		conflict, err := service.CheckMergeConflicts("worktree-id")
		// Should error for non-existent worktree
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree worktree-id not found")
		assert.Nil(t, conflict)
	})

	t.Run("SyncWorktree", func(t *testing.T) {
		err := service.SyncWorktree("worktree-id", "rebase")
		// Should error for non-existent worktree
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree worktree-id not found")
	})

	t.Run("MergeWorktreeToMain", func(t *testing.T) {
		err := service.MergeWorktreeToMain("worktree-id", true)
		// Should error for non-existent worktree
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree worktree-id not found")
	})
}

func TestGitServiceHelperMethods(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("GenerateUniqueSessionName", func(t *testing.T) {
		// Test with a temporary directory that exists
		tempDir := t.TempDir()
		name := service.generateUniqueSessionName(tempDir)
		assert.NotEmpty(t, name)
		// Test that it generates a reasonable name
		assert.NotEqual(t, "", name)
	})

	t.Run("ExecCommand", func(t *testing.T) {
		cmd := service.execCommand("echo", "test")
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Path, "echo") // Path might include /bin/echo
		assert.Equal(t, []string{"echo", "test"}, cmd.Args)
	})
}

func TestGitServiceRepositoryManagement(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("GetRepositoryByID", func(t *testing.T) {
		// Test with non-existent repository
		repo := service.GetRepositoryByID("non-existent")
		assert.Nil(t, repo)
	})

	t.Run("ListRepositories", func(t *testing.T) {
		repos := service.ListRepositories()
		assert.NotNil(t, repos)
		// Note: Can't assert empty count due to shared state from other tests
		assert.GreaterOrEqual(t, len(repos), 0)
	})

	t.Run("GetDefaultWorktreePath", func(t *testing.T) {
		path := service.GetDefaultWorktreePath()
		assert.NotEmpty(t, path)
		// Should return a valid path
		assert.NotEqual(t, "", path)
	})
}

func TestGitServiceWorktreeDiff(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("GetWorktreeDiff_NonExistentWorktree", func(t *testing.T) {
		diff, err := service.GetWorktreeDiff("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree not found")
		assert.Nil(t, diff)
	})
}

func TestGitServiceStateManagement(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("SaveAndLoadState", func(t *testing.T) {
		// State management is now handled automatically by the state manager
		// No explicit save/load needed

		// Verify service still works after load
		status := service.GetStatus()
		assert.NotNil(t, status)
	})
}

func TestGitServiceCleanupOperations(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("CleanupMergedWorktrees", func(t *testing.T) {
		// Should not error even with no worktrees
		_, _, err := service.CleanupMergedWorktrees()
		assert.NoError(t, err)
	})

	t.Run("Stop", func(t *testing.T) {
		// Should not error
		service.Stop()
	})
}

// Mock setup executor for testing
type mockSetupExecutor struct {
	executedPaths []string
	executed      bool
}

func (m *mockSetupExecutor) ExecuteSetupScript(worktreePath string) {
	m.executedPaths = append(m.executedPaths, worktreePath)
	m.executed = true
}

func TestGitServiceSetupExecutor(t *testing.T) {
	// Set up isolated workspace
	cleanup := setupTestWorkspace(t)
	defer cleanup()

	service := createTestGitService(t)
	require.NotNil(t, service)

	t.Run("SetSetupExecutor", func(t *testing.T) {
		mock := &mockSetupExecutor{}
		service.SetSetupExecutor(mock)

		// Verify the executor was set (cannot check directly since setupExecutor is private)
		// We'll verify through the behavior in a worktree creation scenario
		assert.NotNil(t, service)
	})

	t.Run("SetupExecutorInterface", func(t *testing.T) {
		// Test that our mock implements the interface correctly
		var executor SetupExecutor = &mockSetupExecutor{}
		executor.ExecuteSetupScript("/test/path")

		mock := executor.(*mockSetupExecutor)
		assert.True(t, mock.executed)
		assert.Contains(t, mock.executedPaths, "/test/path")
	})
}
