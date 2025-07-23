package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/git"
	"github.com/vanpelt/catnip/internal/git/executor"
	"github.com/vanpelt/catnip/internal/models"
)

// TestGitServiceWithInMemoryOperations demonstrates testing GitService with in-memory git operations
func TestGitServiceWithInMemoryOperations(t *testing.T) {
	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("WORKSPACE_DIR")
	require.NoError(t, os.Setenv("WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

	// Create service with in-memory git operations
	exec := executor.NewInMemoryExecutor()
	operations := git.NewOperationsWithExecutor(exec)
	service := NewGitServiceWithOperations(operations)
	require.NotNil(t, service)
	require.NotNil(t, exec)

	inMemoryExec, ok := exec.(*executor.InMemoryExecutor)
	require.True(t, ok)

	t.Run("InMemoryRepositorySetup", func(t *testing.T) {
		testInMemoryRepositorySetup(t, service, inMemoryExec)
	})

	t.Run("GitOperationsWithInMemory", func(t *testing.T) {
		testGitOperationsWithInMemory(t, service, inMemoryExec)
	})

	t.Run("WorktreeOperationsWithInMemory", func(t *testing.T) {
		testWorktreeOperationsWithInMemory(t, service, inMemoryExec)
	})
}

func testInMemoryRepositorySetup(t *testing.T, service *GitService, exec *executor.InMemoryExecutor) {
	t.Run("CreateTestRepository", func(t *testing.T) {
		// Create a test repository with some history
		repo, err := executor.CreateTestRepositoryWithHistory()
		require.NoError(t, err)
		assert.NotNil(t, repo)

		// Add it to the executor at a known path
		repoPath := "/test/repo"
		exec.AddRepository(repoPath, repo)

		// Verify we can execute git commands on it
		output, err := exec.ExecuteGitWithWorkingDir(repoPath, "status", "--porcelain")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output)) // Clean repository
	})

	t.Run("CreateTestRepositoryWithConflicts", func(t *testing.T) {
		// Create a repository set up for conflicts
		repo, err := executor.CreateTestRepositoryWithConflicts()
		require.NoError(t, err)
		assert.NotNil(t, repo)

		conflictPath := "/test/conflict-repo"
		exec.AddRepository(conflictPath, repo)

		// Verify repository is accessible
		output, err := exec.ExecuteGitWithWorkingDir(conflictPath, "branch", "--show-current")
		assert.NoError(t, err)
		assert.NotEmpty(t, string(output))
	})
}

func testGitOperationsWithInMemory(t *testing.T, service *GitService, exec *executor.InMemoryExecutor) {
	// Create a test repository
	repo, err := executor.CreateTestRepositoryWithHistory()
	require.NoError(t, err)

	repoPath := "/test/git-ops"
	exec.AddRepository(repoPath, repo)

	t.Run("BasicGitCommands", func(t *testing.T) {
		// Test basic git operations through the operations interface

		// Test ExecuteGit
		output, err := service.operations.ExecuteGit(repoPath, "status", "--porcelain")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output))

		// Test branch operations
		exists := service.operations.BranchExists(repoPath, "main", false)
		assert.True(t, exists) // Should exist in our test repo

		exists = service.operations.BranchExists(repoPath, "nonexistent", false)
		assert.False(t, exists)

		// Test remote operations
		remoteURL, err := service.operations.GetRemoteURL(repoPath)
		assert.NoError(t, err)
		assert.Equal(t, "https://github.com/test/repo.git", remoteURL)
	})

	t.Run("StatusOperations", func(t *testing.T) {

		// Test worktree status operations
		isDirty := service.operations.IsDirty(repoPath)
		assert.False(t, isDirty) // Clean repository

		hasConflicts := service.operations.HasConflicts(repoPath)
		assert.False(t, hasConflicts) // No conflicts

		// Test comprehensive status
		status, err := service.operations.GetStatus(repoPath)
		assert.NoError(t, err)
		assert.NotNil(t, status)
	})

	t.Run("FetchOperations", func(t *testing.T) {
		// Test fetch operations (should not error in mock implementation)
		err := service.operations.FetchBranchFast(repoPath, "main")
		assert.NoError(t, err)

		err = service.operations.FetchBranchFull(repoPath, "main")
		assert.NoError(t, err)
	})
}

func testWorktreeOperationsWithInMemory(t *testing.T, service *GitService, exec *executor.InMemoryExecutor) {
	// Clear any existing state from previous test runs
	for k := range service.worktrees {
		delete(service.worktrees, k)
	}
	for k := range service.repositories {
		delete(service.repositories, k)
	}

	// Create test repositories and add them to service state
	repo1, err := executor.CreateTestRepositoryWithHistory()
	require.NoError(t, err)
	repo1Path := "/test/worktree-repo1"
	exec.AddRepository(repo1Path, repo1)

	// Manually add repository to service state for testing
	mockRepo := &models.Repository{
		ID:            "test/worktree-repo",
		Path:          repo1Path,
		URL:           "https://github.com/test/worktree-repo.git",
		DefaultBranch: "main",
		CreatedAt:     time.Now(),
	}
	service.repositories["test/worktree-repo"] = mockRepo

	t.Run("ListWorktrees", func(t *testing.T) {
		// Initially empty
		worktrees := service.ListWorktrees()
		assert.Empty(t, worktrees)

		// Add some test worktrees
		mockWorktree1 := &models.Worktree{
			ID:           "wt-1",
			RepoID:       "test/worktree-repo",
			Name:         "worktree-repo/catnip/felix",
			Path:         "/test/worktrees/wt-1",
			Branch:       "catnip/felix",
			SourceBranch: "main",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		service.worktrees["wt-1"] = mockWorktree1

		mockWorktree2 := &models.Worktree{
			ID:           "wt-2",
			RepoID:       "test/worktree-repo",
			Name:         "worktree-repo/catnip/whiskers",
			Path:         "/test/worktrees/wt-2",
			Branch:       "catnip/whiskers",
			SourceBranch: "main",
			CreatedAt:    time.Now(),
			LastAccessed: time.Now(),
		}
		service.worktrees["wt-2"] = mockWorktree2

		// Now should have worktrees
		worktrees = service.ListWorktrees()
		assert.Len(t, worktrees, 2)

		// Sort worktrees by ID for deterministic testing
		sort.Slice(worktrees, func(i, j int) bool {
			return worktrees[i].ID < worktrees[j].ID
		})

		assert.Equal(t, "wt-1", worktrees[0].ID)
		assert.Equal(t, "catnip/felix", worktrees[0].Branch)
		assert.Equal(t, "wt-2", worktrees[1].ID)
		assert.Equal(t, "catnip/whiskers", worktrees[1].Branch)
	})

	t.Run("GetStatus", func(t *testing.T) {
		status := service.GetStatus()
		assert.NotNil(t, status)
		assert.NotNil(t, status.Repositories)

		// Should include our test repository
		assert.GreaterOrEqual(t, len(status.Repositories), 1)

		// Should include worktree count
		assert.Equal(t, 2, status.WorktreeCount) // From previous test
	})

	t.Run("GetRepositoryByID", func(t *testing.T) {
		repo := service.GetRepositoryByID("test/worktree-repo")
		assert.NotNil(t, repo)
		assert.Equal(t, "test/worktree-repo", repo.ID)
		assert.Equal(t, repo1Path, repo.Path)

		// Non-existent repository
		repo = service.GetRepositoryByID("nonexistent")
		assert.Nil(t, repo)
	})

	t.Run("ListRepositories", func(t *testing.T) {
		repos := service.ListRepositories()
		assert.Len(t, repos, 1)
		assert.Equal(t, "test/worktree-repo", repos[0].ID)
	})
}

// TestGitServiceInMemoryAdvanced demonstrates advanced testing scenarios with in-memory operations
func TestGitServiceInMemoryAdvanced(t *testing.T) {
	// Create service with in-memory operations
	exec := executor.NewInMemoryExecutor()
	operations := git.NewOperationsWithExecutor(exec)
	service := NewGitServiceWithOperations(operations)

	inMemoryExec, ok := exec.(*executor.InMemoryExecutor)
	require.True(t, ok)

	// Create a complex test scenario
	repo, err := executor.CreateTestRepositoryWithHistory()
	require.NoError(t, err)

	repoPath := "/advanced/test/repo"
	inMemoryExec.AddRepository(repoPath, repo)

	t.Run("GitOperationChaining", func(t *testing.T) {
		// Test that we can chain multiple git operations

		// 1. Check status
		isDirty := service.operations.IsDirty(repoPath)
		assert.False(t, isDirty)

		// 2. Check branches
		exists := service.operations.BranchExists(repoPath, "main", false)
		assert.True(t, exists)

		// 3. Get remote URL
		url, err := service.operations.GetRemoteURL(repoPath)
		assert.NoError(t, err)
		assert.Equal(t, "https://github.com/test/repo.git", url)

		// 4. Perform fetch
		err = service.operations.FetchBranchFast(repoPath, "main")
		assert.NoError(t, err)
	})

	t.Run("ServiceOperationsWithInMemory", func(t *testing.T) {
		// Test that service-level operations work with in-memory backend

		// Clear any existing state from previous test runs
		for k := range service.repositories {
			delete(service.repositories, k)
		}

		// Add repository to service state
		mockRepo := &models.Repository{
			ID:            "advanced/test",
			Path:          repoPath,
			URL:           "https://github.com/advanced/test.git",
			DefaultBranch: "main",
			CreatedAt:     time.Now(),
		}
		service.repositories["advanced/test"] = mockRepo

		// Test operations that should work with in-memory backend
		repos := service.ListRepositories()
		assert.Len(t, repos, 1)

		status := service.GetStatus()
		assert.NotNil(t, status)
		assert.Equal(t, 1, len(status.Repositories))
	})
}

// TestGitServiceLocalRepositoryOperations tests local repository operations with in-memory setup
func TestGitServiceLocalRepositoryOperations(t *testing.T) {
	// Create temporary workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("CATNIP_WORKSPACE_DIR")
	require.NoError(t, os.Setenv("CATNIP_WORKSPACE_DIR", tempDir))
	defer func() {
		if oldWorkspace == "" {
			_ = os.Unsetenv("CATNIP_WORKSPACE_DIR")
		} else {
			_ = os.Setenv("CATNIP_WORKSPACE_DIR", oldWorkspace)
		}
	}()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

	// Create a real test repository on the filesystem for worktree operations
	localRepoPath := filepath.Join(tempDir, "repos", "test-project")
	require.NoError(t, createRealTestRepository(localRepoPath))

	// Create service with regular git operations (not in-memory) for real filesystem operations
	service := NewGitService()

	// Manually add the local repository to the GitService state
	// This simulates what detectLocalRepos() would do
	localRepoModel := &models.Repository{
		ID:            "local/test-project",
		URL:           "file://" + localRepoPath,
		Path:          localRepoPath,
		DefaultBranch: "main", // Our test repos now use main
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
	}
	service.repositories["local/test-project"] = localRepoModel

	t.Run("LocalRepositoryOperations", func(t *testing.T) {
		testLocalRepositoryOperations(t, service, nil) // No executor needed for real git operations
	})

	t.Run("LocalRepositoryWorktreeLifecycle", func(t *testing.T) {
		testLocalRepositoryWorktreeLifecycle(t, service, nil) // No executor needed for real git operations
	})
}

func testLocalRepositoryOperations(t *testing.T, service *GitService, exec *executor.InMemoryExecutor) {
	repoID := "local/test-project"

	t.Run("GetLocalRepoBranches", func(t *testing.T) {
		// Test the getLocalRepoBranches function
		repo, exists := service.repositories[repoID]
		require.True(t, exists)

		branches, err := service.getLocalRepoBranches(repo.Path)
		assert.NoError(t, err)
		assert.Contains(t, branches, "main")
		assert.Contains(t, branches, "feature/test")
		assert.Len(t, branches, 2) // main and feature/test from CreateTestRepositoryWithHistory
	})

	t.Run("GetLocalRepoDefaultBranch", func(t *testing.T) {
		repo, exists := service.repositories[repoID]
		require.True(t, exists)

		defaultBranch := service.getLocalRepoDefaultBranch(repo.Path)
		assert.Equal(t, "main", defaultBranch)
	})

	t.Run("GetRepositoryBranches", func(t *testing.T) {
		// Test the public GetRepositoryBranches method for local repos
		branches, err := service.GetRepositoryBranches(repoID)
		assert.NoError(t, err)
		assert.Contains(t, branches, "main")
		assert.Contains(t, branches, "feature/test")
	})

	t.Run("HandleLocalRepoWorktree", func(t *testing.T) {
		// Test creating a worktree for the local repository
		repo, worktree, err := service.handleLocalRepoWorktree(repoID, "main")
		assert.NoError(t, err)
		assert.NotNil(t, repo)
		assert.NotNil(t, worktree)

		// Verify the worktree was created correctly
		assert.Equal(t, repoID, worktree.RepoID)
		assert.Equal(t, "main", worktree.SourceBranch)
		assert.Contains(t, worktree.Name, "test-project/")            // Should have repo prefix
		assert.True(t, strings.HasPrefix(worktree.Branch, "catnip/")) // Should be a catnip branch

		// Verify the worktree branch was created in the live repo
		branches, err := service.getLocalRepoBranches(repo.Path)
		assert.NoError(t, err)
		assert.Contains(t, branches, worktree.Branch)

		// Verify worktree was added to service
		worktrees := service.ListWorktrees()
		assert.GreaterOrEqual(t, len(worktrees), 1)

		// Find our worktree in the list
		found := false
		for _, wt := range worktrees {
			if wt.ID == worktree.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "Created worktree should be in the list")

		// Verify worktree directory was created
		assert.DirExists(t, worktree.Path)

		// Clean up this worktree for the next tests
		_ = service.DeleteWorktree(worktree.ID)
	})

	t.Run("CreateLocalRepoWorktreeDirectly", func(t *testing.T) {
		// Test the createLocalRepoWorktree function directly
		repo, exists := service.repositories[repoID]
		require.True(t, exists)

		// Generate a unique name for this test
		uniqueName := service.generateUniqueSessionName(repo.Path)
		worktree, err := service.createLocalRepoWorktree(repo, "main", uniqueName)

		assert.NoError(t, err)
		assert.NotNil(t, worktree)
		assert.Equal(t, repoID, worktree.RepoID)
		assert.Equal(t, "main", worktree.SourceBranch)
		assert.Equal(t, uniqueName, worktree.Branch)

		// Verify the worktree directory was created
		assert.DirExists(t, worktree.Path)

		// Verify the branch exists in the live repo
		branches, err := service.getLocalRepoBranches(repo.Path)
		assert.NoError(t, err)
		assert.Contains(t, branches, uniqueName)

		// Clean up this worktree for the next tests
		_ = service.DeleteWorktree(worktree.ID)
	})

	t.Run("ShouldCreateInitialWorktree", func(t *testing.T) {
		// Test with a fresh repo ID that has no worktrees
		freshRepoID := "local/fresh-project"

		// Add a fresh repository
		freshRepoModel := &models.Repository{
			ID:            freshRepoID,
			URL:           "file:///live/fresh-project",
			Path:          "/live/fresh-project",
			DefaultBranch: "main",
			CreatedAt:     time.Now(),
			LastAccessed:  time.Now(),
		}
		service.repositories[freshRepoID] = freshRepoModel

		// Should return true for a repo with no worktrees
		should := service.shouldCreateInitialWorktree(freshRepoID)
		assert.True(t, should)

		// After creating a worktree, should return false
		// (We can't easily test this without actual filesystem since it checks /workspace)
		// But we can test the logic path
	})
}

func testLocalRepositoryWorktreeLifecycle(t *testing.T, service *GitService, exec *executor.InMemoryExecutor) {
	repoID := "local/test-project"

	t.Run("CreateAndDeleteWorktree", func(t *testing.T) {
		// Clear any existing worktrees from previous tests
		for id := range service.worktrees {
			_ = service.DeleteWorktree(id)
		}

		// Get the repository
		repo, exists := service.repositories[repoID]
		require.True(t, exists)

		// Create a worktree
		repoObj, worktree, err := service.handleLocalRepoWorktree(repoID, "main")
		require.NoError(t, err)
		require.NotNil(t, repoObj)
		require.NotNil(t, worktree)

		worktreeID := worktree.ID
		worktreBranch := worktree.Branch

		// Verify worktree exists
		worktrees := service.ListWorktrees()
		assert.Len(t, worktrees, 1)
		assert.Equal(t, worktreeID, worktrees[0].ID)

		// Verify branch exists in live repo
		branches, err := service.getLocalRepoBranches(repo.Path)
		require.NoError(t, err)
		assert.Contains(t, branches, worktreBranch)

		// Verify worktree directory exists
		assert.DirExists(t, worktree.Path)

		// Delete the worktree
		err = service.DeleteWorktree(worktreeID)
		assert.NoError(t, err)

		// Verify worktree was removed from service
		worktrees = service.ListWorktrees()
		assert.Len(t, worktrees, 0)

		// Verify branch was deleted from live repo
		branches, err = service.getLocalRepoBranches(repo.Path)
		require.NoError(t, err)
		assert.NotContains(t, branches, worktreBranch)

		// Verify worktree directory was removed
		assert.NoDirExists(t, worktree.Path)
	})

	t.Run("CreateMultipleWorktreesAndDeleteOne", func(t *testing.T) {
		// Clear any existing worktrees from previous tests
		for id := range service.worktrees {
			_ = service.DeleteWorktree(id)
		}

		// Create two worktrees
		_, worktree1, err := service.handleLocalRepoWorktree(repoID, "main")
		require.NoError(t, err)

		_, worktree2, err := service.handleLocalRepoWorktree(repoID, "feature/test")
		require.NoError(t, err)

		// Verify both worktrees exist
		worktrees := service.ListWorktrees()
		assert.Len(t, worktrees, 2)

		// Verify both branches exist in live repo
		repo := service.repositories[repoID]
		branches, err := service.getLocalRepoBranches(repo.Path)
		require.NoError(t, err)
		assert.Contains(t, branches, worktree1.Branch)
		assert.Contains(t, branches, worktree2.Branch)

		// Delete first worktree
		err = service.DeleteWorktree(worktree1.ID)
		assert.NoError(t, err)

		// Verify only one worktree remains
		worktrees = service.ListWorktrees()
		assert.Len(t, worktrees, 1)
		assert.Equal(t, worktree2.ID, worktrees[0].ID)

		// Verify first branch was deleted but second remains
		branches, err = service.getLocalRepoBranches(repo.Path)
		require.NoError(t, err)
		assert.NotContains(t, branches, worktree1.Branch)
		assert.Contains(t, branches, worktree2.Branch)

		// Clean up remaining worktree
		_ = service.DeleteWorktree(worktree2.ID)
	})

	t.Run("DeleteNonexistentWorktree", func(t *testing.T) {
		// Try to delete a worktree that doesn't exist
		err := service.DeleteWorktree("nonexistent-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "worktree nonexistent-id not found")
	})
}

// TestInMemoryExecutorDirectly tests the InMemoryExecutor independently
func TestInMemoryExecutorDirectly(t *testing.T) {
	exec := executor.NewInMemoryExecutor()
	inMemoryExec, ok := exec.(*executor.InMemoryExecutor)
	require.True(t, ok)

	t.Run("BasicCommandExecution", func(t *testing.T) {
		// Test non-git commands
		output, err := inMemoryExec.Execute("", "echo", "hello")
		assert.NoError(t, err)
		assert.Equal(t, "hello\n", string(output))
	})

	t.Run("GitCommandsWithoutRepository", func(t *testing.T) {
		// Should fail gracefully when no repository is found
		_, err := inMemoryExec.ExecuteGitWithWorkingDir("/nonexistent", "status")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no repository found")
	})

	t.Run("GitCommandsWithRepository", func(t *testing.T) {
		// Create and add a test repository
		repo, err := executor.NewTestRepository("/test/direct")
		require.NoError(t, err)

		// Add initial commit to make it valid
		err = repo.CommitFile("README.md", "# Test", "Initial commit")
		require.NoError(t, err)

		// Rename master to main for consistency
		err = repo.RenameBranch("master", "main")
		require.NoError(t, err)

		inMemoryExec.AddRepository("/test/direct", repo)

		// Test git commands
		output, err := inMemoryExec.ExecuteGitWithWorkingDir("/test/direct", "status", "--porcelain")
		assert.NoError(t, err)
		assert.Equal(t, "", string(output)) // Clean repository

		output, err = inMemoryExec.ExecuteGitWithWorkingDir("/test/direct", "branch", "--show-current")
		assert.NoError(t, err)
		assert.Contains(t, string(output), "main") // renamed from master
	})
}

// createRealTestRepository creates a real git repository on the filesystem for testing
func createRealTestRepository(repoPath string) error {
	// Create directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return err
	}

	// Initialize git repo
	if err := exec.Command("git", "-C", repoPath, "init").Run(); err != nil {
		return err
	}

	// Configure git user for commits
	if err := exec.Command("git", "-C", repoPath, "config", "user.name", "Test User").Run(); err != nil {
		return err
	}
	if err := exec.Command("git", "-C", repoPath, "config", "user.email", "test@example.com").Run(); err != nil {
		return err
	}

	// Create initial file and commit
	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repository\n\nThis is a test.\n"), 0644); err != nil {
		return err
	}

	if err := exec.Command("git", "-C", repoPath, "add", "README.md").Run(); err != nil {
		return err
	}

	if err := exec.Command("git", "-C", repoPath, "commit", "-m", "Initial commit").Run(); err != nil {
		return err
	}

	// Rename master to main if needed
	_ = exec.Command("git", "-C", repoPath, "branch", "-m", "master", "main").Run()
	// Ignore error - might already be main

	// Create a feature branch
	if err := exec.Command("git", "-C", repoPath, "checkout", "-b", "feature/test").Run(); err != nil {
		return err
	}

	// Add commit to feature branch
	featurePath := filepath.Join(repoPath, "feature.txt")
	if err := os.WriteFile(featurePath, []byte("New feature implementation\n"), 0644); err != nil {
		return err
	}

	if err := exec.Command("git", "-C", repoPath, "add", "feature.txt").Run(); err != nil {
		return err
	}

	if err := exec.Command("git", "-C", repoPath, "commit", "-m", "Add new feature").Run(); err != nil {
		return err
	}

	// Switch back to main
	if err := exec.Command("git", "-C", repoPath, "checkout", "main").Run(); err != nil {
		return err
	}

	return nil
}
