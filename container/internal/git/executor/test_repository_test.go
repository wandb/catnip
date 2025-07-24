package executor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
	})
}
