package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/test/integration/common"
)

// TestAutoCommitting tests git status and auto-commit functionality
func TestAutoCommitting(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository
	repoPath := ts.CreateTestRepository(t, "commit-test-repo")

	// Create a test file in the repository
	testFile := filepath.Join(repoPath, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Test git status to see uncommitted changes
	resp, body, err := ts.MakeRequest("GET", "/v1/git/status", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var statusResp models.GitStatus
	require.NoError(t, json.Unmarshal(body, &statusResp))

	t.Logf("Git status: %+v", statusResp)
}

// TestGitHubRepositoriesListing tests GitHub repository listing
func TestGitHubRepositoriesListing(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Test listing GitHub repositories
	resp, body, err := ts.MakeRequest("GET", "/v1/git/github/repos", nil)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var repos []models.Repository
		require.NoError(t, json.Unmarshal(body, &repos))

		assert.NotEmpty(t, repos)

		t.Logf("Found %d repositories", len(repos))
		for _, repo := range repos {
			t.Logf("Repository: %+v", repo)
		}
	} else {
		t.Logf("GitHub repos response (%d): %s", resp.StatusCode, string(body))
	}
}
