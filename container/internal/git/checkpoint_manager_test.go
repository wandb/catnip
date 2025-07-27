package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGitService is a mock implementation of GitService for testing
type MockGitService struct {
	addCommitCalled bool
	lastCommitTitle string
	returnHash      string
	returnError     error
}

func (m *MockGitService) GitAddCommitGetHash(workDir, title string) (string, error) {
	m.addCommitCalled = true
	m.lastCommitTitle = title
	return m.returnHash, m.returnError
}

// MockSessionService is a mock implementation of SessionService for testing
type MockSessionService struct {
	addToHistoryCalled bool
	lastTitle          string
	lastCommitHash     string
	activeSession      interface{}
	sessionExists      bool
}

func (m *MockSessionService) AddToSessionHistory(workDir, title, commitHash string) error {
	m.addToHistoryCalled = true
	m.lastTitle = title
	m.lastCommitHash = commitHash
	return nil
}

func (m *MockSessionService) GetActiveSession(workDir string) (interface{}, bool) {
	return m.activeSession, m.sessionExists
}

func TestNewSessionCheckpointManager(t *testing.T) {
	workDir := "/test/workspace"
	gitService := &MockGitService{}
	sessionService := &MockSessionService{}

	cm := NewSessionCheckpointManager(workDir, gitService, sessionService)

	assert.NotNil(t, cm)
	assert.Equal(t, workDir, cm.workDir)
	assert.Equal(t, gitService, cm.gitService)
	assert.Equal(t, sessionService, cm.sessionService)
	assert.Equal(t, 0, cm.checkpointCount)
	assert.False(t, cm.lastCommitTime.IsZero())
}

func TestGetCheckpointTimeout(t *testing.T) {
	// Test default timeout
	os.Unsetenv("CATNIP_COMMIT_TIMEOUT_SECONDS")
	timeout := getCheckpointTimeout()
	assert.Equal(t, DefaultCheckpointTimeoutSeconds*time.Second, timeout)

	// Test custom timeout
	os.Setenv("CATNIP_COMMIT_TIMEOUT_SECONDS", "60")
	defer os.Unsetenv("CATNIP_COMMIT_TIMEOUT_SECONDS")
	timeout = getCheckpointTimeout()
	assert.Equal(t, 60*time.Second, timeout)

	// Test invalid timeout (should use default)
	os.Setenv("CATNIP_COMMIT_TIMEOUT_SECONDS", "invalid")
	timeout = getCheckpointTimeout()
	assert.Equal(t, DefaultCheckpointTimeoutSeconds*time.Second, timeout)

	// Test zero timeout (should use default)
	os.Setenv("CATNIP_COMMIT_TIMEOUT_SECONDS", "0")
	timeout = getCheckpointTimeout()
	assert.Equal(t, DefaultCheckpointTimeoutSeconds*time.Second, timeout)
}

func TestShouldCreateCheckpoint(t *testing.T) {
	cm := &SessionCheckpointManager{
		lastCommitTime: time.Now(),
	}

	// Should not create checkpoint immediately
	assert.False(t, cm.ShouldCreateCheckpoint())

	// Should create checkpoint after timeout
	cm.lastCommitTime = time.Now().Add(-31 * time.Second)
	assert.True(t, cm.ShouldCreateCheckpoint())
}

func TestCreateCheckpoint(t *testing.T) {
	t.Run("successful checkpoint", func(t *testing.T) {
		mockGit := &MockGitService{
			returnHash: "abc123",
		}
		mockSession := &MockSessionService{}

		cm := &SessionCheckpointManager{
			gitService:      mockGit,
			sessionService:  mockSession,
			workDir:         "/test/workspace",
			checkpointCount: 0,
			lastCommitTime:  time.Now().Add(-1 * time.Hour), // Old time
		}

		err := cm.CreateCheckpoint("Test Title")
		require.NoError(t, err)

		assert.True(t, mockGit.addCommitCalled)
		assert.Equal(t, "Test Title checkpoint: 1", mockGit.lastCommitTitle)
		assert.True(t, mockSession.addToHistoryCalled)
		assert.Equal(t, "Test Title checkpoint: 1", mockSession.lastTitle)
		assert.Equal(t, "abc123", mockSession.lastCommitHash)
		assert.Equal(t, 1, cm.checkpointCount)
		assert.True(t, time.Since(cm.lastCommitTime) < time.Second)
	})

	t.Run("no git service", func(t *testing.T) {
		cm := &SessionCheckpointManager{
			gitService: nil,
		}

		err := cm.CreateCheckpoint("Test Title")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "git service not available")
	})

	t.Run("empty commit hash", func(t *testing.T) {
		mockGit := &MockGitService{
			returnHash: "",
		}
		mockSession := &MockSessionService{}

		cm := &SessionCheckpointManager{
			gitService:      mockGit,
			sessionService:  mockSession,
			workDir:         "/test/workspace",
			checkpointCount: 0,
		}

		err := cm.CreateCheckpoint("Test Title")
		require.NoError(t, err)

		assert.True(t, mockGit.addCommitCalled)
		assert.False(t, mockSession.addToHistoryCalled) // Should not add to history if no commit
		assert.Equal(t, 0, cm.checkpointCount)          // Count should not increase
	})
}

func TestReset(t *testing.T) {
	cm := &SessionCheckpointManager{
		checkpointCount: 5,
		lastCommitTime:  time.Now().Add(-1 * time.Hour),
	}

	cm.Reset()

	assert.Equal(t, 0, cm.checkpointCount)
	assert.True(t, time.Since(cm.lastCommitTime) < time.Second)
}

func TestUpdateLastCommitTime(t *testing.T) {
	cm := &SessionCheckpointManager{
		lastCommitTime: time.Now().Add(-1 * time.Hour),
	}

	oldTime := cm.lastCommitTime
	cm.UpdateLastCommitTime()

	assert.True(t, cm.lastCommitTime.After(oldTime))
	assert.True(t, time.Since(cm.lastCommitTime) < time.Second)
}

func TestFileWatcher(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	cm := &SessionCheckpointManager{
		workDir:        tempDir,
		lastCommitTime: time.Now().Add(-1 * time.Hour), // Old time to trigger checkpoint
	}

	// Create subdirectories to test recursive watching
	subDir := filepath.Join(tempDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	// Create .git directory that should be ignored
	gitDir := filepath.Join(tempDir, ".git")
	err = os.Mkdir(gitDir, 0755)
	require.NoError(t, err)

	// Start file watcher
	err = cm.StartFileWatcher()
	require.NoError(t, err)
	defer cm.StopFileWatcher()

	// Set up a channel to detect when file change handler is called
	handlerCalled := make(chan bool, 1)
	cm.SetFileChangeHandler(func() {
		select {
		case handlerCalled <- true:
		default:
		}
	})

	// Create a file in the watched directory
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Wait for the file change handler to be called (with timeout)
	select {
	case <-handlerCalled:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("File change handler was not called within timeout")
	}
}

func TestDetectClaudeTitle(t *testing.T) {
	t.Run("active session with title", func(t *testing.T) {
		mockSession := &MockSessionService{
			activeSession: map[string]interface{}{
				"title": map[string]interface{}{
					"title": "Active Claude Session",
				},
			},
			sessionExists: true,
		}

		cm := &SessionCheckpointManager{
			sessionService: mockSession,
			workDir:        "/test/workspace",
		}

		title, err := cm.DetectClaudeTitle()
		require.NoError(t, err)
		assert.Equal(t, "Active Claude Session", title)
	})

	t.Run("no active session", func(t *testing.T) {
		mockSession := &MockSessionService{
			sessionExists: false,
		}

		cm := &SessionCheckpointManager{
			sessionService: mockSession,
			workDir:        "/test/workspace",
		}

		_, err := cm.DetectClaudeTitle()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no Claude session or title found")
	})
}

func TestAddWatchRecursive(t *testing.T) {
	tempDir := t.TempDir()

	// Create directory structure
	dirs := []string{
		"src",
		"src/components",
		".git",           // Should be skipped
		"node_modules",   // Should be skipped
		"dist",           // Should be skipped
		"build",          // Should be skipped
		".next",          // Should be skipped
	}

	for _, dir := range dirs {
		err := os.MkdirAll(filepath.Join(tempDir, dir), 0755)
		require.NoError(t, err)
	}

	cm := &SessionCheckpointManager{
		workDir: tempDir,
	}

	// Start watcher
	err := cm.StartFileWatcher()
	require.NoError(t, err)
	defer cm.StopFileWatcher()

	// Verify watcher is set up
	assert.NotNil(t, cm.watcher)
}

// TestConcurrentAccess tests thread safety of checkpoint manager
func TestConcurrentAccess(t *testing.T) {
	cm := &SessionCheckpointManager{
		lastCommitTime:  time.Now(),
		checkpointCount: 0,
	}

	// Run multiple goroutines that access and modify the checkpoint manager
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cm.ShouldCreateCheckpoint()
				cm.UpdateLastCommitTime()
				cm.Reset()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without panicking, the concurrent access is safe
}