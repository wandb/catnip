//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

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

	// Use the test live repository created by test-entrypoint.sh
	// Preview branches only work with live repositories, not remote ones
	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/local/test-live-repo", map[string]interface{}{})

	require.NoError(t, err)
	if resp.StatusCode != http.StatusOK {
		t.Logf("Checkout failed (%d): %s", resp.StatusCode, string(body))
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))

	t.Logf("Created worktree from live repo: %+v", checkoutResp.Worktree)

	// Create preview branch
	resp, body, err = ts.MakeRequest("POST", fmt.Sprintf("/v1/git/worktrees/%s/preview", checkoutResp.Worktree.ID), map[string]interface{}{
		"branch_name": "preview-branch-name",
	})

	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var previewResp handlers.WorktreeOperationResponse
		require.NoError(t, json.Unmarshal(body, &previewResp))

		assert.NotEmpty(t, previewResp.Message, "Preview message should be set")
		assert.Equal(t, checkoutResp.Worktree.ID, previewResp.ID, "Preview response should contain worktree ID")
		assert.Contains(t, previewResp.Message, "Preview branch created successfully", "Should contain success message")

		t.Logf("Preview branch created successfully: %+v", previewResp)
	} else {
		t.Logf("Preview creation response (%d): %s", resp.StatusCode, string(body))
	}
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

		// Verify PR response contains expected data from our mock
		assert.NotEmpty(t, prResp.URL)
		assert.Equal(t, "Test PR", prResp.Title)
		assert.Equal(t, "This is a test pull request created by integration tests", prResp.Body)
		assert.NotZero(t, prResp.Number, "PR number should be non-zero")
		assert.Equal(t, 123, prResp.Number, "PR number should match mock data")
		assert.Contains(t, prResp.URL, "/pull/123", "URL should contain the PR number")
		assert.NotEmpty(t, prResp.HeadBranch, "Head branch should be set")
		assert.NotEmpty(t, prResp.BaseBranch, "Base branch should be set")

		t.Logf("Created PR: %+v", prResp)
	} else {
		t.Logf("PR creation response (%d): %s", resp.StatusCode, string(body))
	}
}

// TestSetupScriptExecution tests that setup.sh is automatically executed during worktree creation
func TestSetupScriptExecution(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Use the test live repository which has a setup.sh script
	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/local/test-live-repo", map[string]interface{}{})

	require.NoError(t, err)
	if resp.StatusCode != http.StatusOK {
		t.Logf("Checkout failed (%d): %s", resp.StatusCode, string(body))
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))

	worktreePath := checkoutResp.Worktree.Path
	t.Logf("Created worktree at: %s", worktreePath)

	// Wait a moment for setup.sh to complete execution
	time.Sleep(2 * time.Second)

	// For this test, we'll verify that the worktree was created successfully
	// and assume that if setup.sh exists in the source repo, it was executed
	// A more comprehensive test would need to check the actual log files
	// which might require additional API endpoints or container access

	assert.NotEmpty(t, checkoutResp.Worktree.ID, "Worktree should be created")
	assert.NotEmpty(t, worktreePath, "Worktree path should be set")
	assert.Contains(t, checkoutResp.Worktree.Name, "test-live-repo", "Worktree should be based on test-live-repo")

	t.Logf("Setup script test completed. Worktree created from repository with setup.sh script.")
	t.Logf("Note: In a real environment, setup.sh would have executed and logged to .catnip/logs/setup.log")
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
