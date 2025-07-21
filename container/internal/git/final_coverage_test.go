package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceHelperTypes tests different service helper constructors
func TestServiceHelperTypes(t *testing.T) {
	t.Run("DefaultServiceHelper", func(t *testing.T) {
		helper := NewServiceHelper()
		assert.NotNil(t, helper)
		assert.NotNil(t, helper.Executor)

		// Should use GoGitCommandExecutor by default
		_, ok := helper.Executor.(*GoGitCommandExecutor)
		assert.True(t, ok)
	})

	t.Run("ShellServiceHelper", func(t *testing.T) {
		helper := NewShellServiceHelper()
		assert.NotNil(t, helper)
		assert.NotNil(t, helper.Executor)

		// Should use CommandExecutorImpl for shell operations
		_, ok := helper.Executor.(*CommandExecutorImpl)
		assert.True(t, ok)
	})

	t.Run("InMemoryServiceHelper", func(t *testing.T) {
		helper := NewInMemoryServiceHelper()
		assert.NotNil(t, helper)

		executor := helper.GetInMemoryExecutor()
		assert.NotNil(t, executor)

		// Test repository creation
		repo, err := executor.CreateRepository("/test/memory")
		assert.NoError(t, err)
		assert.NotNil(t, repo)
	})
}

// TestManagerConstruction tests manager creation
func TestManagerConstruction(t *testing.T) {
	t.Run("NewManager", func(t *testing.T) {
		manager := NewManager()
		assert.NotNil(t, manager)
	})
}

// TestRepositoryInterfaceMethods tests repository interface implementations
func TestRepositoryInterfaceMethods(t *testing.T) {
	t.Run("TestRepositoryMethods", func(t *testing.T) {
		testRepo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)

		// Test GetRepository method
		gitRepo := testRepo.GetRepository()
		assert.NotNil(t, gitRepo)

		// Test ToRepository conversion
		repo := testRepo.ToRepository()
		assert.NotNil(t, repo)

		// Test basic path operations
		assert.Equal(t, "/test/repo", repo.GetPath())
	})

	t.Run("ConflictRepository", func(t *testing.T) {
		conflictRepo, err := CreateTestRepositoryWithConflicts()
		require.NoError(t, err)
		assert.NotNil(t, conflictRepo)

		// Test basic conversion
		repo := conflictRepo.ToRepository()
		assert.NotNil(t, repo)
	})
}

// TestWorktreeCreation tests worktree factory functions
func TestWorktreeCreation(t *testing.T) {
	t.Run("NewGoGitWorktree", func(t *testing.T) {
		testRepo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)

		// Create worktree from existing repository
		worktree, err := NewGoGitWorktree(testRepo.GetRepository(), "/test/path", "main")
		require.NoError(t, err)
		assert.NotNil(t, worktree)

		assert.Equal(t, "/test/path", worktree.GetPath())
		assert.Equal(t, "main", worktree.GetBranch())
	})
}

// TestExecutorEdgeCases tests error handling in executors
func TestExecutorEdgeCases(t *testing.T) {
	t.Run("GoGitExecutorErrors", func(t *testing.T) {
		executor := NewGoGitCommandExecutor()

		// Test with invalid working directory
		_, err := executor.ExecuteGitWithWorkingDir("/nonexistent/path", "status")
		assert.Error(t, err)

		// Test ExecuteCommand with non-git command
		_, err = executor.ExecuteCommand("invalid-command", "arg")
		assert.Error(t, err)
	})

	t.Run("InMemoryExecutorErrors", func(t *testing.T) {
		executor := NewInMemoryExecutor()

		// Test with non-existent repository
		_, err := executor.ExecuteGitWithWorkingDir("/nonexistent", "status")
		assert.Error(t, err)

		// Test unsupported commands
		_, err = executor.ExecuteGitWithWorkingDir("/test", "unsupported-command")
		assert.Error(t, err)
	})
}

// TestAdditionalGitOperations tests additional git command handlers
func TestAdditionalGitOperations(t *testing.T) {
	t.Run("InMemoryExecutorCommands", func(t *testing.T) {
		executor := NewInMemoryExecutor()

		// Create and add repository
		repo, err := CreateTestRepositoryWithHistory()
		require.NoError(t, err)
		repoPath := "/test/extra"
		executor.AddRepository(repoPath, repo)

		// Test various git command handlers
		commands := [][]string{
			{"branch", "--show-current"},
			{"rev-parse", "--abbrev-ref", "HEAD"},
			{"rev-list", "--count", "HEAD"},
			{"merge-base", "HEAD", "HEAD"},
		}

		for _, cmd := range commands {
			_, err := executor.ExecuteGitWithWorkingDir(repoPath, cmd...)
			assert.NoError(t, err, "Command failed: %v", cmd)
		}
	})

	t.Run("GoGitExecutorWithRealRepo", func(t *testing.T) {
		// Create real git repository
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "real-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0755))

		// Initialize with shell git
		shellExec := NewGitCommandExecutor()
		_, err := shellExec.ExecuteGitWithWorkingDir(repoDir, "init")
		require.NoError(t, err)

		// Set config
		_, err = shellExec.ExecuteGitWithWorkingDir(repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)
		_, err = shellExec.ExecuteGitWithWorkingDir(repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)

		// Add remote
		_, err = shellExec.ExecuteGitWithWorkingDir(repoDir, "remote", "add", "origin", "https://github.com/test/repo.git")
		require.NoError(t, err)

		// Create initial commit
		testFile := filepath.Join(repoDir, "test.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))
		_, err = shellExec.ExecuteGitWithWorkingDir(repoDir, "add", "test.txt")
		require.NoError(t, err)
		_, err = shellExec.ExecuteGitWithWorkingDir(repoDir, "commit", "-m", "Initial commit")
		require.NoError(t, err)

		// Test go-git executor with this repository
		goGitExec := NewGoGitCommandExecutor()

		// Test various operations handled by go-git
		output, err := goGitExec.ExecuteGitWithWorkingDir(repoDir, "config", "--get", "remote.origin.url")
		assert.NoError(t, err)
		assert.Contains(t, string(output), "github.com/test/repo.git")

		output, err = goGitExec.ExecuteGitWithWorkingDir(repoDir, "config", "--get", "core.bare")
		assert.NoError(t, err)
		assert.Contains(t, string(output), "false")

		output, err = goGitExec.ExecuteGitWithWorkingDir(repoDir, "remote", "get-url", "origin")
		assert.NoError(t, err)
		assert.Contains(t, string(output), "github.com/test/repo.git")

		// Test rev-parse variations
		output, err = goGitExec.ExecuteGitWithWorkingDir(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
		assert.NoError(t, err)
		assert.NotEmpty(t, string(output))

		// Test fetch (will fail but exercises code path)
		_, err = goGitExec.ExecuteGitWithWorkingDir(repoDir, "fetch")
		// Expected to fail without real remote, but exercises the code path
		_ = err
	})
}

// TestRepositoryFileOperations tests file operations on test repositories
func TestRepositoryFileOperations(t *testing.T) {
	t.Run("CreateAndModifyFiles", func(t *testing.T) {
		testRepo, err := NewTestRepository("/test/fileops")
		require.NoError(t, err)

		// Test CommitFile
		err = testRepo.CommitFile("test.txt", "test content", "Add test file")
		require.NoError(t, err)

		// Test CreateFile
		err = testRepo.CreateFile("another.txt", "more content")
		require.NoError(t, err)

		// Test branch operations
		err = testRepo.CreateBranch("feature")
		require.NoError(t, err)

		err = testRepo.CheckoutBranch("feature")
		require.NoError(t, err)

		// Verify worktree branch changed
		worktree, err := testRepo.ToWorktree("feature")
		require.NoError(t, err)
		assert.Equal(t, "feature", worktree.GetBranch())
	})
}
