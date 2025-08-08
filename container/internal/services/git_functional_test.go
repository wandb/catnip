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
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace) }()

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
	// Create a temporary directory for test repositories
	tempDir := t.TempDir()
	testRepoPath := filepath.Join(tempDir, "test-repo")

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

		// Add a mock repository for testing using temporary path
		mockRepo := &models.Repository{
			ID:            "test/repo",
			Path:          testRepoPath,
			URL:           "https://github.com/test/repo.git",
			DefaultBranch: "main",
			CreatedAt:     time.Now(),
		}
		_ = service.stateManager.AddRepository(mockRepo)

		// Test retrieval
		retrievedRepo := service.GetRepositoryByID("test/repo")
		assert.NotNil(t, retrievedRepo)
		assert.Equal(t, "test/repo", retrievedRepo.ID)
		assert.Equal(t, testRepoPath, retrievedRepo.Path)
	})

	t.Run("ListRepositories", func(t *testing.T) {
		// Should include our mock repository from previous test
		repos := service.ListRepositories()
		assert.Greater(t, len(repos), 0)

		// Find our test repository
		var testRepo *models.Repository
		for _, repo := range repos {
			if repo.ID == "test/repo" {
				testRepo = repo
				break
			}
		}
		assert.NotNil(t, testRepo, "test/repo should be in the list")
		assert.Equal(t, "test/repo", testRepo.ID)
	})
}

func testWorktreeOperations(t *testing.T, service *GitService) {
	t.Run("ListWorktrees", func(t *testing.T) {
		// List worktrees (may not be empty due to shared state from other tests)
		worktrees := service.ListWorktrees()
		assert.NotNil(t, worktrees)
		// Note: Can't assert empty because state may persist from other tests
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
		// Create a unique temporary path for this worktree
		tempDir := t.TempDir()
		worktreePath := filepath.Join(tempDir, "test-worktree")

		// Create a mock worktree
		mockWorktree := &models.Worktree{
			ID:           "test-worktree",
			RepoID:       "test/repo",
			Name:         "repo/catnip/felix",
			Path:         worktreePath,
			Branch:       "catnip/felix",
			SourceBranch: "main",
			CommitHash:   "abc123",
			CommitCount:  2,
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		_ = service.stateManager.AddWorktree(mockWorktree)

		// Test listing includes our worktree
		worktrees := service.ListWorktrees()
		assert.Greater(t, len(worktrees), 0)

		// Find our test worktree
		var testWorktree *models.Worktree
		for _, w := range worktrees {
			if w.ID == "test-worktree" {
				testWorktree = w
				break
			}
		}
		assert.NotNil(t, testWorktree, "test-worktree should be in the list")
		assert.Equal(t, "test-worktree", testWorktree.ID)
		assert.Equal(t, "catnip/felix", testWorktree.Branch)

		// Test GetWorktreeDiff with mock worktree (will fail due to invalid path, but tests validation)
		diff, err := service.GetWorktreeDiff("test-worktree")
		assert.Error(t, err) // Expected since path doesn't exist
		assert.Nil(t, diff)
	})
}

func testStateManagement(t *testing.T, service *GitService) {
	// Create temporary directories for test state
	tempDir := t.TempDir()
	stateRepoPath := filepath.Join(tempDir, "state-repo")
	stateWorktreePath := filepath.Join(tempDir, "state-worktree")

	t.Run("SaveAndLoadState", func(t *testing.T) {
		// Add some test data using temporary paths
		mockRepo := &models.Repository{
			ID:            "test/state-repo",
			Path:          stateRepoPath,
			URL:           "https://github.com/test/state-repo.git",
			DefaultBranch: "main",
			CreatedAt:     time.Now(),
		}
		_ = service.stateManager.AddRepository(mockRepo)

		mockWorktree := &models.Worktree{
			ID:           "state-worktree",
			RepoID:       "test/state-repo",
			Name:         "state-repo/catnip/felix",
			Path:         stateWorktreePath,
			Branch:       "catnip/felix",
			SourceBranch: "main",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		_ = service.stateManager.AddWorktree(mockWorktree)

		// State is automatically saved by the state manager

		// Create new service - state will be loaded automatically
		newService := NewGitService()

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
	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

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

			// Should start with refs/catnip/
			assert.True(t, strings.HasPrefix(name, "refs/catnip/"), "Name should start with refs/catnip/: %s", name)

			// Should be a valid catnip branch name
			assert.True(t, isCatnipBranch(name), "Generated name should be valid catnip branch: %s", name)
		}
	})

	t.Run("IsCatnipBranch", func(t *testing.T) {
		// Test new refs/catnip/ format
		assert.True(t, isCatnipBranch("refs/catnip/felix"))
		assert.True(t, isCatnipBranch("refs/catnip/fluffy-felix"))
		// Test legacy catnip/ format for backward compatibility
		assert.True(t, isCatnipBranch("catnip/felix"))
		assert.True(t, isCatnipBranch("catnip/fluffy-felix"))
		// Test non-catnip branches
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

	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

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
	_ = service.stateManager.AddWorktree(mockWorktree)

	mockRepo := &models.Repository{
		ID:            "test/gh-repo",
		Path:          "/test/gh/repo",
		URL:           "https://github.com/test/gh-repo.git",
		DefaultBranch: "main",
		CreatedAt:     time.Now(),
	}
	_ = service.stateManager.AddRepository(mockRepo)

	t.Run("CreatePullRequest_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		pr, err := service.CreatePullRequest("non-existent", "Test PR", "Test body", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree non-existent not found")
		assert.Nil(t, pr)

		// Test with valid worktree (will fail at git operations, but validates worktree exists)
		pr, err = service.CreatePullRequest("gh-test-worktree", "Test PR", "Test body", false)
		assert.Error(t, err) // Expected - no real git repo
		assert.Nil(t, pr)
	})

	t.Run("UpdatePullRequest_ValidatesWorktree", func(t *testing.T) {
		// Test with non-existent worktree
		pr, err := service.UpdatePullRequest("non-existent", "Updated PR", "Updated body", false)
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
	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

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
	_ = service.stateManager.AddWorktree(mockWorktree)

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
	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

	service := NewGitService()

	t.Run("CleanupMergedWorktrees", func(t *testing.T) {
		// Should not error even with no worktrees
		_, _, err := service.CleanupMergedWorktrees()
		assert.NoError(t, err)

		// Add some test worktrees
		worktree1 := &models.Worktree{
			ID:           "test1",
			Branch:       "catnip/mittens",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		_ = service.stateManager.AddWorktree(worktree1)

		worktree2 := &models.Worktree{
			ID:           "test2",
			Branch:       "catnip/shadow",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		_ = service.stateManager.AddWorktree(worktree2)

		// Should not error with worktrees (though cleanup may not work without real git repos)
		_, _, err = service.CleanupMergedWorktrees()
		assert.NoError(t, err)
	})

	t.Run("CleanupUnusedBranches", func(t *testing.T) {
		// Should not error
		service.cleanupUnusedBranches()
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
