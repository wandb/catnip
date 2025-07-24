// Package common provides shared testing utilities
package common //nolint:revive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSuite holds the test environment
type TestSuite struct {
	BaseURL    string
	TestDir    string
	HTTPClient *http.Client
	cleanup    func()
}

// SetupTestSuite initializes the test environment to connect to external test server
func SetupTestSuite(t *testing.T) *TestSuite {
	// Create temporary test directory
	testDir, err := os.MkdirTemp("", "catnip-integration-test-*")
	require.NoError(t, err)

	// Get test server URL from environment or use default
	baseURL := os.Getenv("CATNIP_TEST_SERVER_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8181"
	}

	// Create HTTP client with reasonable timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Set up test environment variables
	_ = os.Setenv("CATNIP_TEST_MODE", "1")

	testDataDir := os.Getenv("CATNIP_TEST_DATA_DIR")
	if testDataDir == "" {
		testDataDir = filepath.Join(testDir, "test_data")
		_ = os.Setenv("CATNIP_TEST_DATA_DIR", testDataDir)
	}

	// Create test data directories
	require.NoError(t, os.MkdirAll(filepath.Join(testDataDir, "claude_responses"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testDataDir, "gh_data"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testDataDir, "git_data"), 0755))

	// Verify that the test server is running
	healthURL := baseURL + "/health"
	resp, err := httpClient.Get(healthURL)
	if err != nil {
		require.FailNow(t, fmt.Sprintf("Test server is not running at %s. Start it with './run_integration_tests.sh start'", baseURL))
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		require.FailNow(t, fmt.Sprintf("Test server at %s returned status %d. Expected 200", baseURL, resp.StatusCode))
	}

	return &TestSuite{
		BaseURL:    baseURL,
		TestDir:    testDir,
		HTTPClient: httpClient,
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

// MakeRequest is a helper function to make HTTP requests to the test server
func (ts *TestSuite) MakeRequest(method, path string, body interface{}) (*http.Response, []byte, error) {
	var bodyReader io.Reader

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	// Construct full URL
	url := ts.BaseURL + path

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := ts.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

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
