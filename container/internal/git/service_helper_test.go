package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/git/executor"
)

// TestServiceHelperTypes tests different service helper constructors
func TestServiceHelperTypes(t *testing.T) {
	t.Run("DefaultServiceHelper", func(t *testing.T) {
		helper := NewServiceHelper()
		assert.NotNil(t, helper)
		assert.NotNil(t, helper.Executor)

		// Should use GitExecutor by default
		_, ok := helper.Executor.(*executor.GitExecutor)
		assert.True(t, ok)
	})

	t.Run("ShellServiceHelper", func(t *testing.T) {
		helper := NewShellServiceHelper()
		assert.NotNil(t, helper)
		assert.NotNil(t, helper.Executor)

		// Should use ShellExecutor for shell operations
		_, ok := helper.Executor.(*executor.ShellExecutor)
		assert.True(t, ok)
	})

	t.Run("InMemoryServiceHelper", func(t *testing.T) {
		helper := NewInMemoryServiceHelper()
		assert.NotNil(t, helper)

		inMemoryExec, ok := helper.Executor.(*executor.InMemoryExecutor)
		require.True(t, ok)
		assert.NotNil(t, inMemoryExec)

		// Test repository creation
		repo, err := inMemoryExec.CreateRepository("/test/memory")
		assert.NoError(t, err)
		assert.NotNil(t, repo)
	})
}
