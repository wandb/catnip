package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vanpelt/catnip/internal/git/executor"
)

// TestOperationsConstruction tests Operations creation
func TestOperationsConstruction(t *testing.T) {
	t.Run("NewOperations", func(t *testing.T) {
		ops := NewOperations()
		assert.NotNil(t, ops)

		// Should use GitExecutor by default
		impl := ops.(*OperationsImpl)
		_, ok := impl.executor.(*executor.GitExecutor)
		assert.True(t, ok)
	})

	t.Run("NewOperationsWithExecutor", func(t *testing.T) {
		exec := executor.NewInMemoryExecutor()
		ops := NewOperationsWithExecutor(exec)
		assert.NotNil(t, ops)

		impl := ops.(*OperationsImpl)
		assert.Equal(t, exec, impl.executor)
	})
}

// TestOperationsWorktreeOperations tests operations interface worktree methods
func TestOperationsWorktreeOperations(t *testing.T) {
	ops := NewOperations()

	t.Run("CreateWorktree", func(t *testing.T) {
		// Test creating a worktree with invalid path (should fail)
		err := ops.CreateWorktree("/nonexistent/repo", "/test/worktree", "test-branch", "main")
		assert.Error(t, err) // Expected to fail
	})

	t.Run("ListWorktrees", func(t *testing.T) {
		// Test listing worktrees on non-existent repo
		_, err := ops.ListWorktrees("/nonexistent/repo")
		assert.Error(t, err) // Expected to fail
	})

	t.Run("RemoveWorktree", func(t *testing.T) {
		// Test removing a non-existent worktree
		err := ops.RemoveWorktree("/nonexistent/repo", "/test/worktree", false)
		assert.Error(t, err) // Expected to fail
	})

	t.Run("UtilityOperations", func(t *testing.T) {
		// Test utility operations
		isRepo := ops.IsGitRepository("/nonexistent/path")
		assert.False(t, isRepo)

		_, err := ops.GetGitRoot("/nonexistent/path")
		assert.Error(t, err)
	})
}
