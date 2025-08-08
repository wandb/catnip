package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// MockGitService is a mock implementation of GitService
type MockGitService struct {
	mock.Mock
}

func (m *MockGitService) CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error) {
	args := m.Called(org, repo, branch)
	if args.Error(2) != nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*models.Repository), args.Get(1).(*models.Worktree), nil
}

func (m *MockGitService) GetStatus() *models.GitStatus {
	args := m.Called()
	return args.Get(0).(*models.GitStatus)
}

func (m *MockGitService) ListWorktrees() []*models.Worktree {
	args := m.Called()
	return args.Get(0).([]*models.Worktree)
}

func (m *MockGitService) IsWorktreeStatusCached(id string) bool {
	args := m.Called(id)
	return args.Bool(0)
}

func (m *MockGitService) GetWorktree(id string) (*models.Worktree, bool) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, false
	}
	return args.Get(0).(*models.Worktree), args.Bool(1)
}

func (m *MockGitService) UpdateWorktreeFields(id string, updates map[string]interface{}) error {
	args := m.Called(id, updates)
	return args.Error(0)
}

func (m *MockGitService) ListGitHubRepositories() ([]map[string]interface{}, error) {
	args := m.Called()
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]map[string]interface{}), nil
}

func (m *MockGitService) GetRepositoryBranches(repoID string) ([]string, error) {
	args := m.Called(repoID)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), nil
}

func (m *MockGitService) CreateWorktree(repoID, branchName, baseBranch string) (*models.Worktree, error) {
	args := m.Called(repoID, branchName, baseBranch)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Worktree), nil
}

func (m *MockGitService) DeleteWorktree(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockGitService) CheckConflicts(worktreeID string, operation string) (bool, []string, error) {
	args := m.Called(worktreeID, operation)
	return args.Bool(0), args.Get(1).([]string), args.Error(2)
}

func (m *MockGitService) SyncWorktree(worktreeID, strategy string) error {
	args := m.Called(worktreeID, strategy)
	return args.Error(0)
}

func (m *MockGitService) MergeWorktree(worktreeID, strategy string) error {
	args := m.Called(worktreeID, strategy)
	return args.Error(0)
}

func (m *MockGitService) GetWorktreeDiff(worktreeID string) (string, []string, int, int, error) {
	args := m.Called(worktreeID)
	return args.String(0), args.Get(1).([]string), args.Int(2), args.Int(3), args.Error(4)
}

// MockSessionService is a mock implementation of SessionService
type MockSessionService struct {
	mock.Mock
}

func (m *MockSessionService) GetActiveSession(path string) (*services.SessionInfo, bool) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, false
	}
	return args.Get(0).(*services.SessionInfo), args.Bool(1)
}

func (m *MockSessionService) GetClaudeActivityState(path string) models.ClaudeActivityState {
	args := m.Called(path)
	return args.Get(0).(models.ClaudeActivityState)
}

// MockClaudeMonitorService is a mock implementation of ClaudeMonitorService
type MockClaudeMonitorService struct {
	mock.Mock
}

func (m *MockClaudeMonitorService) GetTodos(path string) ([]models.Todo, error) {
	args := m.Called(path)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Todo), nil
}

// Setup test helper
func setupGitHandlerTest() (*GitHandler, *MockGitService, *MockSessionService, *MockClaudeMonitorService, *fiber.App) {
	mockGitService := new(MockGitService)
	mockSessionService := new(MockSessionService)
	mockClaudeMonitor := new(MockClaudeMonitorService)

	handler := NewGitHandler(
		(*services.GitService)(nil), // We'll use the mock directly
		(*services.GitHTTPService)(nil),
		(*services.SessionService)(nil),
		(*services.ClaudeMonitorService)(nil),
	)

	// Replace with mocks
	handler.gitService = (*services.GitService)(mockGitService)
	handler.sessionService = (*services.SessionService)(mockSessionService)
	handler.claudeMonitor = (*services.ClaudeMonitorService)(mockClaudeMonitor)

	app := fiber.New()

	return handler, mockGitService, mockSessionService, mockClaudeMonitor, app
}

func TestCheckoutRepository(t *testing.T) {
	handler, mockGitService, _, _, app := setupGitHandlerTest()

	t.Run("successful checkout", func(t *testing.T) {
		expectedRepo := &models.Repository{
			ID:   "repo-123",
			Name: "test-repo",
			Org:  "test-org",
		}
		expectedWorktree := &models.Worktree{
			ID:     "wt-123",
			Branch: "main",
			Path:   "/workspace/test-repo",
		}

		mockGitService.On("CheckoutRepository", "test-org", "test-repo", "main").
			Return(expectedRepo, expectedWorktree, nil)

		app.Post("/v1/git/checkout/:org/:repo", handler.CheckoutRepository)

		req := httptest.NewRequest("POST", "/v1/git/checkout/test-org/test-repo?branch=main", nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "Repository checked out successfully", result["message"])
		assert.NotNil(t, result["repository"])
		assert.NotNil(t, result["worktree"])

		mockGitService.AssertExpectations(t)
	})

	t.Run("checkout with error", func(t *testing.T) {
		mockGitService := new(MockGitService)
		handler.gitService = (*services.GitService)(mockGitService)

		mockGitService.On("CheckoutRepository", "test-org", "bad-repo", "").
			Return((*models.Repository)(nil), (*models.Worktree)(nil), fmt.Errorf("repository not found"))

		app.Post("/v1/git/checkout/:org/:repo", handler.CheckoutRepository)

		req := httptest.NewRequest("POST", "/v1/git/checkout/test-org/bad-repo", nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, 500, resp.StatusCode)

		var result map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "repository not found", result["error"])

		mockGitService.AssertExpectations(t)
	})
}

func TestGetStatus(t *testing.T) {
	handler, mockGitService, _, _, app := setupGitHandlerTest()

	expectedStatus := &models.GitStatus{
		Repository: &models.Repository{
			ID:   "repo-123",
			Name: "test-repo",
			Org:  "test-org",
		},
		CurrentWorktree: &models.Worktree{
			ID:     "wt-123",
			Branch: "main",
			Path:   "/workspace/test-repo",
		},
	}

	mockGitService.On("GetStatus").Return(expectedStatus)

	app.Get("/v1/git/status", handler.GetStatus)

	req := httptest.NewRequest("GET", "/v1/git/status", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result models.GitStatus
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	assert.Equal(t, expectedStatus.Repository.ID, result.Repository.ID)
	assert.Equal(t, expectedStatus.CurrentWorktree.ID, result.CurrentWorktree.ID)

	mockGitService.AssertExpectations(t)
}

func TestListWorktrees(t *testing.T) {
	handler, mockGitService, mockSessionService, mockClaudeMonitor, app := setupGitHandlerTest()

	worktrees := []*models.Worktree{
		{
			ID:     "wt-1",
			Branch: "main",
			Path:   "/workspace/repo/main",
		},
		{
			ID:     "wt-2",
			Branch: "feature",
			Path:   "/workspace/repo/feature",
		},
	}

	mockGitService.On("ListWorktrees").Return(worktrees)
	mockGitService.On("IsWorktreeStatusCached", "wt-1").Return(true)
	mockGitService.On("IsWorktreeStatusCached", "wt-2").Return(false)

	// Mock session service calls
	mockSessionService.On("GetActiveSession", "/workspace/repo/main").Return((*services.SessionInfo)(nil), false)
	mockSessionService.On("GetActiveSession", "/workspace/repo/feature").Return((*services.SessionInfo)(nil), false)
	mockSessionService.On("GetClaudeActivityState", "/workspace/repo/main").Return(models.ClaudeIdle)
	mockSessionService.On("GetClaudeActivityState", "/workspace/repo/feature").Return(models.ClaudeActive)

	// Mock todos
	mockClaudeMonitor.On("GetTodos", "/workspace/repo/main").Return([]models.Todo{}, nil)
	mockClaudeMonitor.On("GetTodos", "/workspace/repo/feature").Return([]models.Todo{
		{ID: "todo-1", Content: "Test todo", Status: "pending"},
	}, nil)

	app.Get("/v1/git/worktrees", handler.ListWorktrees)

	req := httptest.NewRequest("GET", "/v1/git/worktrees", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result []EnhancedWorktree
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	assert.Len(t, result, 2)
	assert.Equal(t, "wt-1", result[0].ID)
	assert.True(t, result[0].CacheStatus.IsCached)
	assert.False(t, result[0].CacheStatus.IsLoading)

	assert.Equal(t, "wt-2", result[1].ID)
	assert.False(t, result[1].CacheStatus.IsCached)
	assert.True(t, result[1].CacheStatus.IsLoading)
	assert.Len(t, result[1].Todos, 1)

	mockGitService.AssertExpectations(t)
	mockSessionService.AssertExpectations(t)
	mockClaudeMonitor.AssertExpectations(t)
}

func TestUpdateWorktree(t *testing.T) {
	handler, mockGitService, _, _, app := setupGitHandlerTest()

	t.Run("successful update", func(t *testing.T) {
		updates := map[string]interface{}{
			"branch": "new-feature",
			"status": "active",
		}

		updatedWorktree := &models.Worktree{
			ID:     "wt-123",
			Branch: "new-feature",
			Path:   "/workspace/test-repo",
			Status: "active",
		}

		mockGitService.On("UpdateWorktreeFields", "wt-123", updates).Return(nil)
		mockGitService.On("GetWorktree", "wt-123").Return(updatedWorktree, true)

		app.Patch("/v1/git/worktrees/:id", handler.UpdateWorktree)

		body, _ := json.Marshal(updates)
		req := httptest.NewRequest("PATCH", "/v1/git/worktrees/wt-123", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result models.Worktree
		respBody, _ := io.ReadAll(resp.Body)
		json.Unmarshal(respBody, &result)

		assert.Equal(t, "wt-123", result.ID)
		assert.Equal(t, "new-feature", result.Branch)
		assert.Equal(t, "active", result.Status)

		mockGitService.AssertExpectations(t)
	})

	t.Run("worktree not found", func(t *testing.T) {
		mockGitService := new(MockGitService)
		handler.gitService = (*services.GitService)(mockGitService)

		updates := map[string]interface{}{"branch": "new-feature"}

		mockGitService.On("UpdateWorktreeFields", "non-existent", updates).Return(nil)
		mockGitService.On("GetWorktree", "non-existent").Return((*models.Worktree)(nil), false)

		app.Patch("/v1/git/worktrees/:id", handler.UpdateWorktree)

		body, _ := json.Marshal(updates)
		req := httptest.NewRequest("PATCH", "/v1/git/worktrees/non-existent", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)

		var result map[string]interface{}
		respBody, _ := io.ReadAll(resp.Body)
		json.Unmarshal(respBody, &result)

		assert.Equal(t, "Worktree not found", result["error"])

		mockGitService.AssertExpectations(t)
	})

	t.Run("invalid request body", func(t *testing.T) {
		app.Patch("/v1/git/worktrees/:id", handler.UpdateWorktree)

		req := httptest.NewRequest("PATCH", "/v1/git/worktrees/wt-123", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)

		var result map[string]interface{}
		respBody, _ := io.ReadAll(resp.Body)
		json.Unmarshal(respBody, &result)

		assert.Contains(t, result["error"], "Invalid request body")
	})
}

func TestListGitHubRepositories(t *testing.T) {
	handler, mockGitService, _, _, app := setupGitHandlerTest()

	t.Run("successful list", func(t *testing.T) {
		repos := []map[string]interface{}{
			{
				"id":        123,
				"name":      "repo1",
				"full_name": "user/repo1",
				"private":   false,
			},
			{
				"id":        456,
				"name":      "repo2",
				"full_name": "user/repo2",
				"private":   true,
			},
		}

		mockGitService.On("ListGitHubRepositories").Return(repos, nil)

		app.Get("/v1/git/github/repos", handler.ListGitHubRepositories)

		req := httptest.NewRequest("GET", "/v1/git/github/repos", nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result []map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Len(t, result, 2)
		assert.Equal(t, float64(123), result[0]["id"])
		assert.Equal(t, "repo1", result[0]["name"])

		mockGitService.AssertExpectations(t)
	})

	t.Run("error from service", func(t *testing.T) {
		mockGitService := new(MockGitService)
		handler.gitService = (*services.GitService)(mockGitService)

		mockGitService.On("ListGitHubRepositories").Return(([]map[string]interface{})(nil), fmt.Errorf("API error"))

		app.Get("/v1/git/github/repos", handler.ListGitHubRepositories)

		req := httptest.NewRequest("GET", "/v1/git/github/repos", nil)
		resp, err := app.Test(req)

		assert.NoError(t, err)
		assert.Equal(t, 500, resp.StatusCode)

		var result map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "API error", result["error"])

		mockGitService.AssertExpectations(t)
	})
}
