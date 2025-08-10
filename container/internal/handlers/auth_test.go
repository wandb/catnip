//nolint:errcheck // Test file - ignoring error checks for json.Unmarshal
package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/config"
	"gopkg.in/yaml.v2"
)

func TestNewAuthHandler(t *testing.T) {
	handler := NewAuthHandler()
	assert.NotNil(t, handler)
	assert.Nil(t, handler.activeAuth)
}

func TestAuthHandler_GetAuthStatus(t *testing.T) {
	app := fiber.New()

	t.Run("no active auth - not authenticated", func(t *testing.T) {
		// Create handler with mock that returns no user (not authenticated)
		mockChecker := NewMockGitHubAuthChecker(nil, fmt.Errorf("not authenticated"))
		handler := NewAuthHandlerWithChecker(mockChecker)
		app.Get("/v1/auth/github/status", handler.GetAuthStatus)

		req := httptest.NewRequest("GET", "/v1/auth/github/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result AuthStatusResponse
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "none", result.Status)
		assert.Empty(t, result.Error)
		assert.Nil(t, result.User)
	})

	t.Run("no active auth - authenticated", func(t *testing.T) {
		// Create handler with mock that returns an authenticated user
		testUser := &AuthUser{
			Username: "testuser",
			Scopes:   []string{"repo", "read:org"},
		}
		mockChecker := NewMockGitHubAuthChecker(testUser, nil)
		handler := NewAuthHandlerWithChecker(mockChecker)
		app.Get("/v1/auth/github/status", handler.GetAuthStatus)

		req := httptest.NewRequest("GET", "/v1/auth/github/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result AuthStatusResponse
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "authenticated", result.Status)
		assert.Empty(t, result.Error)
		assert.NotNil(t, result.User)
		assert.Equal(t, "testuser", result.User.Username)
		assert.Contains(t, result.User.Scopes, "repo")
		assert.Contains(t, result.User.Scopes, "read:org")
	})

	t.Run("active auth process", func(t *testing.T) {
		// Set up an active auth process
		handler.activeAuth = &AuthProcess{
			Status: "waiting",
			Code:   "1234-5678",
			URL:    "https://github.com/login/device",
		}

		req := httptest.NewRequest("GET", "/v1/auth/github/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result AuthStatusResponse
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "waiting", result.Status)
		assert.Empty(t, result.Error)

		// Clean up
		handler.activeAuth = nil
	})

	t.Run("active auth process with error", func(t *testing.T) {
		// Set up an active auth process with error
		handler.activeAuth = &AuthProcess{
			Status: "error",
			Error:  "authentication failed",
		}

		req := httptest.NewRequest("GET", "/v1/auth/github/status", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result AuthStatusResponse
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "error", result.Status)
		assert.Equal(t, "authentication failed", result.Error)

		// Clean up
		handler.activeAuth = nil
	})
}

func TestAuthHandler_ResetAuthState(t *testing.T) {
	handler := NewAuthHandler()
	app := fiber.New()
	app.Post("/v1/auth/github/reset", handler.ResetAuthState)

	t.Run("reset with no active auth", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/auth/github/reset", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "reset", result["status"])
		assert.Nil(t, handler.activeAuth)
	})

	t.Run("reset with active auth", func(t *testing.T) {
		// Set up an active auth process (without actual command)
		handler.activeAuth = &AuthProcess{
			Status: "waiting",
			Code:   "1234-5678",
		}

		req := httptest.NewRequest("POST", "/v1/auth/github/reset", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &result)

		assert.Equal(t, "reset", result["status"])
		assert.Nil(t, handler.activeAuth)
	})
}

func TestAuthHandler_readGitHubHosts(t *testing.T) {
	handler := NewAuthHandler()

	t.Run("no hosts file", func(t *testing.T) {
		// Set up a temporary home directory without hosts file
		tmpDir := t.TempDir()
		originalHomeDir := config.Runtime.HomeDir
		config.Runtime.HomeDir = tmpDir
		defer func() { config.Runtime.HomeDir = originalHomeDir }()

		user, err := handler.readGitHubHosts()
		assert.Error(t, err)
		assert.Nil(t, user)
	})

	t.Run("valid hosts file", func(t *testing.T) {
		// Set up a temporary home directory with hosts file
		tmpDir := t.TempDir()
		originalHomeDir := config.Runtime.HomeDir
		config.Runtime.HomeDir = tmpDir
		defer func() { config.Runtime.HomeDir = originalHomeDir }()

		// Create .config/gh directory
		ghDir := filepath.Join(tmpDir, ".config", "gh")
		require.NoError(t, os.MkdirAll(ghDir, 0755))

		// Create hosts.yml file
		hostsContent := GitHubHosts{
			GitHubCom: GitHubHost{
				User: "testuser",
				Users: map[string]GitHubUser{
					"testuser": {
						OAuthToken: "gho_test123",
					},
				},
				OAuthToken: "gho_test123",
			},
		}

		hostsData, err := yaml.Marshal(hostsContent)
		require.NoError(t, err)

		hostsPath := filepath.Join(ghDir, "hosts.yml")
		require.NoError(t, os.WriteFile(hostsPath, hostsData, 0644))

		user, err := handler.readGitHubHosts()
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "testuser", user.Username)
		// Note: Scopes will be empty since we can't mock the gh command
		assert.NotNil(t, user.Scopes)
	})

	t.Run("invalid yaml in hosts file", func(t *testing.T) {
		// Set up a temporary home directory with invalid hosts file
		tmpDir := t.TempDir()
		originalHomeDir := config.Runtime.HomeDir
		config.Runtime.HomeDir = tmpDir
		defer func() { config.Runtime.HomeDir = originalHomeDir }()

		// Create .config/gh directory
		ghDir := filepath.Join(tmpDir, ".config", "gh")
		require.NoError(t, os.MkdirAll(ghDir, 0755))

		// Create invalid hosts.yml file
		hostsPath := filepath.Join(ghDir, "hosts.yml")
		require.NoError(t, os.WriteFile(hostsPath, []byte("invalid: yaml: content:"), 0644))

		user, err := handler.readGitHubHosts()
		assert.Error(t, err)
		assert.Nil(t, user)
	})

	t.Run("hosts file with no user", func(t *testing.T) {
		// Set up a temporary home directory with hosts file but no user
		tmpDir := t.TempDir()
		originalHomeDir := config.Runtime.HomeDir
		config.Runtime.HomeDir = tmpDir
		defer func() { config.Runtime.HomeDir = originalHomeDir }()

		// Create .config/gh directory
		ghDir := filepath.Join(tmpDir, ".config", "gh")
		require.NoError(t, os.MkdirAll(ghDir, 0755))

		// Create hosts.yml file without user
		hostsContent := GitHubHosts{
			GitHubCom: GitHubHost{
				Users: map[string]GitHubUser{},
			},
		}

		hostsData, err := yaml.Marshal(hostsContent)
		require.NoError(t, err)

		hostsPath := filepath.Join(ghDir, "hosts.yml")
		require.NoError(t, os.WriteFile(hostsPath, hostsData, 0644))

		user, err := handler.readGitHubHosts()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no authenticated user found")
		assert.Nil(t, user)
	})
}

func TestAuthProcess_struct(t *testing.T) {
	now := time.Now()
	process := &AuthProcess{
		Code:      "1234-5678",
		URL:       "https://github.com/login/device",
		Status:    "waiting",
		Error:     "",
		StartedAt: now,
	}

	assert.Equal(t, "1234-5678", process.Code)
	assert.Equal(t, "https://github.com/login/device", process.URL)
	assert.Equal(t, "waiting", process.Status)
	assert.Empty(t, process.Error)
	assert.Equal(t, now, process.StartedAt)
}

func TestAuthStartResponse_struct(t *testing.T) {
	response := AuthStartResponse{
		Code:   "1234-5678",
		URL:    "https://github.com/login/device",
		Status: "waiting",
	}

	assert.Equal(t, "1234-5678", response.Code)
	assert.Equal(t, "https://github.com/login/device", response.URL)
	assert.Equal(t, "waiting", response.Status)
}

func TestAuthStatusResponse_struct(t *testing.T) {
	user := &AuthUser{
		Username: "testuser",
		Scopes:   []string{"repo", "read:org"},
	}

	response := AuthStatusResponse{
		Status: "authenticated",
		Error:  "",
		User:   user,
	}

	assert.Equal(t, "authenticated", response.Status)
	assert.Empty(t, response.Error)
	assert.NotNil(t, response.User)
	assert.Equal(t, "testuser", response.User.Username)
	assert.Contains(t, response.User.Scopes, "repo")
	assert.Contains(t, response.User.Scopes, "read:org")
}

func TestAuthUser_struct(t *testing.T) {
	user := AuthUser{
		Username: "testuser",
		Scopes:   []string{"repo", "read:org", "workflow"},
	}

	assert.Equal(t, "testuser", user.Username)
	assert.Len(t, user.Scopes, 3)
	assert.Contains(t, user.Scopes, "repo")
	assert.Contains(t, user.Scopes, "read:org")
	assert.Contains(t, user.Scopes, "workflow")
}

// MockGitHubAuthChecker is a mock implementation for testing
type MockGitHubAuthChecker struct {
	user *AuthUser
	err  error
}

// CheckGitHubAuthStatus implements the interface for mocking
func (m *MockGitHubAuthChecker) CheckGitHubAuthStatus() (*AuthUser, error) {
	return m.user, m.err
}

// NewMockGitHubAuthChecker creates a new mock with specified behavior
func NewMockGitHubAuthChecker(user *AuthUser, err error) *MockGitHubAuthChecker {
	return &MockGitHubAuthChecker{
		user: user,
		err:  err,
	}
}
