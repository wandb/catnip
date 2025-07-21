package git

import (
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoGitRepositoryExtraFunctions(t *testing.T) {
	t.Run("NewGoGitRepository", func(t *testing.T) {
		storage := memory.NewStorage()
		fs := memfs.New()
		repo := NewGoGitRepository(storage, fs, "/test/path")

		assert.NotNil(t, repo)
		assert.Equal(t, "/test/path", repo.GetPath())
	})

	t.Run("FetchWithDepth", func(t *testing.T) {
		// Create a gogit repository with test data
		repo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)

		goGitRepo := NewGoGitRepositoryFromExisting(repo.GetRepository(), "/test/path")

		// Test FetchWithDepth (will fail without real remote, but tests the code path)
		err = goGitRepo.FetchWithDepth("main", 5)
		// We expect this to fail because we don't have a real remote, but it exercises the code
		assert.Error(t, err)
	})

	t.Run("Unshallow", func(t *testing.T) {
		// Create a gogit repository
		repo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)

		goGitRepo := NewGoGitRepositoryFromExisting(repo.GetRepository(), "/test/path")

		// Test Unshallow - for non-shallow repo, should be no-op
		err = goGitRepo.Unshallow()
		// This should succeed for a non-shallow repo
		assert.NoError(t, err)
	})

	t.Run("Clone", func(t *testing.T) {
		storage := memory.NewStorage()
		fs := memfs.New()
		goGitRepo := NewGoGitRepository(storage, fs, "/test/path")

		// Test Clone with invalid URL (will fail but exercises code)
		err := goGitRepo.Clone("invalid-url", "/test/path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to clone repository")
	})
}

func TestGoGitWorktreeExtraFunctions(t *testing.T) {
	t.Run("GetConflictedFiles", func(t *testing.T) {
		// Create a test repository
		repo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)

		worktree, err := NewGoGitWorktree(repo.GetRepository(), "/test/path", "main")
		require.NoError(t, err)

		// Get conflicted files (should be empty for clean repo)
		files := worktree.GetConflictedFiles()
		assert.Empty(t, files)
	})

	t.Run("NetworkOperations", func(t *testing.T) {
		// Create a test repository
		repo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)

		worktree, err := NewGoGitWorktree(repo.GetRepository(), "/test/path", "main")
		require.NoError(t, err)

		// Test Pull (will fail without remote but exercises code)
		err = worktree.Pull()
		assert.Error(t, err) // Expected to fail without remote setup

		// Test Push (will fail without remote but exercises code)
		err = worktree.Push()
		assert.Error(t, err) // Expected to fail without remote setup

		// Test PushForce (will fail without remote but exercises code)
		err = worktree.PushForce()
		assert.Error(t, err) // Expected to fail without remote setup
	})

	t.Run("UnsupportedOperations", func(t *testing.T) {
		// Create a test repository
		repo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)

		worktree, err := NewGoGitWorktree(repo.GetRepository(), "/test/path", "main")
		require.NoError(t, err)

		// Test Rebase (not fully supported)
		err = worktree.Rebase("main")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rebase not supported")
	})
}

func TestInMemoryExecutorExtraFunctions(t *testing.T) {
	executor := NewInMemoryExecutor()

	// Create and add a test repository
	repo, err := CreateTestRepositoryWithHistory()
	require.NoError(t, err)
	repoPath := "/test/inmemory"
	executor.AddRepository(repoPath, repo)

	t.Run("HandleStatus", func(t *testing.T) {
		// This tests the handleStatus function in InMemoryExecutor
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "status", "--porcelain")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output)) // Clean repository
	})

	t.Run("HandleRevParse", func(t *testing.T) {
		// Test rev-parse HEAD
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "rev-parse", "HEAD")
		assert.NoError(t, err)
		assert.NotEmpty(t, string(output))
	})

	t.Run("HandleRevList", func(t *testing.T) {
		// Test rev-list --count
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "rev-list", "--count", "HEAD")
		assert.NoError(t, err)
		assert.NotEmpty(t, string(output))
	})

	t.Run("HandleFetch", func(t *testing.T) {
		// Test fetch (will succeed in mock)
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "fetch")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output)) // Mock fetch returns empty
	})

	t.Run("HandlePush", func(t *testing.T) {
		// Test push (will succeed in mock)
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "push")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output)) // Mock push returns empty
	})

	t.Run("HandleDiff", func(t *testing.T) {
		// Test diff (will return empty for clean repo)
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "diff")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output)) // Clean repo, no diff
	})

	t.Run("HandleLsFiles", func(t *testing.T) {
		// Test ls-files command
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "ls-files")
		assert.NoError(t, err)
		// Might be empty for in-memory test repo, but should not error
		_ = string(output) // Just test that it doesn't crash
	})

	t.Run("HandleShow", func(t *testing.T) {
		// Test show command
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "show", "HEAD")
		assert.NoError(t, err)
		assert.NotEmpty(t, string(output)) // Should show commit info
	})

	t.Run("HandleMergeBase", func(t *testing.T) {
		// Test merge-base command
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "merge-base", "HEAD", "HEAD")
		assert.NoError(t, err)
		assert.NotEmpty(t, string(output)) // Should return commit hash
	})

	t.Run("OtherCommands", func(t *testing.T) {
		// Test commands that will exercise other handlers
		_, err := executor.ExecuteGitWithWorkingDir(repoPath, "add", ".")
		assert.NoError(t, err) // Mock should succeed

		_, err = executor.ExecuteGitWithWorkingDir(repoPath, "commit", "-m", "test")
		assert.NoError(t, err) // Mock should succeed

		_, err = executor.ExecuteGitWithWorkingDir(repoPath, "checkout", "main")
		assert.NoError(t, err) // Mock should succeed

		_, err = executor.ExecuteGitWithWorkingDir(repoPath, "config", "user.name")
		assert.NoError(t, err) // Mock should succeed

		// Test worktree command to exercise more handlers
		_, err = executor.ExecuteGitWithWorkingDir(repoPath, "worktree", "list")
		assert.NoError(t, err) // Mock should succeed
	})

	// Test direct executor methods for coverage
	t.Run("ExecutorMethods", func(t *testing.T) {
		// Test Execute method (alias) - use ExecuteGitWithWorkingDir instead
		output, err := executor.ExecuteGitWithWorkingDir(repoPath, "status", "--porcelain")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output)) // Clean repository

		// Test CreateRepository
		newRepo, err := executor.CreateRepository("/test/new")
		assert.NoError(t, err)
		assert.NotNil(t, newRepo)
	})
}
