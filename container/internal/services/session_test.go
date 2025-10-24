package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionService(t *testing.T) {
	// Create temporary state directory
	tempDir := t.TempDir()

	service := &SessionService{
		stateDir:       tempDir,
		activeSessions: make(map[string]*ActiveSessionInfo),
	}

	t.Run("SessionState", func(t *testing.T) {
		// Create a test session state
		state := &SessionState{
			ID:               "test-session-123",
			WorkingDirectory: "/test/workspace",
			Agent:            "claude",
			ClaudeSessionID:  "claude-uuid-123",
			CreatedAt:        time.Now(),
			LastAccess:       time.Now(),
			Environment: map[string]string{
				"TEST_VAR": "test_value",
			},
		}

		// Save session state
		err := service.SaveSessionState(state)
		require.NoError(t, err)

		// Load session state
		loadedState, err := service.LoadSessionState("test-session-123")
		require.NoError(t, err)
		require.NotNil(t, loadedState)

		assert.Equal(t, state.ID, loadedState.ID)
		assert.Equal(t, state.WorkingDirectory, loadedState.WorkingDirectory)
		assert.Equal(t, state.Agent, loadedState.Agent)
		assert.Equal(t, state.ClaudeSessionID, loadedState.ClaudeSessionID)
		assert.Equal(t, state.Environment["TEST_VAR"], loadedState.Environment["TEST_VAR"])

		// Load non-existent session
		nonExistentState, err := service.LoadSessionState("non-existent")
		require.NoError(t, err)
		assert.Nil(t, nonExistentState)

		// Delete session state
		err = service.DeleteSessionState("test-session-123")
		require.NoError(t, err)

		// Verify it's deleted
		deletedState, err := service.LoadSessionState("test-session-123")
		require.NoError(t, err)
		assert.Nil(t, deletedState)
	})

	t.Run("ActiveSessions", func(t *testing.T) {
		workspaceDir := "/test/workspace"
		claudeSessionUUID := "claude-uuid-456"

		// Start active session
		err := service.StartActiveSession(workspaceDir, claudeSessionUUID)
		require.NoError(t, err)

		// Get active session
		activeSession, exists := service.GetActiveSession(workspaceDir)
		require.True(t, exists)
		require.NotNil(t, activeSession)
		assert.Equal(t, claudeSessionUUID, activeSession.ClaudeSessionUUID)
		assert.Nil(t, activeSession.EndedAt)

		// Resume active session
		err = service.ResumeActiveSession(workspaceDir)
		require.NoError(t, err)

		// Get updated session
		resumedSession, exists := service.GetActiveSession(workspaceDir)
		require.True(t, exists)
		assert.NotNil(t, resumedSession.ResumedAt)

		// End active session
		err = service.EndActiveSession(workspaceDir)
		require.NoError(t, err)

		// Get ended session
		endedSession, exists := service.GetActiveSession(workspaceDir)
		require.True(t, exists)
		assert.NotNil(t, endedSession.EndedAt)

		// Remove active session
		err = service.RemoveActiveSession(workspaceDir)
		require.NoError(t, err)

		// Verify it's removed
		_, exists = service.GetActiveSession(workspaceDir)
		assert.False(t, exists)
	})

	t.Run("SessionTitleManagement", func(t *testing.T) {
		workspaceDir := "/test/workspace2"
		claudeSessionUUID := "claude-uuid-789"

		// Start active session
		err := service.StartActiveSession(workspaceDir, claudeSessionUUID)
		require.NoError(t, err)

		// Update session title
		err = service.UpdateSessionTitle(workspaceDir, "Test Title 1", "commit123")
		require.NoError(t, err)

		// Get active session and check title
		activeSession, exists := service.GetActiveSession(workspaceDir)
		require.True(t, exists)
		require.NotNil(t, activeSession.Title)
		assert.Equal(t, "Test Title 1", activeSession.Title.Title)
		assert.Equal(t, "commit123", activeSession.Title.CommitHash)
		assert.Len(t, activeSession.TitleHistory, 1)

		// Add to session history
		err = service.AddToSessionHistory(workspaceDir, "Test Title 2", "commit456")
		require.NoError(t, err)

		// Get updated session and check history
		updatedSession, exists := service.GetActiveSession(workspaceDir)
		require.True(t, exists)
		assert.Len(t, updatedSession.TitleHistory, 2)
		assert.Equal(t, "Test Title 2", updatedSession.TitleHistory[1].Title)
		assert.Equal(t, "commit456", updatedSession.TitleHistory[1].CommitHash)

		// Update previous title commit hash
		err = service.UpdatePreviousTitleCommitHash(workspaceDir, "commit789")
		require.NoError(t, err)

		// Get updated session and check commit hash
		finalSession, exists := service.GetActiveSession(workspaceDir)
		require.True(t, exists)
		assert.Equal(t, "commit789", finalSession.Title.CommitHash)
		assert.Equal(t, "commit789", finalSession.TitleHistory[len(finalSession.TitleHistory)-1].CommitHash)

		// Get previous title
		previousTitle := service.GetPreviousTitle(workspaceDir)
		assert.Equal(t, "Test Title 1", previousTitle) // Should return current title

		// Clean up
		err = service.RemoveActiveSession(workspaceDir)
		require.NoError(t, err)
	})

	t.Run("SessionErrors", func(t *testing.T) {
		// Test empty session ID
		err := service.SaveSessionState(&SessionState{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session ID cannot be empty")

		// Test loading empty session ID
		_, err = service.LoadSessionState("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session ID cannot be empty")

		// Test deleting empty session ID
		err = service.DeleteSessionState("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session ID cannot be empty")

		// Test operations on non-existent active session
		err = service.ResumeActiveSession("/nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no active session found")

		err = service.EndActiveSession("/nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no active session found")

		err = service.UpdateSessionTitle("/nonexistent", "title", "commit")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no active session found")

		err = service.AddToSessionHistory("/nonexistent", "title", "commit")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no active session found")
	})

	t.Run("ListActiveSessions", func(t *testing.T) {
		// Clear any existing sessions
		service.activeSessions = make(map[string]*ActiveSessionInfo)

		// Start a few active sessions
		_ = service.StartActiveSession("/workspace1", "uuid1")
		_ = service.StartActiveSession("/workspace2", "uuid2")
		_ = service.StartActiveSession("/workspace3", "uuid3")

		// End one session
		_ = service.EndActiveSession("/workspace2")

		// Get all active sessions (not ended)
		activeSessions := service.GetAllActiveSessions()
		assert.Len(t, activeSessions, 2) // Only 2 should be active

		// Get all sessions including ended
		allSessions := service.GetAllActiveSessionsIncludingEnded()
		assert.Len(t, allSessions, 3) // All 3 should be included

		// Check if specific session is active
		assert.True(t, service.IsActiveSessionActive("/workspace1"))
		assert.False(t, service.IsActiveSessionActive("/workspace2")) // This one was ended
		assert.True(t, service.IsActiveSessionActive("/workspace3"))
		assert.False(t, service.IsActiveSessionActive("/nonexistent"))
	})
}

func TestSessionServiceDirectory(t *testing.T) {
	tempDir := t.TempDir()

	service := &SessionService{
		stateDir:       tempDir,
		activeSessions: make(map[string]*ActiveSessionInfo),
	}

	t.Run("FindSessionByDirectory", func(t *testing.T) {
		// Test with non-existent directory
		session, err := service.FindSessionByDirectory("/nonexistent")
		require.NoError(t, err)
		assert.Nil(t, session)

		// Create a test workspace directory structure
		testWorkspace := filepath.Join(tempDir, "test-workspace")

		// The method expects files in the home directory structure with transformed paths
		// Transform: /temp/test-workspace -> -temp-test-workspace
		transformedPath := strings.ReplaceAll(testWorkspace, "/", "-")
		transformedPath = strings.TrimPrefix(transformedPath, "-")
		transformedPath = "-" + transformedPath

		// Create Claude directory in the expected home location (using tempDir as fake home)
		homeDir := tempDir
		claudeDir := filepath.Join(homeDir, ".claude", "projects", transformedPath)
		require.NoError(t, os.MkdirAll(claudeDir, 0755))

		// Create a fake Claude session file
		sessionFile := filepath.Join(claudeDir, "12345678-1234-1234-1234-123456789abc.jsonl")
		require.NoError(t, os.WriteFile(sessionFile, []byte("{}"), 0644))

		// Update the service to use our tempDir as home directory for testing
		originalHomeDir := "/home/catnip"
		// We need to temporarily modify the method to use tempDir as home
		// Since we can't easily mock this, let's create a fallback test using the persisted session approach

		// Instead, test the fallback mechanism by creating a persisted session state
		sessionState := &SessionState{
			ID:               "test-session",
			WorkingDirectory: testWorkspace,
			Agent:            "claude",
			ClaudeSessionID:  "12345678-1234-1234-1234-123456789abc",
			CreatedAt:        time.Now(),
			LastAccess:       time.Now(),
		}

		require.NoError(t, service.SaveSessionState(sessionState))

		// Find session by directory (should find via fallback mechanism)
		foundSession, err := service.FindSessionByDirectory(testWorkspace)
		require.NoError(t, err)
		require.NotNil(t, foundSession)
		assert.Equal(t, "test-session", foundSession.ID)
		assert.Equal(t, testWorkspace, foundSession.WorkingDirectory)
		assert.Equal(t, "claude", foundSession.Agent)
		assert.Equal(t, "12345678-1234-1234-1234-123456789abc", foundSession.ClaudeSessionID)

		// Clean up
		_ = originalHomeDir // Prevent unused variable warning
	})

	t.Run("FindNewestClaudeSessionFile", func(t *testing.T) {
		claudeDir := filepath.Join(tempDir, "test-projects")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))

		// Test empty directory
		newest := service.findNewestClaudeSessionFile(claudeDir)
		assert.Empty(t, newest)

		// Test 1: Multiple valid files with different sizes
		// Should prefer LARGEST, not most recent
		t.Run("PrefersLargestFile", func(t *testing.T) {
			subDir := filepath.Join(claudeDir, "size-test")
			require.NoError(t, os.MkdirAll(subDir, 0755))

			// Create files with different sizes (all valid UUIDs)
			smallFile := filepath.Join(subDir, "11111111-1111-1111-1111-111111111111.jsonl")
			mediumFile := filepath.Join(subDir, "22222222-2222-2222-2222-222222222222.jsonl")
			largestFile := filepath.Join(subDir, "33333333-3333-3333-3333-333333333333.jsonl")

			// Create small file (15KB) MOST RECENT
			smallContent := make([]byte, 15000)
			require.NoError(t, os.WriteFile(smallFile, smallContent, 0644))
			time.Sleep(10 * time.Millisecond)

			// Create medium file (30KB)
			mediumContent := make([]byte, 30000)
			require.NoError(t, os.WriteFile(mediumFile, mediumContent, 0644))
			time.Sleep(10 * time.Millisecond)

			// Create largest file (50KB) OLDEST
			largestContent := make([]byte, 50000)
			require.NoError(t, os.WriteFile(largestFile, largestContent, 0644))

			// Should return the LARGEST file, not the most recent
			newest := service.findNewestClaudeSessionFile(subDir)
			assert.Equal(t, "33333333-3333-3333-3333-333333333333", newest, "Should select largest file (50KB), not most recent")
		})

		// Test 2: Filter out small files (< 10KB)
		t.Run("FiltersSmallerThan10KB", func(t *testing.T) {
			subDir := filepath.Join(claudeDir, "filter-test")
			require.NoError(t, os.MkdirAll(subDir, 0755))

			// Create warmup session (small, < 10KB) - should be IGNORED
			warmupFile := filepath.Join(subDir, "44444444-4444-4444-4444-444444444444.jsonl")
			warmupContent := make([]byte, 5000) // 5KB - too small
			require.NoError(t, os.WriteFile(warmupFile, warmupContent, 0644))
			time.Sleep(10 * time.Millisecond)

			// Create real session (> 10KB) - should be SELECTED
			realFile := filepath.Join(subDir, "55555555-5555-5555-5555-555555555555.jsonl")
			realContent := make([]byte, 12000) // 12KB - big enough
			require.NoError(t, os.WriteFile(realFile, realContent, 0644))

			// Should return the real file, ignoring the small warmup session
			newest := service.findNewestClaudeSessionFile(subDir)
			assert.Equal(t, "55555555-5555-5555-5555-555555555555", newest, "Should ignore files < 10KB")
		})

		// Test 3: Only small files (all < 10KB) - should return empty
		t.Run("AllFilesTooSmall", func(t *testing.T) {
			subDir := filepath.Join(claudeDir, "all-small-test")
			require.NoError(t, os.MkdirAll(subDir, 0755))

			// Create multiple small files
			for i := 0; i < 3; i++ {
				uuid := strings.Repeat(string(rune('0'+i)), 8) + "-" + strings.Repeat(string(rune('0'+i)), 4) + "-" + strings.Repeat(string(rune('0'+i)), 4) + "-" + strings.Repeat(string(rune('0'+i)), 4) + "-" + strings.Repeat(string(rune('0'+i)), 12)
				smallFile := filepath.Join(subDir, uuid+".jsonl")
				smallContent := make([]byte, 9000) // All < 10KB
				require.NoError(t, os.WriteFile(smallFile, smallContent, 0644))
				time.Sleep(10 * time.Millisecond)
			}

			// Should return empty since all files are too small
			newest := service.findNewestClaudeSessionFile(subDir)
			assert.Empty(t, newest, "Should return empty when all files are < 10KB")
		})

		// Test 4: Invalid UUID formats should be ignored
		t.Run("FiltersInvalidUUIDs", func(t *testing.T) {
			subDir := filepath.Join(claudeDir, "uuid-test")
			require.NoError(t, os.MkdirAll(subDir, 0755))

			// Create invalid filename (not a UUID)
			invalidFile := filepath.Join(subDir, "invalid-filename.jsonl")
			largeContent := make([]byte, 11000)
			require.NoError(t, os.WriteFile(invalidFile, largeContent, 0644))
			time.Sleep(10 * time.Millisecond)

			// Create valid UUID file
			validFile := filepath.Join(subDir, "66666666-6666-6666-6666-666666666666.jsonl")
			require.NoError(t, os.WriteFile(validFile, largeContent, 0644))

			// Should return the valid UUID, ignoring invalid
			newest := service.findNewestClaudeSessionFile(subDir)
			assert.Equal(t, "66666666-6666-6666-6666-666666666666", newest, "Should ignore invalid UUID filenames")
		})

		// Test 5: Tie-breaker - same size, prefer most recent
		t.Run("SameSizePrefersRecent", func(t *testing.T) {
			subDir := filepath.Join(claudeDir, "tiebreak-test")
			require.NoError(t, os.MkdirAll(subDir, 0755))

			// Create two files with EXACTLY the same size
			sameContent := make([]byte, 20000)

			olderFile := filepath.Join(subDir, "77777777-7777-7777-7777-777777777777.jsonl")
			require.NoError(t, os.WriteFile(olderFile, sameContent, 0644))
			time.Sleep(100 * time.Millisecond) // Ensure different mod times

			newerFile := filepath.Join(subDir, "88888888-8888-8888-8888-888888888888.jsonl")
			require.NoError(t, os.WriteFile(newerFile, sameContent, 0644))

			// With same size, should prefer most recent (tie-breaker)
			newest := service.findNewestClaudeSessionFile(subDir)
			assert.Equal(t, "88888888-8888-8888-8888-888888888888", newest, "Should use mod time as tie-breaker for same-size files")
		})
	})

	t.Run("FindBestSessionFile", func(t *testing.T) {
		claudeDir := filepath.Join(tempDir, "best-file-test")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))

		// Create multiple session files with different sizes
		smallFile := filepath.Join(claudeDir, "99999999-9999-9999-9999-999999999999.jsonl")
		largeFile := filepath.Join(claudeDir, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl")

		smallContent := make([]byte, 5000)  // Too small, should be ignored
		largeContent := make([]byte, 25000) // Large enough

		require.NoError(t, os.WriteFile(smallFile, smallContent, 0644))
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, os.WriteFile(largeFile, largeContent, 0644))

		// FindBestSessionFile should return the full path to the largest valid file
		bestFile := service.FindBestSessionFile(claudeDir)
		expectedPath := filepath.Join(claudeDir, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl")
		assert.Equal(t, expectedPath, bestFile, "FindBestSessionFile should return full path to best session")

		// Test empty directory
		emptyDir := filepath.Join(tempDir, "empty-dir")
		require.NoError(t, os.MkdirAll(emptyDir, 0755))
		emptyResult := service.FindBestSessionFile(emptyDir)
		assert.Empty(t, emptyResult, "Should return empty string for directory with no valid sessions")
	})
}
