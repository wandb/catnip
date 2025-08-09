//nolint:errcheck // Test file - ignoring error checks for json.Unmarshal
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

// Create interfaces for the methods we need to mock
type GitServiceInterface interface {
	CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error)
	GetStatus() *models.GitStatus
	ListWorktrees() []*models.Worktree
	IsWorktreeStatusCached(id string) bool
	GetWorktree(id string) (*models.Worktree, bool)
	UpdateWorktreeFields(id string, updates map[string]interface{}) error
	ListGitHubRepositories() ([]map[string]interface{}, error)
}

type SessionServiceInterface interface {
	GetActiveSession(path string) (*services.ActiveSessionInfo, bool)
	GetClaudeActivityState(path string) models.ClaudeActivityState
}

type ClaudeMonitorInterface interface {
	GetTodos(path string) ([]models.Todo, error)
}

// Mock implementations
type mockGitService struct {
	mock.Mock
}

func (m *mockGitService) CheckoutRepository(org, repo, branch string) (*models.Repository, *models.Worktree, error) {
	args := m.Called(org, repo, branch)
	if args.Error(2) != nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*models.Repository), args.Get(1).(*models.Worktree), nil
}

func (m *mockGitService) GetStatus() *models.GitStatus {
	args := m.Called()
	return args.Get(0).(*models.GitStatus)
}

func (m *mockGitService) ListWorktrees() []*models.Worktree {
	args := m.Called()
	return args.Get(0).([]*models.Worktree)
}

func (m *mockGitService) IsWorktreeStatusCached(id string) bool {
	args := m.Called(id)
	return args.Bool(0)
}

func (m *mockGitService) GetWorktree(id string) (*models.Worktree, bool) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, false
	}
	return args.Get(0).(*models.Worktree), args.Bool(1)
}

func (m *mockGitService) UpdateWorktreeFields(id string, updates map[string]interface{}) error {
	args := m.Called(id, updates)
	return args.Error(0)
}

func (m *mockGitService) ListGitHubRepositories() ([]map[string]interface{}, error) {
	args := m.Called()
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]map[string]interface{}), nil
}

type mockSessionService struct {
	mock.Mock
}

func (m *mockSessionService) GetActiveSession(path string) (*services.ActiveSessionInfo, bool) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, false
	}
	return args.Get(0).(*services.ActiveSessionInfo), args.Bool(1)
}

func (m *mockSessionService) GetClaudeActivityState(path string) models.ClaudeActivityState {
	args := m.Called(path)
	return args.Get(0).(models.ClaudeActivityState)
}

type mockClaudeMonitorService struct {
	mock.Mock
}

func (m *mockClaudeMonitorService) GetTodos(path string) ([]models.Todo, error) {
	args := m.Called(path)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Todo), nil
}

// TestGitHandler wraps GitHandler for testing with interfaces
type TestGitHandler struct {
	gitService     GitServiceInterface
	sessionService SessionServiceInterface
	claudeMonitor  ClaudeMonitorInterface
}

func (h *TestGitHandler) CheckoutRepository(c *fiber.Ctx) error {
	org := c.Params("org")
	repo := c.Params("repo")
	branch := c.Query("branch", "")

	repository, worktree, err := h.gitService.CheckoutRepository(org, repo, branch)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"repository": repository,
		"worktree":   worktree,
		"message":    "Repository checked out successfully",
	})
}

func (h *TestGitHandler) GetStatus(c *fiber.Ctx) error {
	status := h.gitService.GetStatus()
	return c.JSON(status)
}

func (h *TestGitHandler) ListWorktrees(c *fiber.Ctx) error {
	worktrees := h.gitService.ListWorktrees()
	enhancedWorktrees := make([]*EnhancedWorktree, 0, len(worktrees))

	for _, worktree := range worktrees {
		// Enhance worktrees with session information
		if sessionInfo, exists := h.sessionService.GetActiveSession(worktree.Path); exists {
			// Convert services.TitleEntry to models.TitleEntry
			if sessionInfo.Title != nil {
				worktree.SessionTitle = &models.TitleEntry{
					Title:      sessionInfo.Title.Title,
					Timestamp:  sessionInfo.Title.Timestamp,
					CommitHash: sessionInfo.Title.CommitHash,
				}
			}

			// Convert []services.TitleEntry to []models.TitleEntry
			if len(sessionInfo.TitleHistory) > 0 {
				history := make([]models.TitleEntry, len(sessionInfo.TitleHistory))
				for i, entry := range sessionInfo.TitleHistory {
					history[i] = models.TitleEntry{
						Title:      entry.Title,
						Timestamp:  entry.Timestamp,
						CommitHash: entry.CommitHash,
					}
				}
				worktree.SessionTitleHistory = history
			}
		}

		// Determine Claude activity state for this worktree
		claudeActivityState := h.sessionService.GetClaudeActivityState(worktree.Path)
		worktree.ClaudeActivityState = claudeActivityState

		// Set backward compatibility flag
		worktree.HasActiveClaudeSession = (claudeActivityState == models.ClaudeActive || claudeActivityState == models.ClaudeRunning)

		// Get todos for this worktree
		if todos, err := h.claudeMonitor.GetTodos(worktree.Path); err == nil {
			worktree.Todos = todos
		}
		// If there's an error getting todos, we'll leave Todos as nil (which is fine)

		// Create enhanced worktree with cache status
		enhanced := &EnhancedWorktree{
			Worktree: worktree,
			CacheStatus: &WorktreeCacheStatus{
				IsCached:  h.gitService.IsWorktreeStatusCached(worktree.ID),
				IsLoading: !h.gitService.IsWorktreeStatusCached(worktree.ID), // Loading if not cached
			},
		}

		enhancedWorktrees = append(enhancedWorktrees, enhanced)
	}

	return c.JSON(enhancedWorktrees)
}

func (h *TestGitHandler) UpdateWorktree(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	if worktreeID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Worktree ID is required",
		})
	}

	// Parse the request body to get the fields to update
	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid request body: %v", err),
		})
	}

	// Update the worktree using the state manager
	if err := h.gitService.UpdateWorktreeFields(worktreeID, updates); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to update worktree: %v", err),
		})
	}

	// Get the updated worktree
	worktree, exists := h.gitService.GetWorktree(worktreeID)
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Worktree not found",
		})
	}

	return c.JSON(worktree)
}

func (h *TestGitHandler) ListGitHubRepositories(c *fiber.Ctx) error {
	repos, err := h.gitService.ListGitHubRepositories()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(repos)
}

// Setup test helper
func setupGitHandlerTest() (*TestGitHandler, *mockGitService, *mockSessionService, *mockClaudeMonitorService, *fiber.App) {
	mockGit := new(mockGitService)
	mockSession := new(mockSessionService)
	mockClaude := new(mockClaudeMonitorService)

	handler := &TestGitHandler{
		gitService:     mockGit,
		sessionService: mockSession,
		claudeMonitor:  mockClaude,
	}

	app := fiber.New()

	return handler, mockGit, mockSession, mockClaude, app
}

func TestCheckoutRepository(t *testing.T) {
	handler, mockGitService, _, _, app := setupGitHandlerTest()

	t.Run("successful checkout", func(t *testing.T) {
		expectedRepo := &models.Repository{
			ID:            "test-org/test-repo",
			URL:           "https://github.com/test-org/test-repo",
			Path:          "/workspace/repos/test-org_test-repo.git",
			DefaultBranch: "main",
			Available:     true,
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
		handler, mockGitService, _, _, app := setupGitHandlerTest()

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
		Repositories: map[string]*models.Repository{
			"test-org/test-repo": {
				ID:            "test-org/test-repo",
				URL:           "https://github.com/test-org/test-repo",
				Path:          "/workspace/repos/test-org_test-repo.git",
				DefaultBranch: "main",
				Available:     true,
			},
		},
		WorktreeCount: 1,
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

	assert.Equal(t, expectedStatus.WorktreeCount, result.WorktreeCount)
	assert.NotNil(t, result.Repositories)
	assert.Contains(t, result.Repositories, "test-org/test-repo")

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
	mockSessionService.On("GetActiveSession", "/workspace/repo/main").Return((*services.ActiveSessionInfo)(nil), false)
	mockSessionService.On("GetActiveSession", "/workspace/repo/feature").Return((*services.ActiveSessionInfo)(nil), false)
	mockSessionService.On("GetClaudeActivityState", "/workspace/repo/main").Return(models.ClaudeInactive)
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
			RepoID: "test-org/test-repo",
			Name:   "test-worktree",
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
		assert.Equal(t, "/workspace/test-repo", result.Path)

		mockGitService.AssertExpectations(t)
	})

	t.Run("worktree not found", func(t *testing.T) {
		handler, mockGitService, _, _, app := setupGitHandlerTest()

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
		handler, mockGitService, _, _, app := setupGitHandlerTest()

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
