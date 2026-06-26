package git

import (
	"os"
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

func (m *MockGitService) RefreshWorktreeStatus(workDir string) error {
	return nil
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

func (m *MockSessionService) UpdateSessionTitle(workDir, title, commitHash string) error {
	return nil
}

func (m *MockSessionService) GetPreviousTitle(workDir string) string {
	return ""
}

func (m *MockSessionService) UpdatePreviousTitleCommitHash(workDir string, commitHash string) error {
	return nil
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
	_ = os.Unsetenv("CATNIP_COMMIT_TIMEOUT_SECONDS")
	timeout := GetCheckpointTimeout()
	assert.Equal(t, DefaultCheckpointTimeoutSeconds*time.Second, timeout)

	// Test custom timeout
	_ = os.Setenv("CATNIP_COMMIT_TIMEOUT_SECONDS", "60")
	defer func() { _ = os.Unsetenv("CATNIP_COMMIT_TIMEOUT_SECONDS") }()
	timeout = GetCheckpointTimeout()
	assert.Equal(t, 60*time.Second, timeout)

	// Test invalid timeout (should use default)
	_ = os.Setenv("CATNIP_COMMIT_TIMEOUT_SECONDS", "invalid")
	timeout = GetCheckpointTimeout()
	assert.Equal(t, DefaultCheckpointTimeoutSeconds*time.Second, timeout)

	// Test zero timeout (should use default)
	_ = os.Setenv("CATNIP_COMMIT_TIMEOUT_SECONDS", "0")
	timeout = GetCheckpointTimeout()
	assert.Equal(t, DefaultCheckpointTimeoutSeconds*time.Second, timeout)
}

func TestIsCheckpointEnabled(t *testing.T) {
	// Test disabled by default
	_ = os.Unsetenv("CATNIP_ENABLE_CHECKPOINTS")
	enabled := IsCheckpointEnabled()
	assert.False(t, enabled)

	// Test enabled with "true"
	_ = os.Setenv("CATNIP_ENABLE_CHECKPOINTS", "true")
	defer func() { _ = os.Unsetenv("CATNIP_ENABLE_CHECKPOINTS") }()
	enabled = IsCheckpointEnabled()
	assert.True(t, enabled)

	// Test enabled with "1"
	_ = os.Setenv("CATNIP_ENABLE_CHECKPOINTS", "1")
	enabled = IsCheckpointEnabled()
	assert.True(t, enabled)

	// Test disabled with other values
	_ = os.Setenv("CATNIP_ENABLE_CHECKPOINTS", "false")
	enabled = IsCheckpointEnabled()
	assert.False(t, enabled)

	_ = os.Setenv("CATNIP_ENABLE_CHECKPOINTS", "0")
	enabled = IsCheckpointEnabled()
	assert.False(t, enabled)

	_ = os.Setenv("CATNIP_ENABLE_CHECKPOINTS", "yes")
	enabled = IsCheckpointEnabled()
	assert.False(t, enabled)
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
