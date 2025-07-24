package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/handlers"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/test/integration/common"
)

// TestClaudeSessionTitleHandling tests Claude session creation and title handling
func TestClaudeSessionTitleHandling(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository
	_ = ts.CreateTestRepository(t, "claude-test-repo")

	// First create a worktree
	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/claude-test-repo", map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))
	worktreePath := checkoutResp.Worktree.Path

	// Test Claude session creation with actual worktree path
	_, body, err = ts.MakeRequest("POST", "/v1/claude/messages", map[string]interface{}{
		"prompt":        "Create a new function called hello_world",
		"workspace":     worktreePath,
		"system_prompt": "You are a helpful coding assistant",
	})

	require.NoError(t, err)

	var claudeResp models.CreateCompletionResponse
	require.NoError(t, json.Unmarshal(body, &claudeResp))

	t.Logf("Claude response: %+v", claudeResp)

	// Test session summary retrieval with the actual worktree path
	resp, body, err = ts.MakeRequest("GET", "/v1/claude/session?worktree_path="+worktreePath, nil)
	require.NoError(t, err)

	t.Logf("Session summary response (%d): %s", resp.StatusCode, string(body))
}
