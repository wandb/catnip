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
)

// MockAuthService implements the AuthService interface for testing
type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) GetAuthStatus(sessionID string) (*models.AuthStatus, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*models.AuthStatus), args.Error(1)
}

func (m *MockAuthService) ResetAuthState(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

func (m *MockAuthService) ParseGitHubHostsFile(sessionID string) ([]*models.GitHubHost, error) {
	args := m.Called(sessionID)
	return args.Get(0).([]*models.GitHubHost), args.Error(1)
}

func TestAuthHandler_GetAuthStatus(t *testing.T) {
	mockAuthService := new(MockAuthService)
	mockSessionService := new(MockSessionService)

	handler := &AuthHandler{
		AuthService:    mockAuthService,
		SessionService: mockSessionService,
	}

	app := fiber.New()
	app.Post("/auth-status", handler.GetAuthStatus)

	requestBody := `{"sessionId": "session-123"}`
	expectedAuthStatus := &models.AuthStatus{
		IsAuthenticated: true,
		Username:        "testuser",
		Provider:        "github",
		TokenExpiry:     "2024-12-31T23:59:59Z",
	}

	mockAuthService.On("GetAuthStatus", "session-123").Return(expectedAuthStatus, nil)

	req := httptest.NewRequest("POST", "/auth-status", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response models.AuthStatus
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)
	assert.True(t, response.IsAuthenticated)
	assert.Equal(t, "testuser", response.Username)
	assert.Equal(t, "github", response.Provider)

	mockAuthService.AssertExpectations(t)
}

func TestAuthHandler_GetAuthStatus_Unauthenticated(t *testing.T) {
	mockAuthService := new(MockAuthService)
	mockSessionService := new(MockSessionService)

	handler := &AuthHandler{
		AuthService:    mockAuthService,
		SessionService: mockSessionService,
	}

	app := fiber.New()
	app.Post("/auth-status", handler.GetAuthStatus)

	requestBody := `{"sessionId": "session-123"}`
	expectedAuthStatus := &models.AuthStatus{
		IsAuthenticated: false,
		Username:        "",
		Provider:        "",
		TokenExpiry:     "",
	}

	mockAuthService.On("GetAuthStatus", "session-123").Return(expectedAuthStatus, nil)

	req := httptest.NewRequest("POST", "/auth-status", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response models.AuthStatus
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)
	assert.False(t, response.IsAuthenticated)
	assert.Empty(t, response.Username)

	mockAuthService.AssertExpectations(t)
}

func TestAuthHandler_ResetAuthState(t *testing.T) {
	mockAuthService := new(MockAuthService)
	mockSessionService := new(MockSessionService)

	handler := &AuthHandler{
		AuthService:    mockAuthService,
		SessionService: mockSessionService,
	}

	app := fiber.New()
	app.Post("/reset-auth", handler.ResetAuthState)

	requestBody := `{"sessionId": "session-123"}`

	mockAuthService.On("ResetAuthState", "session-123").Return(nil)

	req := httptest.NewRequest("POST", "/reset-auth", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	mockAuthService.AssertExpectations(t)
}

func TestAuthHandler_ParseGitHubHostsFile(t *testing.T) {
	mockAuthService := new(MockAuthService)
	mockSessionService := new(MockSessionService)

	handler := &AuthHandler{
		AuthService:    mockAuthService,
		SessionService: mockSessionService,
	}

	app := fiber.New()
	app.Post("/github-hosts", handler.ParseGitHubHostsFile)

	requestBody := `{"sessionId": "session-123"}`
	expectedHosts := []*models.GitHubHost{
		{
			Hostname: "github.com",
			User:     "testuser",
			Protocol: "https",
		},
		{
			Hostname: "github.enterprise.com",
			User:     "enterpriseuser",
			Protocol: "ssh",
		},
	}

	mockAuthService.On("ParseGitHubHostsFile", "session-123").Return(expectedHosts, nil)

	req := httptest.NewRequest("POST", "/github-hosts", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response []*models.GitHubHost
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, "github.com", response[0].Hostname)
	assert.Equal(t, "testuser", response[0].User)
	assert.Equal(t, "https", response[0].Protocol)
	assert.Equal(t, "github.enterprise.com", response[1].Hostname)

	mockAuthService.AssertExpectations(t)
}
