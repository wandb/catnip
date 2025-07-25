// Package common provides shared testing utilities
package common //nolint:revive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// CreateTestRepository creates a real git repository inside the container that can be cloned
func (ts *TestSuite) CreateTestRepository(t *testing.T, name string) string {
	// Create the repository inside the container using docker exec
	containerName := "catnip-test"
	repoPath := filepath.Join("/tmp", "test-repos", name)

	// Clean up any existing repository
	cleanupCmd := exec.Command("docker", "exec", containerName, "rm", "-rf", repoPath)
	_ = cleanupCmd.Run() // Ignore errors if directory doesn't exist

	// Create directory structure
	mkdirCmd := exec.Command("docker", "exec", containerName, "mkdir", "-p", repoPath)
	require.NoError(t, mkdirCmd.Run(), "Failed to create repository directory in container")

	// Initialize git repository inside container with main branch
	initCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "init", "--initial-branch=main", repoPath)
	require.NoError(t, initCmd.Run(), "Failed to initialize git repository")

	// Configure git user for commits
	configNameCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", repoPath, "config", "user.name", "Test User")
	require.NoError(t, configNameCmd.Run(), "Failed to set git user name")

	configEmailCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", repoPath, "config", "user.email", "test@example.com")
	require.NoError(t, configEmailCmd.Run(), "Failed to set git user email")

	// Fix ownership issues by adding safe directory
	configSafeCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "config", "--global", "--add", "safe.directory", "*")
	require.NoError(t, configSafeCmd.Run(), "Failed to set git safe directory")

	// Also add the specific repository path
	configSafeSpecificCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "config", "--global", "--add", "safe.directory", repoPath)
	require.NoError(t, configSafeSpecificCmd.Run(), "Failed to set git safe directory for specific repo")

	// Create initial file
	content := fmt.Sprintf("# %s\n\nThis is a test repository for integration testing.\n", name)
	createFileCmd := exec.Command("docker", "exec", containerName, "sh", "-c",
		fmt.Sprintf("echo '%s' > %s/README.md", content, repoPath))
	require.NoError(t, createFileCmd.Run(), "Failed to create README.md")

	// Add and commit the initial file
	addCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", repoPath, "add", "README.md")
	require.NoError(t, addCmd.Run(), "Failed to add README.md")

	commitCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", repoPath, "commit", "-m", "Initial commit")
	require.NoError(t, commitCmd.Run(), "Failed to create initial commit")

	// Fix ownership - change ownership to catnip user to match the git service
	chownCmd := exec.Command("docker", "exec", containerName, "chown", "-R", "catnip:catnip", repoPath)
	require.NoError(t, chownCmd.Run(), "Failed to change ownership of test repository")

	// Check what branch was created
	branchCmd := exec.Command("docker", "exec", containerName, "/usr/bin/git", "-C", repoPath, "branch", "--show-current")
	branchOutput, err := branchCmd.Output()
	require.NoError(t, err, "Failed to get current branch")
	currentBranch := strings.TrimSpace(string(branchOutput))

	t.Logf("Created test repository at: %s (branch: %s)", repoPath, currentBranch)
	return repoPath
}
