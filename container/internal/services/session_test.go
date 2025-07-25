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

		// Create multiple session files with different timestamps
		file1 := filepath.Join(claudeDir, "11111111-1111-1111-1111-111111111111.jsonl")
		file2 := filepath.Join(claudeDir, "22222222-2222-2222-2222-222222222222.jsonl")
		file3 := filepath.Join(claudeDir, "invalid-filename.jsonl")

		// Create files with different modification times
		require.NoError(t, os.WriteFile(file1, []byte("{}"), 0644))
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, os.WriteFile(file2, []byte("{}"), 0644))
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, os.WriteFile(file3, []byte("{}"), 0644))

		// Should return the newest valid UUID file
		newest = service.findNewestClaudeSessionFile(claudeDir)
		assert.Equal(t, "22222222-2222-2222-2222-222222222222", newest)
	})
}
