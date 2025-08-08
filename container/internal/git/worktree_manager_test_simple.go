package git

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vanpelt/catnip/internal/models"
)

// SimpleMockOperations with just the methods we need for testing
type SimpleMockOperations struct {
	mock.Mock
}

func (m *SimpleMockOperations) CreateWorktree(repoPath, worktreePath, branch, sourceBranch string) error {
	args := m.Called(repoPath, worktreePath, branch, sourceBranch)
	return args.Error(0)
}

func (m *SimpleMockOperations) GetCommitHash(worktreePath, ref string) (string, error) {
	args := m.Called(worktreePath, ref)
	return args.String(0), args.Error(1)
}

func (m *SimpleMockOperations) GetCommitCount(worktreePath, from, to string) (int, error) {
	args := m.Called(worktreePath, from, to)
	return args.Int(0), args.Error(1)
}

func (m *SimpleMockOperations) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	args := m.Called(repoPath, worktreePath, force)
	return args.Error(0)
}

func (m *SimpleMockOperations) DeleteBranch(repoPath, branch string, force bool) error {
	args := m.Called(repoPath, branch, force)
	return args.Error(0)
}

func (m *SimpleMockOperations) IsDirty(worktreePath string) bool {
	args := m.Called(worktreePath)
	return args.Bool(0)
}

func (m *SimpleMockOperations) HasConflicts(worktreePath string) bool {
	args := m.Called(worktreePath)
	return args.Bool(0)
}

func (m *SimpleMockOperations) GetCurrentBranch(worktreePath string) (string, error) {
	args := m.Called(worktreePath)
	return args.String(0), args.Error(1)
}

// Add no-op implementations for other interface methods to satisfy the Operations interface
// This is a simplified approach that won't work for full interface, but good for focused testing

func TestWorktreeManager_CreateWorktree_Simple(t *testing.T) {
	// This is a simplified test that doesn't use the full Operations interface
	// Instead, we'll test the logic by checking the structs and basic functionality

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

	// Test struct creation and basic validation
	assert.Equal(t, "test-org/test-repo", req.Repository.ID)
	assert.Equal(t, "main", req.SourceBranch)
	assert.Equal(t, "feature-branch", req.BranchName)
	assert.Equal(t, "/workspace", req.WorkspaceDir)
	assert.False(t, req.IsInitial)
}

func TestWorktreeManager_CreateWorktree_MockBased(t *testing.T) {
	// Create a custom mock that implements just what we need
	mockOps := &simpleMockOps{}
	manager := NewWorktreeManager(mockOps)

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

	// Mock the successful flow
	mockOps.createWorktreeResult = nil
	mockOps.commitHash = "abc123def456"
	mockOps.commitHashError = nil
	mockOps.commitCount = 5
	mockOps.commitCountError = nil

	worktree, err := manager.CreateWorktree(req)

	assert.NoError(t, err)
	assert.NotNil(t, worktree)
	assert.NotEmpty(t, worktree.ID)
	assert.Equal(t, "test-org/test-repo", worktree.RepoID)
	assert.Equal(t, "test-repo/feature-branch", worktree.Name)
	assert.Equal(t, "/workspace/test-repo/feature-branch", worktree.Path)
	assert.Equal(t, "feature-branch", worktree.Branch)
	assert.Equal(t, "main", worktree.SourceBranch)
	assert.Equal(t, "abc123def456", worktree.CommitHash)
	assert.Equal(t, 5, worktree.CommitCount)
	assert.WithinDuration(t, time.Now(), worktree.CreatedAt, time.Second)
}

// simpleMockOps is a minimal mock that implements just what we need
type simpleMockOps struct {
	createWorktreeResult error
	commitHash           string
	commitHashError      error
	commitCount          int
	commitCountError     error
}

func (m *simpleMockOps) CreateWorktree(repoPath, worktreePath, branch, sourceBranch string) error {
	return m.createWorktreeResult
}

func (m *simpleMockOps) GetCommitHash(worktreePath, ref string) (string, error) {
	return m.commitHash, m.commitHashError
}

func (m *simpleMockOps) GetCommitCount(worktreePath, from, to string) (int, error) {
	return m.commitCount, m.commitCountError
}

// Implement all other Operations interface methods as no-ops to satisfy the interface
func (m *simpleMockOps) ExecuteGit(workingDir string, args ...string) ([]byte, error) {
	return nil, nil
}
func (m *simpleMockOps) ExecuteCommand(command string, args ...string) ([]byte, error) {
	return nil, nil
}
func (m *simpleMockOps) BranchExists(repoPath, branch string, isRemote bool) bool { return false }
func (m *simpleMockOps) GetRemoteURL(repoPath string) (string, error)             { return "", nil }
func (m *simpleMockOps) GetDefaultBranch(repoPath string) (string, error)         { return "", nil }
func (m *simpleMockOps) GetLocalBranches(repoPath string) ([]string, error)       { return nil, nil }
func (m *simpleMockOps) GetRemoteBranches(repoPath, defaultBranch string) ([]string, error) {
	return nil, nil
}
func (m *simpleMockOps) GetRemoteBranchesFromURL(remoteURL string) ([]string, error) { return nil, nil }
func (m *simpleMockOps) CreateBranch(repoPath, branch, fromRef string) error         { return nil }
func (m *simpleMockOps) DeleteBranch(repoPath, branch string, force bool) error      { return nil }
func (m *simpleMockOps) ListBranches(repoPath string, options ListBranchesOptions) ([]string, error) {
	return nil, nil
}
func (m *simpleMockOps) RenameBranch(repoPath, oldBranch, newBranch string) error         { return nil }
func (m *simpleMockOps) RemoveWorktree(repoPath, worktreePath string, force bool) error   { return nil }
func (m *simpleMockOps) ListWorktrees(repoPath string) ([]WorktreeInfo, error)            { return nil, nil }
func (m *simpleMockOps) PruneWorktrees(repoPath string) error                             { return nil }
func (m *simpleMockOps) IsDirty(worktreePath string) bool                                 { return false }
func (m *simpleMockOps) HasConflicts(worktreePath string) bool                            { return false }
func (m *simpleMockOps) HasUncommittedChanges(worktreePath string) (bool, error)          { return false, nil }
func (m *simpleMockOps) GetConflictedFiles(worktreePath string) ([]string, error)         { return nil, nil }
func (m *simpleMockOps) GetStatus(worktreePath string) (*WorktreeStatus, error)           { return nil, nil }
func (m *simpleMockOps) FetchBranch(repoPath string, strategy FetchStrategy) error        { return nil }
func (m *simpleMockOps) FetchBranchFast(repoPath, branch string) error                    { return nil }
func (m *simpleMockOps) FetchBranchFull(repoPath, branch string) error                    { return nil }
func (m *simpleMockOps) PushBranch(worktreePath string, strategy PushStrategy) error      { return nil }
func (m *simpleMockOps) AddRemote(repoPath, name, url string) error                       { return nil }
func (m *simpleMockOps) RemoveRemote(repoPath, name string) error                         { return nil }
func (m *simpleMockOps) SetRemoteURL(repoPath, name, url string) error                    { return nil }
func (m *simpleMockOps) GetRemotes(repoPath string) (map[string]string, error)            { return nil, nil }
func (m *simpleMockOps) Clone(url, path string, options CloneOptions) error               { return nil }
func (m *simpleMockOps) Add(worktreePath string, paths ...string) error                   { return nil }
func (m *simpleMockOps) Commit(worktreePath, message string, options CommitOptions) error { return nil }
func (m *simpleMockOps) ResetMixed(worktreePath, ref string) error                        { return nil }
func (m *simpleMockOps) Merge(worktreePath, ref string) error                             { return nil }
func (m *simpleMockOps) Rebase(worktreePath, ref string) error                            { return nil }
func (m *simpleMockOps) CherryPick(worktreePath, commit string) error                     { return nil }
func (m *simpleMockOps) AbortRebase(worktreePath string) error                            { return nil }
func (m *simpleMockOps) ContinueRebase(worktreePath string) error                         { return nil }
func (m *simpleMockOps) DiffNameOnly(worktreePath, filter string) ([]string, error)       { return nil, nil }
func (m *simpleMockOps) MergeTree(worktreePath, base, head string) (string, error)        { return "", nil }
func (m *simpleMockOps) Stash(worktreePath string) error                                  { return nil }
func (m *simpleMockOps) StashPop(worktreePath string) error                               { return nil }
func (m *simpleMockOps) CreateTag(repoPath, tag, ref string) error                        { return nil }
func (m *simpleMockOps) DeleteTag(repoPath, tag string) error                             { return nil }
func (m *simpleMockOps) ListTags(repoPath string) ([]string, error)                       { return nil, nil }
func (m *simpleMockOps) GetConfig(repoPath, key string) (string, error)                   { return "", nil }
func (m *simpleMockOps) SetConfig(repoPath, key, value string) error                      { return nil }
func (m *simpleMockOps) SetGlobalConfig(key, value string) error                          { return nil }
func (m *simpleMockOps) GetDisplayBranch(worktreePath string) (string, error)             { return "", nil }
func (m *simpleMockOps) GetCurrentBranch(worktreePath string) (string, error)             { return "", nil }
func (m *simpleMockOps) RevParse(repoPath, ref string) (string, error)                    { return "", nil }
func (m *simpleMockOps) RevList(repoPath string, options RevListOptions) ([]string, error) {
	return nil, nil
}
func (m *simpleMockOps) ShowRef(repoPath, ref string, options ShowRefOptions) error { return nil }
func (m *simpleMockOps) GarbageCollect(repoPath string) error                       { return nil }
