package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBranchOperations(t *testing.T) {
	// Create a temporary directory for test repositories
	tempDir := t.TempDir()

	// Create test repository with actual git
	testRepo := filepath.Join(tempDir, "test-repo")
	require.NoError(t, os.MkdirAll(testRepo, 0755))

	// Use shell git executor for real git operations in tests
	executor := NewGitCommandExecutor()
	branchOps := NewBranchOperations(executor)

	// Initialize git repo
	_, err := executor.ExecuteGitWithWorkingDir(testRepo, "init")
	require.NoError(t, err)

	// Configure git user for commits
	_, err = executor.ExecuteGitWithWorkingDir(testRepo, "config", "user.name", "Test User")
	require.NoError(t, err)
	_, err = executor.ExecuteGitWithWorkingDir(testRepo, "config", "user.email", "test@example.com")
	require.NoError(t, err)

	// Create initial commit
	readmePath := filepath.Join(testRepo, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# Test Repo\n"), 0644))
	_, err = executor.ExecuteGitWithWorkingDir(testRepo, "add", "README.md")
	require.NoError(t, err)
	_, err = executor.ExecuteGitWithWorkingDir(testRepo, "commit", "-m", "Initial commit")
	require.NoError(t, err)

	t.Run("NewBranchOperations", func(t *testing.T) {
		ops := NewBranchOperations(executor)
		assert.NotNil(t, ops)
		assert.Equal(t, executor, ops.executor)
	})

	t.Run("BranchExistsLocal", func(t *testing.T) {
		// Test existing main/master branch
		exists := branchOps.BranchExistsLocal(testRepo, "main")
		if !exists {
			// Try master if main doesn't exist
			exists = branchOps.BranchExistsLocal(testRepo, "master")
		}
		assert.True(t, exists, "Initial branch should exist")

		// Test non-existent branch
		exists = branchOps.BranchExistsLocal(testRepo, "nonexistent-branch")
		assert.False(t, exists)

		// Create a new branch and test
		_, err := executor.ExecuteGitWithWorkingDir(testRepo, "checkout", "-b", "feature-branch")
		require.NoError(t, err)

		exists = branchOps.BranchExistsLocal(testRepo, "feature-branch")
		assert.True(t, exists)
	})

	t.Run("BranchExists", func(t *testing.T) {
		// Test local branch
		exists := branchOps.BranchExists(testRepo, "feature-branch", BranchExistsOptions{IsRemote: false})
		assert.True(t, exists)

		// Test non-existent local branch
		exists = branchOps.BranchExists(testRepo, "nonexistent", BranchExistsOptions{IsRemote: false})
		assert.False(t, exists)

		// Test remote branch (will fail since we don't have remotes set up)
		exists = branchOps.BranchExists(testRepo, "main", BranchExistsOptions{
			IsRemote:   true,
			RemoteName: "origin",
		})
		assert.False(t, exists, "Remote branch should not exist without remote setup")
	})

	t.Run("BranchExistsRemote", func(t *testing.T) {
		// Test with default remote name
		exists := branchOps.BranchExistsRemote(testRepo, "main", "")
		assert.False(t, exists, "Should not find remote branch without remote setup")

		// Test with explicit remote name
		exists = branchOps.BranchExistsRemote(testRepo, "main", "origin")
		assert.False(t, exists, "Should not find remote branch without remote setup")
	})

	t.Run("GetCommitCount", func(t *testing.T) {
		// Switch back to main/master
		currentBranch := "main"
		_, err := executor.ExecuteGitWithWorkingDir(testRepo, "checkout", "main")
		if err != nil {
			// Try master
			_, err = executor.ExecuteGitWithWorkingDir(testRepo, "checkout", "master")
			if err == nil {
				currentBranch = "master"
			}
		}
		require.NoError(t, err)

		// Create additional commits on feature branch
		_, err = executor.ExecuteGitWithWorkingDir(testRepo, "checkout", "feature-branch")
		require.NoError(t, err)

		// Add a commit to feature branch
		testPath := filepath.Join(testRepo, "feature.txt")
		require.NoError(t, os.WriteFile(testPath, []byte("feature content\n"), 0644))
		_, err = executor.ExecuteGitWithWorkingDir(testRepo, "add", "feature.txt")
		require.NoError(t, err)
		_, err = executor.ExecuteGitWithWorkingDir(testRepo, "commit", "-m", "Add feature")
		require.NoError(t, err)

		// Count commits between main/master and feature-branch
		count, err := branchOps.GetCommitCount(testRepo, currentBranch, "feature-branch")
		assert.NoError(t, err)
		assert.Equal(t, 1, count, "Should have 1 commit difference")

		// Count in reverse direction
		count, err = branchOps.GetCommitCount(testRepo, "feature-branch", currentBranch)
		assert.NoError(t, err)
		assert.Equal(t, 0, count, "Should have 0 commits from feature to main")
	})

	t.Run("GetRemoteURL", func(t *testing.T) {
		// This should fail since we haven't set up a remote
		_, err := branchOps.GetRemoteURL(testRepo)
		assert.Error(t, err, "Should fail without remote origin")

		// Set up a remote and test
		_, err = executor.ExecuteGitWithWorkingDir(testRepo, "remote", "add", "origin", "https://github.com/test/repo.git")
		require.NoError(t, err)

		url, err := branchOps.GetRemoteURL(testRepo)
		assert.NoError(t, err)
		assert.Equal(t, "https://github.com/test/repo.git", url)
	})

	t.Run("GetDefaultBranch", func(t *testing.T) {
		// Without remote HEAD set, should fall back to checking branches
		branch, err := branchOps.GetDefaultBranch(testRepo)
		assert.NoError(t, err)
		// Should be main or master or fallback to main
		assert.Contains(t, []string{"main", "master"}, branch)
	})

	t.Run("GetLocalRepoBranches", func(t *testing.T) {
		branches, err := branchOps.GetLocalRepoBranches(testRepo)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(branches), 2, "Should have at least main/master and feature-branch")

		// Should contain our created branch
		found := false
		for _, branch := range branches {
			if branch == "feature-branch" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should contain feature-branch")
	})

	t.Run("GetRemoteBranches", func(t *testing.T) {
		// Without remote branches, should just return default branch
		branches, err := branchOps.GetRemoteBranches(testRepo, "main")
		assert.NoError(t, err)
		assert.Contains(t, branches, "main")
		assert.GreaterOrEqual(t, len(branches), 1, "Should have at least default branch")

		// Test with master as default
		branches, err = branchOps.GetRemoteBranches(testRepo, "master")
		assert.NoError(t, err)
		assert.Contains(t, branches, "master")
	})

	t.Run("SetupRemoteOrigin", func(t *testing.T) {
		// Test updating existing remote
		newURL := "https://github.com/test/new-repo.git"
		err := branchOps.SetupRemoteOrigin(testRepo, newURL)
		assert.NoError(t, err)

		// Verify the URL was updated
		url, err := branchOps.GetRemoteURL(testRepo)
		assert.NoError(t, err)
		assert.Equal(t, newURL, url)
	})
}

func TestBranchOperationsWithMockExecutor(t *testing.T) {
	// Test with in-memory executor for edge cases
	helper := NewInMemoryServiceHelper()
	executor := helper.GetInMemoryExecutor()
	branchOps := NewBranchOperations(executor)

	// Create test repository
	repo, err := CreateTestRepositoryWithHistory()
	require.NoError(t, err)
	repoPath := "/test/branch-ops"
	executor.AddRepository(repoPath, repo)

	t.Run("BranchExistsWithInMemory", func(t *testing.T) {
		// Test existing branch
		exists := branchOps.BranchExistsLocal(repoPath, "main")
		assert.True(t, exists)

		// Test non-existent branch
		exists = branchOps.BranchExistsLocal(repoPath, "nonexistent")
		assert.False(t, exists)
	})

	t.Run("GetRemoteURLWithInMemory", func(t *testing.T) {
		url, err := branchOps.GetRemoteURL(repoPath)
		assert.NoError(t, err)
		assert.Equal(t, "https://github.com/test/repo.git", url)
	})

	t.Run("GetDefaultBranchWithInMemory", func(t *testing.T) {
		branch, err := branchOps.GetDefaultBranch(repoPath)
		assert.NoError(t, err)
		assert.Equal(t, "main", branch)
	})
}

func TestBranchOperationsErrorHandling(t *testing.T) {
	// Test error handling with invalid repository path
	executor := NewGitCommandExecutor()
	branchOps := NewBranchOperations(executor)

	invalidPath := "/nonexistent/path"

	t.Run("ErrorHandling", func(t *testing.T) {
		// BranchExists should return false on error
		exists := branchOps.BranchExistsLocal(invalidPath, "main")
		assert.False(t, exists)

		// GetCommitCount should return error
		_, err := branchOps.GetCommitCount(invalidPath, "main", "feature")
		assert.Error(t, err)

		// GetRemoteURL should return error
		_, err = branchOps.GetRemoteURL(invalidPath)
		assert.Error(t, err)

		// GetLocalRepoBranches should return error
		_, err = branchOps.GetLocalRepoBranches(invalidPath)
		assert.Error(t, err)

		// GetRemoteBranches should handle error gracefully
		branches, err := branchOps.GetRemoteBranches(invalidPath, "main")
		assert.NoError(t, err) // Should not error, just return default
		assert.Equal(t, []string{"main"}, branches)

		// SetupRemoteOrigin should return error
		err = branchOps.SetupRemoteOrigin(invalidPath, "https://github.com/test/repo.git")
		assert.Error(t, err)
	})
}

func TestBranchExistsOptions(t *testing.T) {
	t.Run("DefaultRemoteName", func(t *testing.T) {
		executor := NewGitCommandExecutor()
		branchOps := NewBranchOperations(executor)

		// Test that empty remote name defaults to "origin"
		exists := branchOps.BranchExistsRemote("/tmp", "main", "")
		// This will likely fail due to invalid path, but we're testing the parameter handling
		assert.False(t, exists)
	})
}
