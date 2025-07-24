package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitSyncService(t *testing.T) {
	// Skip if not in CI or test environment
	if os.Getenv("CI") == "" && os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=1 to run")
	}

	// Create test workspace
	tempDir := t.TempDir()
	oldWorkspace := os.Getenv("WORKSPACE_DIR")
	require.NoError(t, os.Setenv("WORKSPACE_DIR", tempDir))
	defer func() { _ = os.Setenv("WORKSPACE_DIR", oldWorkspace) }()

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "repos"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "worktrees"), 0755))

	// Create git service
	gitService := NewGitService()
	require.NotNil(t, gitService)

	// Create commit sync service
	commitSync := NewCommitSyncService(gitService)
	require.NotNil(t, commitSync)

	t.Run("NewCommitSyncService", func(t *testing.T) {
		assert.NotNil(t, commitSync.gitService)
		assert.Equal(t, 30*time.Second, commitSync.syncInterval)
		assert.False(t, commitSync.running)
	})

	t.Run("FindRepositoryForWorktree_NoWorktrees", func(t *testing.T) {
		repo, err := commitSync.findRepositoryForWorktree("/nonexistent/path")
		assert.Error(t, err)
		assert.Nil(t, repo)
		assert.Contains(t, err.Error(), "worktree not found for path")
	})
}

func TestCommitSyncServiceLifecycle(t *testing.T) {
	// Create git service
	gitService := NewGitService()
	require.NotNil(t, gitService)

	// Create commit sync service
	commitSync := NewCommitSyncService(gitService)
	require.NotNil(t, commitSync)

	t.Run("StartAndStop", func(t *testing.T) {
		// Start the service
		err := commitSync.Start()
		assert.NoError(t, err)
		assert.True(t, commitSync.running)

		// Try to start again - should error
		err = commitSync.Start()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already running")

		// Stop the service
		commitSync.Stop()

		// Give it a moment to stop
		time.Sleep(100 * time.Millisecond)
		assert.False(t, commitSync.running)
	})
}

func TestCommitInfo(t *testing.T) {
	t.Run("CommitInfoStructure", func(t *testing.T) {
		commitInfo := &CommitInfo{
			WorktreePath: "/test/path",
			CommitHash:   "abc123",
			Branch:       "main",
			Message:      "Test commit",
			Author:       "Test Author <test@example.com>",
			Timestamp:    time.Now(),
		}

		assert.Equal(t, "/test/path", commitInfo.WorktreePath)
		assert.Equal(t, "abc123", commitInfo.CommitHash)
		assert.Equal(t, "main", commitInfo.Branch)
		assert.Equal(t, "Test commit", commitInfo.Message)
		assert.Equal(t, "Test Author <test@example.com>", commitInfo.Author)
		assert.False(t, commitInfo.Timestamp.IsZero())
	})
}

// MockCommitSyncService for testing internal methods
type MockCommitSyncService struct {
	*CommitSyncService
	gitCommands [][]string
}

func NewMockCommitSyncService(gitService *GitService) *MockCommitSyncService {
	return &MockCommitSyncService{
		CommitSyncService: NewCommitSyncService(gitService),
		gitCommands:       [][]string{},
	}
}

func TestCommitSyncServiceMethods(t *testing.T) {
	// Create git service
	gitService := NewGitService()
	require.NotNil(t, gitService)

	// Create mock commit sync service
	commitSync := NewMockCommitSyncService(gitService)
	require.NotNil(t, commitSync)

	t.Run("ServiceCreation", func(t *testing.T) {
		assert.NotNil(t, commitSync.gitService)
		assert.NotNil(t, commitSync.stopChan)
		assert.Equal(t, 30*time.Second, commitSync.syncInterval)
		assert.False(t, commitSync.running)
		assert.Empty(t, commitSync.gitCommands)
	})

	t.Run("FindRepositoryForWorktree_Empty", func(t *testing.T) {
		repo, err := commitSync.findRepositoryForWorktree("/test/path")
		assert.Error(t, err)
		assert.Nil(t, repo)
	})
}
