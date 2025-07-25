//go:build integration

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/handlers"
	"github.com/vanpelt/catnip/test/integration/common"
)

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp int64                  `json:"timestamp"`
	ID        string                 `json:"id"`
}

// SSEEventMessage wraps the SSE event structure
type SSEEventMessage struct {
	Event SSEEvent `json:"event"`
}

// TestWorktreeCachePerformance tests the performance improvement of cached worktree status
func TestWorktreeCachePerformance(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository
	_ = ts.CreateTestRepository(t, "cache-perf-repo")

	// Create multiple worktrees to test scalability
	worktreeIDs := make([]string, 5)

	t.Log("Creating 5 worktrees to test cache performance...")
	for i := 0; i < 5; i++ {
		resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/cache-perf-repo", map[string]interface{}{})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create worktree %d: %s", i, string(body))

		var checkoutResp handlers.CheckoutResponse
		require.NoError(t, json.Unmarshal(body, &checkoutResp))
		worktreeIDs[i] = checkoutResp.Worktree.ID

		t.Logf("Created worktree %d: %s", i, checkoutResp.Worktree.Name)
	}

	// Wait a moment for cache to populate
	time.Sleep(2 * time.Second)

	// Benchmark ListWorktrees performance - should be fast with cache
	iterations := 10
	var totalDuration time.Duration

	t.Log("Benchmarking ListWorktrees endpoint performance...")
	for i := 0; i < iterations; i++ {
		start := time.Now()

		resp, body, err := ts.MakeRequest("GET", "/v1/git/worktrees", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		duration := time.Since(start)
		totalDuration += duration

		// Verify we get enhanced worktree responses
		var enhancedWorktrees []handlers.EnhancedWorktree
		require.NoError(t, json.Unmarshal(body, &enhancedWorktrees))

		assert.GreaterOrEqual(t, len(enhancedWorktrees), 5, "Should return at least 5 worktrees")

		// Check that worktrees have cache status metadata
		for _, wt := range enhancedWorktrees {
			assert.NotNil(t, wt.CacheStatus, "Worktree should have cache status metadata")
			t.Logf("Worktree %s: cached=%v, loading=%v",
				wt.Name, wt.CacheStatus.IsCached, wt.CacheStatus.IsLoading)
		}

		t.Logf("Iteration %d: %v", i+1, duration)
	}

	avgDuration := totalDuration / time.Duration(iterations)
	t.Logf("Average ListWorktrees response time: %v", avgDuration)

	// With caching, response time should be very fast (< 100ms even with 5 worktrees)
	assert.Less(t, avgDuration, 100*time.Millisecond,
		"Cached ListWorktrees should respond in under 100ms. Got: %v", avgDuration)

	// Clean up worktrees
	for _, worktreeID := range worktreeIDs {
		resp, _, err := ts.MakeRequest("DELETE", fmt.Sprintf("/v1/git/worktrees/%s", worktreeID), nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
}

// TestWorktreeCacheEvents tests real-time event updates when worktree status changes
func TestWorktreeCacheEvents(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository and worktree
	_ = ts.CreateTestRepository(t, "cache-events-repo")

	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/cache-events-repo", map[string]interface{}{})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))
	worktreeID := checkoutResp.Worktree.ID
	worktreeName := checkoutResp.Worktree.Name

	t.Logf("Created worktree for events test: %s (%s)", worktreeName, worktreeID)

	// Subscribe to SSE events
	eventsChan := make(chan SSEEventMessage, 100)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		subscribeToSSEEvents(ctx, t, ts, eventsChan)
	}()

	// Wait for SSE connection to establish
	time.Sleep(1 * time.Second)

	// Make the worktree dirty by creating a file
	t.Log("Making worktree dirty by creating a test file...")
	containerName := "catnip-test"
	worktreePath := checkoutResp.Worktree.Path

	createFileCmd := exec.Command("docker", "exec", containerName, "sh", "-c",
		fmt.Sprintf("echo 'test content' > %s/test-file.txt", worktreePath))
	require.NoError(t, createFileCmd.Run(), "Failed to create test file in worktree")

	// Wait for filesystem watcher to detect change and cache to update
	t.Log("Waiting for cache events...")

	var receivedStatusUpdate, receivedDirtyEvent bool
	timeout := time.After(10 * time.Second)

	for !receivedStatusUpdate || !receivedDirtyEvent {
		select {
		case event := <-eventsChan:
			t.Logf("Received SSE event: %s", event.Event.Type)

			switch event.Event.Type {
			case "worktree:status_updated":
				if payload, ok := event.Event.Payload["worktree_id"]; ok {
					if payload == worktreeID {
						receivedStatusUpdate = true
						t.Log("✅ Received worktree:status_updated event for our worktree")
					}
				}

			case "worktree:dirty":
				if payload, ok := event.Event.Payload["worktree_id"]; ok {
					if payload == worktreeID {
						receivedDirtyEvent = true
						t.Log("✅ Received worktree:dirty event for our worktree")
					}
				}

			case "worktree:batch_updated":
				t.Log("📦 Received worktree:batch_updated event")
				// Check if our worktree is in the batch
				if updates, ok := event.Event.Payload["updates"].(map[string]interface{}); ok {
					if _, exists := updates[worktreeID]; exists {
						receivedStatusUpdate = true
						t.Log("✅ Our worktree was included in batch update")
					}
				}
			}

		case <-timeout:
			t.Fatal("Timeout waiting for worktree cache events")
		}
	}

	// Verify the cache has been updated by checking ListWorktrees
	t.Log("Verifying ListWorktrees reflects the dirty status...")
	resp, body, err = ts.MakeRequest("GET", "/v1/git/worktrees", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var enhancedWorktrees []handlers.EnhancedWorktree
	require.NoError(t, json.Unmarshal(body, &enhancedWorktrees))

	// Find our worktree and verify it's marked as dirty
	found := false
	for _, wt := range enhancedWorktrees {
		if wt.ID == worktreeID {
			found = true
			assert.True(t, wt.IsDirty, "Worktree should be marked as dirty after file creation")
			// Cache may still be loading for new worktrees, so we just verify it has cache metadata
			assert.NotNil(t, wt.CacheStatus, "Worktree should have cache status metadata")
			t.Logf("✅ Worktree %s is properly marked as dirty: %v (cached: %v)", wt.Name, wt.IsDirty, wt.CacheStatus.IsCached)
			break
		}
	}
	assert.True(t, found, "Should find our worktree in the list")

	// Clean the worktree by committing the changes
	t.Log("Cleaning worktree by committing changes...")
	addCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", worktreePath, "add", ".")
	require.NoError(t, addCmd.Run(), "Failed to stage changes")

	// Configure git user for commit
	configUserCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", worktreePath, "config", "user.email", "test@example.com")
	require.NoError(t, configUserCmd.Run(), "Failed to configure git user email")

	configNameCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", worktreePath, "config", "user.name", "Test User")
	require.NoError(t, configNameCmd.Run(), "Failed to configure git user name")

	commitCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", worktreePath, "commit", "-m", "Test commit")
	require.NoError(t, commitCmd.Run(), "Failed to commit changes")

	// Wait for clean event or status update showing clean
	t.Log("Waiting for clean event...")
	var receivedCleanEvent bool
	timeout = time.After(15 * time.Second) // Increased timeout

	for !receivedCleanEvent {
		select {
		case event := <-eventsChan:
			t.Logf("Received event while waiting for clean: %s", event.Event.Type)

			if event.Event.Type == "worktree:clean" {
				if payload, ok := event.Event.Payload["worktree_id"]; ok {
					if payload == worktreeID {
						receivedCleanEvent = true
						t.Log("✅ Received worktree:clean event for our worktree")
					}
				}
			} else if event.Event.Type == "worktree:status_updated" {
				if payload, ok := event.Event.Payload["worktree_id"]; ok {
					if payload == worktreeID {
						t.Log("📊 Received status update for our worktree, checking if clean...")
						// Check current status via API
						resp, body, err := ts.MakeRequest("GET", "/v1/git/worktrees", nil)
						if err == nil && resp.StatusCode == http.StatusOK {
							var enhancedWorktrees []handlers.EnhancedWorktree
							if json.Unmarshal(body, &enhancedWorktrees) == nil {
								for _, wt := range enhancedWorktrees {
									if wt.ID == worktreeID {
										t.Logf("Current status: dirty=%v, cached=%v", wt.IsDirty, wt.CacheStatus.IsCached)
										if !wt.IsDirty {
											t.Log("✅ Worktree is now clean via status update")
											receivedCleanEvent = true
										}
										break
									}
								}
							}
						}
					}
				}
			}
		case <-timeout:
			t.Fatal("Timeout waiting for worktree:clean event")
		}
	}

	// Final verification
	resp, body, err = ts.MakeRequest("GET", "/v1/git/worktrees", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.Unmarshal(body, &enhancedWorktrees))
	for _, wt := range enhancedWorktrees {
		if wt.ID == worktreeID {
			assert.False(t, wt.IsDirty, "Worktree should be clean after commit")
			assert.Greater(t, wt.CommitCount, 0, "Worktree should have commits ahead")
			t.Logf("✅ Worktree %s is now clean with %d commits ahead", wt.Name, wt.CommitCount)
			break
		}
	}

	// Clean up
	resp, _, err = ts.MakeRequest("DELETE", fmt.Sprintf("/v1/git/worktrees/%s", worktreeID), nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Signal goroutine to stop
	cancel()
	wg.Wait()
	close(eventsChan)
}

// TestWorktreeCacheConsistency tests that cache updates maintain data consistency
func TestWorktreeCacheConsistency(t *testing.T) {
	ts := common.SetupTestSuite(t)
	defer ts.TearDown()

	// Create test repository and worktree
	_ = ts.CreateTestRepository(t, "cache-consistency-repo")

	resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/cache-consistency-repo", map[string]interface{}{})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var checkoutResp handlers.CheckoutResponse
	require.NoError(t, json.Unmarshal(body, &checkoutResp))
	worktreeID := checkoutResp.Worktree.ID

	t.Logf("Created worktree for consistency test: %s", checkoutResp.Worktree.Name)

	// Perform multiple rapid operations to test cache consistency
	operations := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Create file",
			fn: func() error {
				containerName := "catnip-test"
				cmd := exec.Command("docker", "exec", containerName, "sh", "-c",
					fmt.Sprintf("echo 'content %d' > %s/file_%d.txt",
						time.Now().UnixNano(), checkoutResp.Worktree.Path, time.Now().UnixNano()))
				return cmd.Run()
			},
		},
		{
			name: "Stage changes",
			fn: func() error {
				containerName := "catnip-test"
				cmd := exec.Command("docker", "exec", containerName, "/usr/bin/git",
					"-C", checkoutResp.Worktree.Path, "add", ".")
				return cmd.Run()
			},
		},
		{
			name: "Commit changes",
			fn: func() error {
				containerName := "catnip-test"
				worktreePath := checkoutResp.Worktree.Path

				// Configure git user for commit
				configUserCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", worktreePath, "config", "user.email", "test@example.com")
				if err := configUserCmd.Run(); err != nil {
					return err
				}

				configNameCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", worktreePath, "config", "user.name", "Test User")
				if err := configNameCmd.Run(); err != nil {
					return err
				}

				cmd := exec.Command("docker", "exec", containerName, "/usr/bin/git",
					"-C", worktreePath, "commit", "-m", fmt.Sprintf("Consistency test commit %d", time.Now().UnixNano()))
				return cmd.Run()
			},
		},
	}

	// Execute operations and check consistency after each
	for i, op := range operations {
		t.Logf("Executing operation %d: %s", i+1, op.name)

		require.NoError(t, op.fn(), "Operation %s failed", op.name)

		// Wait for cache to update
		time.Sleep(500 * time.Millisecond)

		// Check ListWorktrees consistency
		resp, body, err := ts.MakeRequest("GET", "/v1/git/worktrees", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var enhancedWorktrees []handlers.EnhancedWorktree
		require.NoError(t, json.Unmarshal(body, &enhancedWorktrees))

		// Find our worktree
		found := false
		for _, wt := range enhancedWorktrees {
			if wt.ID == worktreeID {
				found = true

				// Verify cache metadata is consistent
				assert.NotNil(t, wt.CacheStatus, "Cache status should be present")

				t.Logf("After %s: dirty=%v, commits=%d, cached=%v",
					op.name, wt.IsDirty, wt.CommitCount, wt.CacheStatus.IsCached)
				break
			}
		}
		assert.True(t, found, "Should find worktree after operation: %s", op.name)
	}

	// Clean up
	resp, _, err = ts.MakeRequest("DELETE", fmt.Sprintf("/v1/git/worktrees/%s", worktreeID), nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// subscribeToSSEEvents connects to the SSE endpoint and forwards events to a channel
func subscribeToSSEEvents(ctx context.Context, t *testing.T, ts *common.TestSuite, eventsChan chan<- SSEEventMessage) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", ts.BaseURL+"/v1/events", nil)
	require.NoError(t, err)

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "SSE endpoint should return 200")

	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			t.Log("SSE subscription cancelled by context")
			return
		default:
		}

		line := scanner.Text()

		// Skip empty lines and non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract JSON data
		jsonData := strings.TrimPrefix(line, "data: ")

		var sseMessage SSEEventMessage
		if err := json.Unmarshal([]byte(jsonData), &sseMessage); err != nil {
			t.Logf("Failed to parse SSE message: %v, data: %s", err, jsonData)
			continue
		}

		// Only forward worktree-related events
		eventType := sseMessage.Event.Type
		if strings.HasPrefix(eventType, "worktree:") {
			select {
			case eventsChan <- sseMessage:
			case <-time.After(1 * time.Second):
				t.Log("SSE events channel is full, dropping event")
			case <-ctx.Done():
				t.Log("SSE subscription cancelled while sending event")
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Logf("SSE scanner error: %v", err)
	}
}

// BenchmarkCachedListWorktrees benchmarks the performance of the cached ListWorktrees endpoint
func BenchmarkCachedListWorktrees(b *testing.B) {
	ts := common.SetupTestSuite(&testing.T{})
	defer ts.TearDown()

	// Create test repository and multiple worktrees
	_ = ts.CreateTestRepository(&testing.T{}, "benchmark-cache-repo")

	// Create 10 worktrees for a realistic benchmark
	worktreeIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		resp, body, err := ts.MakeRequest("POST", "/v1/git/checkout/testorg/benchmark-cache-repo", map[string]interface{}{})
		if err != nil {
			b.Fatalf("Failed to create worktree %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Failed to create worktree %d: status %d, body: %s", i, resp.StatusCode, string(body))
		}

		var checkoutResp handlers.CheckoutResponse
		if err := json.Unmarshal(body, &checkoutResp); err != nil {
			b.Fatalf("Failed to parse checkout response: %v", err)
		}
		worktreeIDs[i] = checkoutResp.Worktree.ID
	}

	// Wait for cache to populate
	time.Sleep(2 * time.Second)

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark the cached ListWorktrees endpoint
	for i := 0; i < b.N; i++ {
		resp, _, err := ts.MakeRequest("GET", "/v1/git/worktrees", nil)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}
	}

	b.StopTimer()

	// Clean up worktrees
	for _, worktreeID := range worktreeIDs {
		_, _, _ = ts.MakeRequest("DELETE", fmt.Sprintf("/v1/git/worktrees/%s", worktreeID), nil)
	}
}
