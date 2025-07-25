package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/handlers"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/test/integration/common"
)

// TestWorktreeCreation tests the worktree creation API
func TestWorktreeCreation(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository
	_ = ts.CreateTestRepository(t, "test-repo")

	// Test checkout repository (the API doesn't use custom branch names, uses refs/catnip/{name})
	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/test-repo", map[string]interface{}{})

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))

	assert.NotEmpty(t, checkoutResp.Worktree.ID)
	assert.Contains(t, checkoutResp.Worktree.Branch, "refs/catnip/") // Branch follows catnip naming convention

	t.Logf("Created worktree: %+v", checkoutResp)
}

// TestPreviewBranchCreation tests preview branch creation
func TestPreviewBranchCreation(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository and worktree
	_ = ts.CreateTestRepository(t, "preview-test-repo")

	// First create a worktree
	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/preview-test-repo", map[string]interface{}{})

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))

	// Create preview branch
	_, body, err = ts.MakeRequest("POST", fmt.Sprintf("/v1/git/worktrees/%s/preview", checkoutResp.Worktree.ID), map[string]interface{}{
		"branch_name": "preview-branch-name",
	})

	require.NoError(t, err)

	t.Logf("Preview creation response: %s", string(body))
}

// TestPRCreation tests pull request creation
func TestPRCreation(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository and worktree
	_ = ts.CreateTestRepository(t, "pr-test-repo")

	// Create a worktree with some changes
	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/pr-test-repo", map[string]interface{}{})

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))

	// Test PR creation
	resp, body, err = ts.MakeRequest("POST", fmt.Sprintf("/v1/git/worktrees/%s/pr", checkoutResp.Worktree.ID), map[string]interface{}{
		"title": "Test PR",
		"body":  "This is a test pull request created by integration tests",
	})

	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var prResp models.PullRequestResponse
		require.NoError(t, json.Unmarshal(body, &prResp))

		assert.NotEmpty(t, prResp.URL)
		assert.Equal(t, "Test PR", prResp.Title)

		t.Logf("Created PR: %+v", prResp)
	} else {
		t.Logf("PR creation response (%d): %s", resp.StatusCode, string(body))
	}
}

// TestUpstreamSyncing tests upstream syncing functionality
func TestUpstreamSyncing(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository and worktree
	_ = ts.CreateTestRepository(t, "sync-test-repo")

	// Create a worktree
	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/sync-test-repo", map[string]interface{}{})

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))

	// Test sync check
	resp, body, err = ts.MakeRequest("GET", fmt.Sprintf("/v1/git/worktrees/%s/sync/check", checkoutResp.Worktree.ID), nil)
	require.NoError(t, err)

	t.Logf("Sync check response (%d): %s", resp.StatusCode, string(body))

	// Test actual sync
	resp, body, err = ts.MakeRequest("POST", fmt.Sprintf("/v1/git/worktrees/%s/sync", checkoutResp.Worktree.ID), nil)
	require.NoError(t, err)

	t.Logf("Sync response (%d): %s", resp.StatusCode, string(body))
}

// BenchmarkWorktreeCreation benchmarks worktree creation performance
func BenchmarkWorktreeCreation(b *testing.B) {
	ts := common.SetupTestSuite(&testing.T{})
	defer ts.TearDown()

	// Create test repository once
	_ = ts.CreateTestRepository(&testing.T{}, "benchmark-repo")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		branchName := fmt.Sprintf("benchmark-branch-%d", i)

		resp, _, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/benchmark-repo", map[string]interface{}{
			"branch": branchName,
			"create": true,
		})

		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Unexpected status code: %d", resp.StatusCode)
		}
	}
}
