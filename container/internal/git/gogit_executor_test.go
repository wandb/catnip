package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoGitExecutor(t *testing.T) {
	executor := NewGoGitCommandExecutor()

	t.Run("SimpleCommands", func(t *testing.T) {
		// Test that fallback commands work (these will use shell git)
		_, err := executor.ExecuteCommand("echo", "hello")
		assert.NoError(t, err)
	})

	t.Run("GitCommandsWithoutRepo", func(t *testing.T) {
		// Commands that don't require a repo should still work
		_, err := executor.ExecuteGitWithWorkingDir("/tmp", "version")
		assert.NoError(t, err) // This should fallback to shell
	})

	t.Run("GitCommandsWithTestRepo", func(t *testing.T) {
		// Create a temporary test repository
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0755))

		// Initialize with shell git first
		fallback := NewGitCommandExecutor()
		_, err := fallback.ExecuteGitWithWorkingDir(repoDir, "init")
		require.NoError(t, err)

		// Configure git user
		_, err = fallback.ExecuteGitWithWorkingDir(repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)
		_, err = fallback.ExecuteGitWithWorkingDir(repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)

		// Create initial commit
		readmePath := filepath.Join(repoDir, "README.md")
		require.NoError(t, os.WriteFile(readmePath, []byte("# Test\n"), 0644))
		_, err = fallback.ExecuteGitWithWorkingDir(repoDir, "add", "README.md")
		require.NoError(t, err)
		_, err = fallback.ExecuteGitWithWorkingDir(repoDir, "commit", "-m", "Initial commit")
		require.NoError(t, err)

		// Now test go-git operations
		t.Run("Status", func(t *testing.T) {
			output, err := executor.ExecuteGitWithWorkingDir(repoDir, "status", "--porcelain")
			assert.NoError(t, err)
			assert.Equal(t, "", string(output)) // Clean repo
		})

		t.Run("Branch", func(t *testing.T) {
			output, err := executor.ExecuteGitWithWorkingDir(repoDir, "branch", "--show-current")
			assert.NoError(t, err)
			// Should have some branch name (master or main)
			assert.NotEmpty(t, string(output))
		})

		t.Run("BranchList", func(t *testing.T) {
			output, err := executor.ExecuteGitWithWorkingDir(repoDir, "branch")
			assert.NoError(t, err)
			// Should list at least one branch
			assert.Contains(t, string(output), "*")
		})

		t.Run("RevParse", func(t *testing.T) {
			output, err := executor.ExecuteGitWithWorkingDir(repoDir, "rev-parse", "HEAD")
			assert.NoError(t, err)
			// Should return a commit hash
			assert.Len(t, strings.TrimSpace(string(output)), 40) // 40 char commit hash
		})
	})

	t.Run("FallbackOperations", func(t *testing.T) {
		// These operations should fall back to shell git
		// We can't easily test them without setting up complex scenarios,
		// but we can verify they don't panic
		_, err := executor.ExecuteGitWithWorkingDir("/tmp", "merge", "--help")
		// This might fail but shouldn't panic
		_ = err
	})
}

func TestGoGitExecutorTypes(t *testing.T) {
	t.Run("Interface compliance", func(t *testing.T) {
		var _ CommandExecutor = (*GoGitCommandExecutor)(nil)
		_ = NewGoGitCommandExecutor() // Verify it implements CommandExecutor
	})

	t.Run("Factory function", func(t *testing.T) {
		executor := NewGoGitCommandExecutor()
		assert.NotNil(t, executor)

		goGitExec, ok := executor.(*GoGitCommandExecutor)
		assert.True(t, ok)
		assert.NotNil(t, goGitExec.fallbackExecutor)
		assert.NotNil(t, goGitExec.repositoryCache)
	})
}

// TestServiceHelperUsesGoGit verifies that our default service helper uses go-git
func TestServiceHelperUsesGoGit(t *testing.T) {
	helper := NewServiceHelper()
	assert.NotNil(t, helper)
	assert.NotNil(t, helper.Executor)

	// Check that it's our GoGitCommandExecutor
	_, ok := helper.Executor.(*GoGitCommandExecutor)
	assert.True(t, ok, "Default ServiceHelper should use GoGitCommandExecutor")
}

// TestShellServiceHelper verifies that shell service helper uses shell git
func TestShellServiceHelper(t *testing.T) {
	helper := NewShellServiceHelper()
	assert.NotNil(t, helper)
	assert.NotNil(t, helper.Executor)

	// Check that it's the shell CommandExecutorImpl
	_, ok := helper.Executor.(*CommandExecutorImpl)
	assert.True(t, ok, "Shell ServiceHelper should use CommandExecutorImpl")
}
