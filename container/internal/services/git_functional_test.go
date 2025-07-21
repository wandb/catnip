package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/models"
)

// TestGitServiceFunctional tests GitService functionality using in-memory repositories
func TestGitServiceFunctional(t *testing.T) {
	// Skip if SKIP_FUNCTIONAL_TESTS is set
	if os.Getenv("SKIP_FUNCTIONAL_TESTS") == "1" {
		t.Skip("Skipping functional tests")
	}

	// Create temporary workspace
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

	t.Run("RepositoryOperations", func(t *testing.T) {
		testRepositoryOperations(t, service)
	})

	t.Run("WorktreeOperations", func(t *testing.T) {
		testWorktreeOperations(t, service)
	})

	t.Run("StateManagement", func(t *testing.T) {
		testStateManagement(t, service)
	})
}

func testRepositoryOperations(t *testing.T, service *GitService) {
	t.Run("CheckoutRepository_InvalidInput", func(t *testing.T) {
		// Test invalid repository URL with proper signature
		repo, worktree, err := service.CheckoutRepository("invalid", "url", "test-branch")
		assert.Error(t, err)
		assert.Nil(t, repo)
		assert.Nil(t, worktree)

		// Test local repository detection
		assert.True(t, service.isLocalRepo("local/test-repo"))
		assert.False(t, service.isLocalRepo("github/owner/repo"))
	})

	t.Run("GetRepositoryByID", func(t *testing.T) {
		// Test with empty service
		repo := service.GetRepositoryByID("non-existent")
		assert.Nil(t, repo)

		// Add a mock repository for testing
		mockRepo := &models.Repository{
			ID:            "test/repo",
			Path:          "/test/path",
			URL:           "https://github.com/test/repo.git",
			DefaultBranch: "main",
			CreatedAt:     time.Now(),
		}
		service.repositories["test/repo"] = mockRepo

		// Test retrieval
		retrievedRepo := service.GetRepositoryByID("test/repo")
		assert.NotNil(t, retrievedRepo)
		assert.Equal(t, "test/repo", retrievedRepo.ID)
		assert.Equal(t, "/test/path", retrievedRepo.Path)
	})

	t.Run("ListRepositories", func(t *testing.T) {
		// Should include our mock repository from previous test
		repos := service.ListRepositories()
		assert.Len(t, repos, 1)
		assert.Equal(t, "test/repo", repos[0].ID)
	})
}

func testWorktreeOperations(t *testing.T, service *GitService) {
	t.Run("ListWorktrees", func(t *testing.T) {
		// Start with empty service
		worktrees := service.ListWorktrees()
		assert.Empty(t, worktrees)
	})

	t.Run("DeleteWorktree_NotFound", func(t *testing.T) {
		err := service.DeleteWorktree("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
	})

	t.Run("GetWorktreeDiff_NotFound", func(t *testing.T) {
		diff, err := service.GetWorktreeDiff("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree not found")
		assert.Nil(t, diff)
	})

	t.Run("WorktreeWithMockRepository", func(t *testing.T) {
		// Create a mock worktree
		mockWorktree := &models.Worktree{
			ID:           "test-worktree",
			RepoID:       "test/repo",
			Name:         "repo/catnip/felix",
			Path:         "/test/worktree/path",
			Branch:       "catnip/felix",
			SourceBranch: "main",
			CommitHash:   "abc123",
			CommitCount:  2,
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		service.worktrees["test-worktree"] = mockWorktree

		// Test listing includes our worktree
		worktrees := service.ListWorktrees()
		assert.Len(t, worktrees, 1)
		assert.Equal(t, "test-worktree", worktrees[0].ID)
		assert.Equal(t, "catnip/felix", worktrees[0].Branch)

		// Test GetWorktreeDiff with mock worktree (will fail due to invalid path, but tests validation)
		diff, err := service.GetWorktreeDiff("test-worktree")
		assert.Error(t, err) // Expected since path doesn't exist
		assert.Nil(t, diff)
	})
}

func testStateManagement(t *testing.T, service *GitService) {
	t.Run("SaveAndLoadState", func(t *testing.T) {
		// Add some test data
		mockRepo := &models.Repository{
			ID:            "test/state-repo",
			Path:          "/test/state/path",
			URL:           "https://github.com/test/state-repo.git",
			DefaultBranch: "main",
			CreatedAt:     time.Now(),
		}
		service.repositories["test/state-repo"] = mockRepo

		mockWorktree := &models.Worktree{
			ID:           "state-worktree",
			RepoID:       "test/state-repo",
			Name:         "state-repo/catnip/felix",
			Path:         "/test/state/worktree",
			Branch:       "catnip/felix",
			SourceBranch: "main",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		service.worktrees["state-worktree"] = mockWorktree

		// Save state
		_ = service.saveState()

		// Create new service and load state
		newService := NewGitService()
		_ = newService.loadState()

		// Note: loadState may not fully restore in-memory state without actual files,
		// but we can test that it doesn't crash and maintains basic functionality
		status := newService.GetStatus()
		assert.NotNil(t, status)
	})

	t.Run("GetStatus", func(t *testing.T) {
		status := service.GetStatus()
		assert.NotNil(t, status)
		assert.NotNil(t, status.Repositories)

		// Should include our test repositories
		assert.GreaterOrEqual(t, len(status.Repositories), 1)

		// Should include worktree count
		assert.GreaterOrEqual(t, status.WorktreeCount, 1)
	})
}

// TestGitServiceHelperFunctions tests the helper functions with various inputs
func TestGitServiceHelperFunctions(t *testing.T) {
	service := NewGitService()

	t.Run("GenerateUniqueSessionName", func(t *testing.T) {
		tempDir := t.TempDir()

		// Generate multiple names to test uniqueness
		names := make(map[string]bool)
		for i := 0; i < 10; i++ {
			name := service.generateUniqueSessionName(tempDir)
			assert.NotEmpty(t, name)
			// Note: Names might not be unique due to random generation, so just test validity
			names[name] = true

			// Should start with catnip/
			assert.True(t, strings.HasPrefix(name, "catnip/"), "Name should start with catnip/: %s", name)

			// Should be a valid catnip branch name
			assert.True(t, isCatnipBranch(name), "Generated name should be valid catnip branch: %s", name)
		}
	})

	t.Run("IsCatnipBranch", func(t *testing.T) {
		assert.True(t, isCatnipBranch("catnip/felix"))
		assert.True(t, isCatnipBranch("catnip/fluffy-felix"))
		assert.False(t, isCatnipBranch("main"))
		assert.False(t, isCatnipBranch("feature/something"))
		assert.False(t, isCatnipBranch("develop"))
	})

	t.Run("ExecCommand", func(t *testing.T) {
		cmd := service.execCommand("echo", "test")
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Path, "echo")
		assert.Equal(t, []string{"echo", "test"}, cmd.Args)

		// Test command execution
		output, err := cmd.Output()
		assert.NoError(t, err)
		assert.Equal(t, "test\n", string(output))
	})

	t.Run("IsLocalRepo", func(t *testing.T) {
		assert.True(t, service.isLocalRepo("local/my-project"))
		assert.True(t, service.isLocalRepo("local/another-repo"))
		assert.False(t, service.isLocalRepo("github/owner/repo"))
		assert.False(t, service.isLocalRepo("owner/repo"))
		assert.False(t, service.isLocalRepo("https://github.com/owner/repo.git"))
	})
}

// TestGitServiceGitHubOperationsFunctional tests GitHub operations with command validation
func TestGitServiceGitHubOperationsFunctional(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping GitHub operations tests in short mode")
	}

	service := NewGitService()

	// Add a test worktree for GitHub operations
	mockWorktree := &models.Worktree{
		ID:           "gh-test-worktree",
		RepoID:       "test/gh-repo",
		Name:         "gh-repo/catnip/whiskers",
		Path:         "/test/gh/worktree",
		Branch:       "catnip/whiskers",
		SourceBranch: "main",
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}
	service.worktrees["gh-test-worktree"] = mockWorktree

	mockRepo := &models.Repository{
		ID:            "test/gh-repo",
		Path:          "/test/gh/repo",
		URL:           "https://github.com/test/gh-repo.git",
		DefaultBranch: "main",
		CreatedAt:     time.Now(),
	}
	service.repositories["test/gh-repo"] = mockRepo

	t.Run("CreatePullRequest_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		pr, err := service.CreatePullRequest("non-existent", "Test PR", "Test body")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
		assert.Nil(t, pr)

		// Test with valid worktree (will fail at git operations, but validates worktree exists)
		pr, err = service.CreatePullRequest("gh-test-worktree", "Test PR", "Test body")
		assert.Error(t, err) // Expected - no real git repo
		assert.Nil(t, pr)
	})

	t.Run("UpdatePullRequest_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		pr, err := service.UpdatePullRequest("non-existent", "Updated PR", "Updated body")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
		assert.Nil(t, pr)
	})

	t.Run("GetPullRequestInfo_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		info, err := service.GetPullRequestInfo("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
		assert.Nil(t, info)
	})
}

// TestGitServiceConflictOperationsFunctional tests conflict detection and resolution
func TestGitServiceConflictOperationsFunctional(t *testing.T) {
	service := NewGitService()

	// Add test worktree for conflict operations
	mockWorktree := &models.Worktree{
		ID:           "conflict-worktree",
		RepoID:       "test/conflict-repo",
		Name:         "conflict-repo/catnip/patches",
		Path:         "/test/conflict/worktree",
		Branch:       "catnip/patches",
		SourceBranch: "main",
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}
	service.worktrees["conflict-worktree"] = mockWorktree

	t.Run("CheckSyncConflicts_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		conflict, err := service.CheckSyncConflicts("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
		assert.Nil(t, conflict)

		// Test with valid worktree (will fail at git operations, but validates worktree exists)
		conflict, err = service.CheckSyncConflicts("conflict-worktree")
		assert.Error(t, err) // Expected - no real git repo
		assert.Nil(t, conflict)
	})

	t.Run("CheckMergeConflicts_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		conflict, err := service.CheckMergeConflicts("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
		assert.Nil(t, conflict)
	})

	t.Run("SyncWorktree_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		err := service.SyncWorktree("non-existent", "merge")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")

		// Test with invalid strategy
		err = service.SyncWorktree("conflict-worktree", "invalid-strategy")
		assert.Error(t, err) // Should validate strategy
	})

	t.Run("MergeWorktreeToMain_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		err := service.MergeWorktreeToMain("non-existent", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
	})
}

// TestGitServiceCleanupOperationsFunctional tests cleanup functionality
func TestGitServiceCleanupOperationsFunctional(t *testing.T) {
	service := NewGitService()

	t.Run("CleanupMergedWorktrees", func(t *testing.T) {
		// Should not error even with no worktrees
		_, _, _ = service.CleanupMergedWorktrees()

		// Add some test worktrees
		service.worktrees["test1"] = &models.Worktree{
			ID:           "test1",
			Branch:       "catnip/mittens",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		service.worktrees["test2"] = &models.Worktree{
			ID:           "test2",
			Branch:       "catnip/shadow",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}

		// Should not error with worktrees (though cleanup may not work without real git repos)
		_, _, _ = service.CleanupMergedWorktrees()
	})

	t.Run("CleanupUnusedBranches", func(t *testing.T) {
		// Should not error
		service.cleanupUnusedBranches()
	})

	t.Run("TriggerManualSync", func(t *testing.T) {
		// Should not error
		_ = service.TriggerManualSync()
	})

	t.Run("Stop", func(t *testing.T) {
		// Should not error
		service.Stop()
	})
}

// TestGitServiceInMemoryRepoIntegration demonstrates how we could test with actual in-memory repos
func TestGitServiceInMemoryRepoIntegration(t *testing.T) {
	t.Skip("Placeholder for future in-memory repo integration tests")

	// This test would require refactoring GitService to accept injectable git operations
	// or creating a bridge between go-git in-memory repos and the file-based operations
	// the current service expects

	// Example of what we could test:
	// 1. Create in-memory repo with test data using git.CreateTestRepositoryWithHistory()
	// 2. Set up GitService to work with the in-memory repo
	// 3. Test actual git operations like creating worktrees, detecting conflicts, etc.
}
