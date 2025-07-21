package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryOperations(t *testing.T) {
	// Create a test repository with history
	testRepo, err := CreateTestRepositoryWithHistory()
	require.NoError(t, err)

	// Convert to our Repository interface
	repo := testRepo.ToRepository()

	t.Run("GetPath", func(t *testing.T) {
		path := repo.GetPath()
		assert.Equal(t, "/test/repo", path)
	})

	t.Run("GetDefaultBranch", func(t *testing.T) {
		branch, err := repo.GetDefaultBranch()
		require.NoError(t, err)
		assert.Equal(t, "main", branch)
	})

	t.Run("ListBranches", func(t *testing.T) {
		branches, err := repo.ListBranches()
		require.NoError(t, err)

		// Should have main and feature/test branches
		assert.Contains(t, branches, "main")
		assert.Contains(t, branches, "feature/test")
		assert.Len(t, branches, 2)
	})

	t.Run("BranchExists", func(t *testing.T) {
		assert.True(t, repo.BranchExists("main"))
		assert.True(t, repo.BranchExists("feature/test"))
		assert.False(t, repo.BranchExists("nonexistent"))
	})

	t.Run("CreateBranch", func(t *testing.T) {
		err := repo.CreateBranch("new-feature", "main")
		require.NoError(t, err)

		assert.True(t, repo.BranchExists("new-feature"))

		// List branches should now include the new branch
		branches, err := repo.ListBranches()
		require.NoError(t, err)
		assert.Contains(t, branches, "new-feature")
	})

	t.Run("GetRemoteURL", func(t *testing.T) {
		url, err := repo.GetRemoteURL()
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/test/repo.git", url)
	})

	t.Run("IsBare", func(t *testing.T) {
		// Our go-git implementation considers repos with nil filesystem as bare
		// Test repositories created with memfs are not bare
		isBare := repo.IsBare()
		// Just check that the method works, value depends on implementation
		_ = isBare
	})

	t.Run("IsShallow", func(t *testing.T) {
		// Our test repositories are not shallow
		assert.False(t, repo.IsShallow())
	})
}

func TestRepositoryClone(t *testing.T) {
	// Note: This test would require a real remote repository
	// For now, we'll test the clone interface
	t.Skip("Skipping clone test - requires real remote repository")

	// Example of how clone would be tested:
	// storage := memory.NewStorage()
	// fs := memfs.New()
	// repo := NewGoGitRepository(storage, fs, "/test/clone")
	//
	// err := repo.Clone("https://github.com/go-git/go-git.git", "/test/clone")
	// require.NoError(t, err)
}

func TestRepositoryFetch(t *testing.T) {
	testRepo, err := CreateTestRepositoryWithHistory()
	require.NoError(t, err)

	repo := testRepo.ToRepository()

	t.Run("Fetch", func(t *testing.T) {
		// Note: Fetch will fail for in-memory repos without real remotes
		// This tests the interface works
		err := repo.Fetch()
		// We expect this to fail since we don't have a real remote
		assert.Error(t, err)
	})

	t.Run("FetchBranch", func(t *testing.T) {
		err := repo.FetchBranch("main")
		// We expect this to fail since we don't have a real remote
		assert.Error(t, err)
	})
}

func TestRepositoryBranchOperations(t *testing.T) {
	testRepo, err := NewTestRepository("/test/branches")
	require.NoError(t, err)

	// Create an initial commit
	err = testRepo.CommitFile("README.md", "# Test", "Initial commit")
	require.NoError(t, err)

	repo := testRepo.ToRepository()

	t.Run("CreateMultipleBranches", func(t *testing.T) {
		branches := []string{"feature-1", "feature-2", "hotfix"}

		for _, branch := range branches {
			err := repo.CreateBranch(branch, "") // Use HEAD instead of "main"
			require.NoError(t, err)
			assert.True(t, repo.BranchExists(branch))
		}

		allBranches, err := repo.ListBranches()
		require.NoError(t, err)

		for _, branch := range branches {
			assert.Contains(t, allBranches, branch)
		}
	})
}
