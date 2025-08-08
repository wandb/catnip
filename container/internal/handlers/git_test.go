package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// MockGitService implements the GitService interface for testing
type MockGitService struct {
	mock.Mock
}

func (m *MockGitService) CheckoutRepository(sessionID, repoURL, branch string) error {
	args := m.Called(sessionID, repoURL, branch)
	return args.Error(0)
}

func (m *MockGitService) GetStatus(sessionID string) (*models.GitStatus, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*models.GitStatus), args.Error(1)
}

func (m *MockGitService) ListWorktrees(sessionID string) ([]*models.Worktree, error) {
	args := m.Called(sessionID)
	return args.Get(0).([]*models.Worktree), args.Error(1)
}

func (m *MockGitService) UpdateWorktree(sessionID, worktreeID, branchName string) error {
	args := m.Called(sessionID, worktreeID, branchName)
	return args.Error(0)
}

func (m *MockGitService) ListGitHubRepositories(sessionID string) ([]*models.Repository, error) {
	args := m.Called(sessionID)
	return args.Get(0).([]*models.Repository), args.Error(1)
}

// MockSessionService implements the SessionService interface for testing
type MockSessionService struct {
	mock.Mock
}

func (m *MockSessionService) GetSession(sessionID string) (*models.Session, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*models.Session), args.Error(1)
}

// MockClaudeMonitorService implements the ClaudeMonitorService interface for testing
type MockClaudeMonitorService struct {
	mock.Mock
}

func (m *MockClaudeMonitorService) UpdateClaudeActivity(sessionID string, activity models.ClaudeActivityState, details string) {
	m.Called(sessionID, activity, details)
}

func TestGitHandler_CheckoutRepository(t *testing.T) {
	mockGitService := new(MockGitService)
	mockSessionService := new(MockSessionService)
	mockClaudeService := new(MockClaudeMonitorService)

	handler := &GitHandler{
		GitService:           mockGitService,
		SessionService:       mockSessionService,
		ClaudeMonitorService: mockClaudeService,
	}

	app := fiber.New()
	app.Post("/checkout", handler.CheckoutRepository)

	requestBody := `{"sessionId": "session-123", "repoUrl": "https://github.com/test/repo", "branch": "main"}`

	mockGitService.On("CheckoutRepository", "session-123", "https://github.com/test/repo", "main").Return(nil)
	mockClaudeService.On("UpdateClaudeActivity", "session-123", models.ClaudeInactive, "Repository checked out: https://github.com/test/repo").Return()

	req := httptest.NewRequest("POST", "/checkout", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	mockGitService.AssertExpectations(t)
	mockClaudeService.AssertExpectations(t)
}

func TestGitHandler_GetStatus(t *testing.T) {
	mockGitService := new(MockGitService)
	mockSessionService := new(MockSessionService)
	mockClaudeService := new(MockClaudeMonitorService)

	handler := &GitHandler{
		GitService:           mockGitService,
		SessionService:       mockSessionService,
		ClaudeMonitorService: mockClaudeService,
	}

	app := fiber.New()
	app.Post("/status", handler.GetStatus)

	requestBody := `{"sessionId": "session-123"}`
	expectedStatus := &models.GitStatus{
		Branch:     "main",
		IsClean:    true,
		Ahead:      0,
		Behind:     0,
		Modified:   []string{},
		Staged:     []string{},
		Untracked:  []string{},
		HasRemote:  true,
		RemoteName: "origin",
		RemoteURL:  "https://github.com/test/repo",
	}

	mockGitService.On("GetStatus", "session-123").Return(expectedStatus, nil)

	req := httptest.NewRequest("POST", "/status", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response models.GitStatus
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "main", response.Branch)
	assert.True(t, response.IsClean)

	mockGitService.AssertExpectations(t)
}

func TestGitHandler_ListWorktrees(t *testing.T) {
	mockGitService := new(MockGitService)
	mockSessionService := new(MockSessionService)
	mockClaudeService := new(MockClaudeMonitorService)

	handler := &GitHandler{
		GitService:           mockGitService,
		SessionService:       mockSessionService,
		ClaudeMonitorService: mockClaudeService,
	}

	app := fiber.New()
	app.Post("/worktrees", handler.ListWorktrees)

	requestBody := `{"sessionId": "session-123"}`
	expectedWorktrees := []*models.Worktree{
		{
			ID:     "worktree-1",
			Path:   "/workspace/main",
			Branch: "main",
			IsMain: true,
			Head:   "abc123",
		},
		{
			ID:     "worktree-2",
			Path:   "/workspace/feature",
			Branch: "feature-branch",
			IsMain: false,
			Head:   "def456",
		},
	}

	mockGitService.On("ListWorktrees", "session-123").Return(expectedWorktrees, nil)

	req := httptest.NewRequest("POST", "/worktrees", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response []*models.Worktree
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, "main", response[0].Branch)
	assert.Equal(t, "feature-branch", response[1].Branch)

	mockGitService.AssertExpectations(t)
}

func TestGitHandler_UpdateWorktree(t *testing.T) {
	mockGitService := new(MockGitService)
	mockSessionService := new(MockSessionService)
	mockClaudeService := new(MockClaudeMonitorService)

	handler := &GitHandler{
		GitService:           mockGitService,
		SessionService:       mockSessionService,
		ClaudeMonitorService: mockClaudeService,
	}

	app := fiber.New()
	app.Post("/update-worktree", handler.UpdateWorktree)

	requestBody := `{"sessionId": "session-123", "worktreeId": "worktree-1", "branchName": "new-branch"}`

	mockGitService.On("UpdateWorktree", "session-123", "worktree-1", "new-branch").Return(nil)
	mockClaudeService.On("UpdateClaudeActivity", "session-123", models.ClaudeInactive, "Worktree updated: worktree-1 -> new-branch").Return()

	req := httptest.NewRequest("POST", "/update-worktree", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	mockGitService.AssertExpectations(t)
	mockClaudeService.AssertExpectations(t)
}

func TestGitHandler_ListGitHubRepositories(t *testing.T) {
	mockGitService := new(MockGitService)
	mockSessionService := new(MockSessionService)
	mockClaudeService := new(MockClaudeMonitorService)

	handler := &GitHandler{
		GitService:           mockGitService,
		SessionService:       mockSessionService,
		ClaudeMonitorService: mockClaudeService,
	}

	app := fiber.New()
	app.Post("/github-repos", handler.ListGitHubRepositories)

	requestBody := `{"sessionId": "session-123"}`
	expectedRepos := []*models.Repository{
		{
			ID:            "repo-1",
			URL:           "https://github.com/test/repo1",
			Path:          "/workspace/repo1",
			DefaultBranch: "main",
			Available:     true,
		},
		{
			ID:            "repo-2",
			URL:           "https://github.com/test/repo2",
			Path:          "/workspace/repo2",
			DefaultBranch: "master",
			Available:     false,
		},
	}

	mockGitService.On("ListGitHubRepositories", "session-123").Return(expectedRepos, nil)

	req := httptest.NewRequest("POST", "/github-repos", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response []*models.Repository
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, "https://github.com/test/repo1", response[0].URL)
	assert.True(t, response[0].Available)
	assert.False(t, response[1].Available)

	mockGitService.AssertExpectations(t)
}
