package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/models"
)

// MockOperations is a mock implementation of Operations interface
type MockOperations struct {
	mock.Mock
}

func (m *MockOperations) Clone(url, path string, bare bool) error {
	args := m.Called(url, path, bare)
	return args.Error(0)
}

func (m *MockOperations) CreateWorktree(repoPath, worktreePath, branch, sourceBranch string) error {
	args := m.Called(repoPath, worktreePath, branch, sourceBranch)
	return args.Error(0)
}

func (m *MockOperations) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	args := m.Called(repoPath, worktreePath, force)
	return args.Error(0)
}

func (m *MockOperations) DeleteBranch(repoPath, branch string, force bool) error {
	args := m.Called(repoPath, branch, force)
	return args.Error(0)
}

func (m *MockOperations) IsDirty(worktreePath string) bool {
	args := m.Called(worktreePath)
	return args.Bool(0)
}

func (m *MockOperations) HasConflicts(worktreePath string) bool {
	args := m.Called(worktreePath)
	return args.Bool(0)
}

func (m *MockOperations) GetCommitHash(worktreePath, ref string) (string, error) {
	args := m.Called(worktreePath, ref)
	return args.String(0), args.Error(1)
}

func (m *MockOperations) GetCommitCount(worktreePath, from, to string) (int, error) {
	args := m.Called(worktreePath, from, to)
	return args.Int(0), args.Error(1)
}

func (m *MockOperations) IsWorkingDirectoryDirty(worktreePath string) (bool, error) {
	args := m.Called(worktreePath)
	return args.Bool(0), args.Error(1)
}

func (m *MockOperations) HasConflicts(worktreePath string) (bool, error) {
	args := m.Called(worktreePath)
	return args.Bool(0), args.Error(1)
}

func (m *MockOperations) ListBranches(repoPath string, remote bool) ([]string, error) {
	args := m.Called(repoPath, remote)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockOperations) GetCurrentBranch(worktreePath string) (string, error) {
	args := m.Called(worktreePath)
	return args.String(0), args.Error(1)
}

func (m *MockOperations) BranchExists(repoPath, branch string) (bool, error) {
	args := m.Called(repoPath, branch)
	return args.Bool(0), args.Error(1)
}

func (m *MockOperations) GetWorktreeStatus(worktreePath string) (*WorktreeStatus, error) {
	args := m.Called(worktreePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*WorktreeStatus), args.Error(1)
}

func (m *MockOperations) Fetch(worktreePath string) error {
	args := m.Called(worktreePath)
	return args.Error(0)
}

func (m *MockOperations) Pull(worktreePath string) error {
	args := m.Called(worktreePath)
	return args.Error(0)
}

func (m *MockOperations) Push(worktreePath, branch string) error {
	args := m.Called(worktreePath, branch)
	return args.Error(0)
}

func (m *MockOperations) Merge(worktreePath, branch string) error {
	args := m.Called(worktreePath, branch)
	return args.Error(0)
}

func (m *MockOperations) Rebase(worktreePath, branch string) error {
	args := m.Called(worktreePath, branch)
	return args.Error(0)
}

func TestNewWorktreeManager(t *testing.T) {
	mockOps := new(MockOperations)
	manager := NewWorktreeManager(mockOps)

	assert.NotNil(t, manager)
	assert.Equal(t, mockOps, manager.operations)
}

func TestWorktreeManager_CreateWorktree(t *testing.T) {
	mockOps := new(MockOperations)
	manager := NewWorktreeManager(mockOps)

	t.Run("successful creation", func(t *testing.T) {
		repo := &models.Repository{
			ID:   "test-org/test-repo",
			Path: "/repos/test-org_test-repo.git",
		}

		req := CreateWorktreeRequest{
			Repository:   repo,
			SourceBranch: "main",
			BranchName:   "feature-branch",
			WorkspaceDir: "/workspace",
			IsInitial:    false,
		}

		expectedWorktreePath := "/workspace/test-repo/feature-branch"
		expectedCommitHash := "abc123def456"

		mockOps.On("CreateWorktree", "/repos/test-org_test-repo.git", expectedWorktreePath, "feature-branch", "main").Return(nil)
		mockOps.On("GetCommitHash", expectedWorktreePath, "HEAD").Return(expectedCommitHash, nil)
		mockOps.On("GetCommitCount", expectedWorktreePath, "main", "HEAD").Return(5, nil)

		worktree, err := manager.CreateWorktree(req)

		assert.NoError(t, err)
		assert.NotNil(t, worktree)
		assert.NotEmpty(t, worktree.ID) // UUID should be generated
		assert.Equal(t, "test-org/test-repo", worktree.RepoID)
		assert.Equal(t, "test-repo/feature-branch", worktree.Name)
		assert.Equal(t, expectedWorktreePath, worktree.Path)
		assert.Equal(t, "feature-branch", worktree.Branch)
		assert.Equal(t, "main", worktree.SourceBranch)
		assert.Equal(t, expectedCommitHash, worktree.CommitHash)
		assert.Equal(t, 5, worktree.CommitCount)
		assert.False(t, worktree.IsDirty)
		assert.False(t, worktree.HasConflicts)
		assert.WithinDuration(t, time.Now(), worktree.CreatedAt, time.Second)
		assert.WithinDuration(t, time.Now(), worktree.LastAccessed, time.Second)

		mockOps.AssertExpectations(t)
	})

	t.Run("create worktree fails", func(t *testing.T) {
		repo := &models.Repository{
			ID:   "test-org/test-repo",
			Path: "/repos/test-org_test-repo.git",
		}

		req := CreateWorktreeRequest{
			Repository:   repo,
			SourceBranch: "main",
			BranchName:   "feature-branch",
			WorkspaceDir: "/workspace",
			IsInitial:    false,
		}

		expectedWorktreePath := "/workspace/test-repo/feature-branch"

		mockOps.On("CreateWorktree", "/repos/test-org_test-repo.git", expectedWorktreePath, "feature-branch", "main").Return(assert.AnError)

		worktree, err := manager.CreateWorktree(req)

		assert.Error(t, err)
		assert.Nil(t, worktree)
		assert.Contains(t, err.Error(), "failed to create worktree")

		mockOps.AssertExpectations(t)
	})

	t.Run("get commit hash fails", func(t *testing.T) {
		repo := &models.Repository{
			ID:   "test-org/test-repo",
			Path: "/repos/test-org_test-repo.git",
		}

		req := CreateWorktreeRequest{
			Repository:   repo,
			SourceBranch: "main",
			BranchName:   "feature-branch",
			WorkspaceDir: "/workspace",
			IsInitial:    false,
		}

		expectedWorktreePath := "/workspace/test-repo/feature-branch"

		mockOps.On("CreateWorktree", "/repos/test-org_test-repo.git", expectedWorktreePath, "feature-branch", "main").Return(nil)
		mockOps.On("GetCommitHash", expectedWorktreePath, "HEAD").Return("", assert.AnError)

		worktree, err := manager.CreateWorktree(req)

		assert.Error(t, err)
		assert.Nil(t, worktree)
		assert.Contains(t, err.Error(), "failed to get commit hash")

		mockOps.AssertExpectations(t)
	})

	t.Run("commit hash as source branch", func(t *testing.T) {
		repo := &models.Repository{
			ID:   "test-org/test-repo",
			Path: "/repos/test-org_test-repo.git",
		}

		// Use a 40-character commit hash as source branch
		commitHashSource := "1234567890abcdef1234567890abcdef12345678"
		req := CreateWorktreeRequest{
			Repository:   repo,
			SourceBranch: commitHashSource,
			BranchName:   "feature-branch",
			WorkspaceDir: "/workspace",
			IsInitial:    false,
		}

		expectedWorktreePath := "/workspace/test-repo/feature-branch"
		expectedCommitHash := "abc123def456"

		mockOps.On("CreateWorktree", "/repos/test-org_test-repo.git", expectedWorktreePath, "feature-branch", commitHashSource).Return(nil)
		mockOps.On("GetCommitHash", expectedWorktreePath, "HEAD").Return(expectedCommitHash, nil)
		// When source branch is same as branch name, no commit count is calculated
		// (this would happen if findSourceBranch returns the same branch)

		worktree, err := manager.CreateWorktree(req)

		assert.NoError(t, err)
		assert.NotNil(t, worktree)
		assert.Equal(t, "feature-branch", worktree.Branch)
		// Source branch should be resolved from commit hash (mocked to return something)
		// In real scenario, findSourceBranch would resolve this

		mockOps.AssertExpectations(t)
	})
}

func TestWorktreeManager_CreateLocalWorktree(t *testing.T) {
	mockOps := new(MockOperations)
	manager := NewWorktreeManager(mockOps)

	t.Run("successful local creation", func(t *testing.T) {
		repo := &models.Repository{
			ID:   "local-repo",
			Path: "/path/to/local/repo",
		}

		req := CreateWorktreeRequest{
			Repository:   repo,
			SourceBranch: "main",
			BranchName:   "feature-branch",
			WorkspaceDir: "/workspace",
			IsInitial:    false,
		}

		expectedWorktreePath := "/workspace/local-repo/feature-branch"
		expectedCommitHash := "abc123def456"

		mockOps.On("CreateWorktree", "/path/to/local/repo", expectedWorktreePath, "feature-branch", "main").Return(nil)
		mockOps.On("GetCommitHash", expectedWorktreePath, "HEAD").Return(expectedCommitHash, nil)
		mockOps.On("GetCommitCount", expectedWorktreePath, "main", "HEAD").Return(3, nil)

		worktree, err := manager.CreateLocalWorktree(req)

		assert.NoError(t, err)
		assert.NotNil(t, worktree)
		assert.NotEmpty(t, worktree.ID)
		assert.Equal(t, "local-repo", worktree.RepoID)
		assert.Contains(t, worktree.Name, "feature-branch") // Contains because local repos have different naming
		assert.Equal(t, expectedWorktreePath, worktree.Path)
		assert.Equal(t, "feature-branch", worktree.Branch)

		mockOps.AssertExpectations(t)
	})
}

func TestWorktreeManager_DeleteWorktree(t *testing.T) {
	mockOps := new(MockOperations)
	manager := NewWorktreeManager(mockOps)

	worktree := &models.Worktree{
		ID:           "test-id",
		Path:         "/workspace/test-repo/feature-branch",
		Branch:       "feature-branch",
		SourceBranch: "main",
	}

	repo := &models.Repository{
		ID:   "test-org/test-repo",
		Path: "/repos/test-org_test-repo.git",
	}

	t.Run("successful deletion", func(t *testing.T) {
		mockOps.On("RemoveWorktree", "/repos/test-org_test-repo.git", "/workspace/test-repo/feature-branch", true).Return(nil)
		mockOps.On("DeleteBranch", "/repos/test-org_test-repo.git", "feature-branch", true).Return(nil)
		mockOps.On("DeleteBranch", "/repos/test-org_test-repo.git", "catnip/feature-branch", true).Return(assert.AnError) // Preview branch doesn't exist

		err := manager.DeleteWorktree(worktree, repo)

		assert.NoError(t, err)
		mockOps.AssertExpectations(t)
	})

	t.Run("removal fails but continues", func(t *testing.T) {
		mockOps.On("RemoveWorktree", "/repos/test-org_test-repo.git", "/workspace/test-repo/feature-branch", true).Return(assert.AnError)
		mockOps.On("DeleteBranch", "/repos/test-org_test-repo.git", "feature-branch", true).Return(nil)
		mockOps.On("DeleteBranch", "/repos/test-org_test-repo.git", "catnip/feature-branch", true).Return(assert.AnError)

		err := manager.DeleteWorktree(worktree, repo)

		assert.NoError(t, err) // Method doesn't fail even if some steps fail
		mockOps.AssertExpectations(t)
	})
}

func TestWorktreeManager_UpdateWorktreeStatus(t *testing.T) {
	mockOps := new(MockOperations)
	manager := NewWorktreeManager(mockOps)

	worktree := &models.Worktree{
		ID:           "test-id",
		Path:         "/workspace/test-repo/feature-branch",
		Branch:       "feature-branch",
		SourceBranch: "main",
	}

	t.Run("successful status update", func(t *testing.T) {
		mockOps.On("IsDirty", "/workspace/test-repo/feature-branch").Return(true)
		mockOps.On("HasConflicts", "/workspace/test-repo/feature-branch").Return(false)
		mockOps.On("GetCurrentBranch", "/workspace/test-repo/feature-branch").Return("feature-branch", nil)
		mockOps.On("GetCommitHash", "/workspace/test-repo/feature-branch", "HEAD").Return("new-commit-hash", nil)
		mockOps.On("GetCommitCount", "/workspace/test-repo/feature-branch", "origin/main", "HEAD").Return(7, nil)

		getSourceRef := func(w *models.Worktree) string {
			return "origin/" + w.SourceBranch
		}

		manager.UpdateWorktreeStatus(worktree, getSourceRef)

		assert.True(t, worktree.IsDirty)
		assert.False(t, worktree.HasConflicts)
		assert.Equal(t, "new-commit-hash", worktree.CommitHash)
		assert.Equal(t, 7, worktree.CommitCount)
		assert.WithinDuration(t, time.Now(), worktree.LastAccessed, time.Second)

		mockOps.AssertExpectations(t)
	})

	t.Run("branch detection fails", func(t *testing.T) {
		mockOps.On("IsDirty", "/workspace/test-repo/feature-branch").Return(false)
		mockOps.On("HasConflicts", "/workspace/test-repo/feature-branch").Return(false)
		mockOps.On("GetCurrentBranch", "/workspace/test-repo/feature-branch").Return("", assert.AnError)

		getSourceRef := func(w *models.Worktree) string {
			return "origin/" + w.SourceBranch
		}

		manager.UpdateWorktreeStatus(worktree, getSourceRef)

		assert.False(t, worktree.IsDirty)
		assert.False(t, worktree.HasConflicts)
		// Should still work despite branch detection failure

		mockOps.AssertExpectations(t)
	})
}

func TestExtractWorkspaceName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"feature-branch", "feature-branch"},
		{"refs/heads/feature-branch", "feature-branch"},
		{"origin/feature-branch", "feature-branch"},
		{"remotes/origin/feature-branch", "feature-branch"},
		{"main", "main"},
		{"feature/sub-branch", "feature-sub-branch"},
		{"fix/issue-123", "fix-issue-123"},
		{"", ""},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := ExtractWorkspaceName(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestCreateWorktreeRequest_struct(t *testing.T) {
	repo := &models.Repository{
		ID:   "test-org/test-repo",
		Path: "/repos/test-org_test-repo.git",
	}

	req := CreateWorktreeRequest{
		Repository:   repo,
		SourceBranch: "main",
		BranchName:   "feature-branch",
		WorkspaceDir: "/workspace",
		IsInitial:    true,
	}

	assert.Equal(t, repo, req.Repository)
	assert.Equal(t, "main", req.SourceBranch)
	assert.Equal(t, "feature-branch", req.BranchName)
	assert.Equal(t, "/workspace", req.WorkspaceDir)
	assert.True(t, req.IsInitial)
}

func TestWorktreeManager_Integration(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a mock bare repository directory
	repoDir := filepath.Join(tmpDir, "test-repo.git")
	require.NoError(t, os.MkdirAll(repoDir, 0755))

	// Create workspace directory
	workspaceDir := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0755))

	mockOps := new(MockOperations)
	manager := NewWorktreeManager(mockOps)

	repo := &models.Repository{
		ID:   "test-org/test-repo",
		Path: repoDir,
	}

	req := CreateWorktreeRequest{
		Repository:   repo,
		SourceBranch: "main",
		BranchName:   "feature-test",
		WorkspaceDir: workspaceDir,
		IsInitial:    false,
	}

	expectedWorktreePath := filepath.Join(workspaceDir, "test-repo", "feature-test")
	expectedCommitHash := "integration-test-hash"

	// Setup mocks for integration test
	mockOps.On("CreateWorktree", repoDir, expectedWorktreePath, "feature-test", "main").Return(nil)
	mockOps.On("GetCommitHash", expectedWorktreePath, "HEAD").Return(expectedCommitHash, nil)
	mockOps.On("GetCommitCount", expectedWorktreePath, "main", "HEAD").Return(2, nil)

	worktree, err := manager.CreateWorktree(req)

	assert.NoError(t, err)
	assert.NotNil(t, worktree)
	assert.Equal(t, expectedWorktreePath, worktree.Path)
	assert.Equal(t, "test-repo/feature-test", worktree.Name)
	assert.Equal(t, 2, worktree.CommitCount)

	mockOps.AssertExpectations(t)
}
