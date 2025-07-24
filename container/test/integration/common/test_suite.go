// Package common provides shared testing utilities
package common //nolint:revive

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/handlers"
	"github.com/vanpelt/catnip/internal/services"
)

// TestSuite holds the test environment
type TestSuite struct {
	App        *fiber.App
	TestDir    string
	GitService *services.GitService
	cleanup    func()
}

// SetupTestSuite initializes the test environment with mocked commands
func SetupTestSuite(t *testing.T) *TestSuite {
	// Create temporary test directory
	testDir, err := os.MkdirTemp("", "catnip-integration-test-*")
	require.NoError(t, err)

	// Set up test environment variables
	_ = os.Setenv("CATNIP_TEST_MODE", "1")
	_ = os.Setenv("CATNIP_TEST_DATA_DIR", filepath.Join(testDir, "test_data"))

	// Create test data directories
	testDataDir := filepath.Join(testDir, "test_data")
	require.NoError(t, os.MkdirAll(filepath.Join(testDataDir, "claude_responses"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testDataDir, "gh_data"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testDataDir, "git_data"), 0755))

	// Initialize services with test configuration
	gitService := services.NewGitService()
	gitHTTPService := services.NewGitHTTPService(gitService)
	sessionService := services.NewSessionService()
	claudeService := services.NewClaudeService()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return ctx.Status(500).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Setup handlers
	gitHandler := handlers.NewGitHandler(gitService, gitHTTPService, sessionService)
	claudeHandler := handlers.NewClaudeHandler(claudeService)

	// Setup routes using the new RegisterRoutes methods
	v1 := app.Group("/v1")
	gitHandler.RegisterRoutes(v1)
	claudeHandler.RegisterRoutes(v1)

	return &TestSuite{
		App:        app,
		TestDir:    testDir,
		GitService: gitService,
		cleanup: func() {
			_ = os.RemoveAll(testDir)
			_ = os.Unsetenv("CATNIP_TEST_MODE")
			_ = os.Unsetenv("CATNIP_TEST_DATA_DIR")
		},
	}
}

// TearDown cleans up the test environment
func (ts *TestSuite) TearDown() {
	if ts.cleanup != nil {
		ts.cleanup()
	}
}

// MakeRequest is a helper function to make HTTP requests
func (ts *TestSuite) MakeRequest(method, path string, body interface{}) (*http.Response, []byte, error) {
	var bodyReader io.Reader

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := ts.App.Test(req, -1) // -1 means no timeout
	if err != nil {
		return nil, nil, err
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}

	return resp, respBody, nil
}

// CreateTestRepository creates a test git repository
func (ts *TestSuite) CreateTestRepository(t *testing.T, name string) string {
	repoPath := filepath.Join(ts.TestDir, "repos", name)
	require.NoError(t, os.MkdirAll(repoPath, 0755))

	// Initialize git repository
	require.NoError(t, os.Setenv("PATH", "/opt/catnip/test/bin:"+os.Getenv("PATH")))

	// Create initial file and commit
	readmePath := filepath.Join(repoPath, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# Test Repository\n"), 0644))

	return repoPath
}
