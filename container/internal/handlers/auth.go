package handlers

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
	"gopkg.in/yaml.v2"
)

// GitHubAuthChecker defines the interface for checking GitHub authentication status
type GitHubAuthChecker interface {
	CheckGitHubAuthStatus() (*AuthUser, error)
}

// DefaultGitHubAuthChecker implements GitHubAuthChecker using actual GitHub CLI commands
type DefaultGitHubAuthChecker struct{}

// CheckGitHubAuthStatus implements the interface
func (d *DefaultGitHubAuthChecker) CheckGitHubAuthStatus() (*AuthUser, error) {
	// First try reading the hosts.yml file
	if user, err := d.readGitHubHosts(); err == nil && user != nil {
		return user, nil
	}
	// Fallback to running gh auth status command
	return d.runGitHubAuthStatus()
}

// AuthHandler handles authentication flows
type AuthHandler struct {
	activeAuth  *AuthProcess
	authMutex   sync.Mutex
	authChecker GitHubAuthChecker
}

// AuthProcess represents an active authentication process
type AuthProcess struct {
	Cmd       *exec.Cmd
	Code      string
	URL       string
	Status    string // "pending", "waiting", "success", "error"
	Error     string
	StartedAt time.Time
}

// AuthStartResponse represents the auth start response
// @Description Response when starting GitHub device flow authentication
type AuthStartResponse struct {
	// Device verification code to enter on GitHub
	Code string `json:"code" example:"1234-5678"`
	// GitHub device activation URL
	URL string `json:"url" example:"https://github.com/login/device"`
	// Current authentication status
	Status string `json:"status" example:"waiting"`
}

// AuthStatusResponse represents the auth status response
// @Description Response containing the current authentication status
type AuthStatusResponse struct {
	// Authentication status: pending, waiting, success, none, or error
	Status string `json:"status" example:"success"`
	// Error message if authentication failed
	Error string `json:"error,omitempty" example:"authentication timeout"`
	// User information when authenticated
	User *AuthUser `json:"user,omitempty"`
}

// AuthUser represents authenticated user information
// @Description User information when authenticated with GitHub
type AuthUser struct {
	// GitHub username
	Username string `json:"username" example:"vanpelt"`
	// Token scopes
	Scopes []string `json:"scopes" example:"repo,read:org,workflow"`
}

// GitHubHosts represents the structure of GitHub CLI hosts.yml file
type GitHubHosts struct {
	GitHubCom GitHubHost `yaml:"github.com"`
}

type GitHubHost struct {
	Users      map[string]GitHubUser `yaml:"users"`
	OAuthToken string                `yaml:"oauth_token"`
	User       string                `yaml:"user"`
}

type GitHubUser struct {
	OAuthToken string `yaml:"oauth_token"`
}

// NewAuthHandler creates a new auth handler with default GitHub auth checker
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		authChecker: &DefaultGitHubAuthChecker{},
	}
}

// NewAuthHandlerWithChecker creates a new auth handler with a custom GitHub auth checker (for testing)
func NewAuthHandlerWithChecker(checker GitHubAuthChecker) *AuthHandler {
	return &AuthHandler{
		authChecker: checker,
	}
}

// readGitHubHosts reads the GitHub CLI hosts.yml file
func (h *AuthHandler) readGitHubHosts() (*AuthUser, error) {
	hostsPath := filepath.Join(config.Runtime.HomeDir, ".config", "gh", "hosts.yml")

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return nil, err
	}

	var hosts GitHubHosts
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return nil, err
	}

	if hosts.GitHubCom.User == "" {
		return nil, fmt.Errorf("no authenticated user found")
	}

	// Get token scopes using gh command
	scopes := h.getTokenScopes()

	return &AuthUser{
		Username: hosts.GitHubCom.User,
		Scopes:   scopes,
	}, nil
}

// runGitHubAuthStatus runs gh auth status command
func (h *AuthHandler) runGitHubAuthStatus() (*AuthUser, error) {
	cmd := exec.Command("bash", "--login", "-c", "gh auth status --show-token 2>/dev/null")

	// In containerized mode, override HOME for catnip user
	// In native mode, use the existing environment
	if config.Runtime.IsContainerized() {
		cmd.Env = append(os.Environ(),
			"HOME="+config.Runtime.HomeDir,
		)
	} else {
		cmd.Env = os.Environ()
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	outputStr := string(output)

	// Parse username
	usernameRegex := regexp.MustCompile(`account (\w+)`)
	usernameMatches := usernameRegex.FindStringSubmatch(outputStr)
	if len(usernameMatches) < 2 {
		return nil, fmt.Errorf("could not parse username")
	}

	// Get token scopes
	scopes := h.getTokenScopes()

	return &AuthUser{
		Username: usernameMatches[1],
		Scopes:   scopes,
	}, nil
}

// getTokenScopes gets the token scopes from gh auth status
func (h *AuthHandler) getTokenScopes() []string {
	cmd := exec.Command("bash", "--login", "-c", "gh auth status 2>&1 | grep 'Token scopes'")

	// In containerized mode, override HOME for catnip user
	// In native mode, use the existing environment
	if config.Runtime.IsContainerized() {
		cmd.Env = append(os.Environ(),
			"HOME="+config.Runtime.HomeDir,
		)
	} else {
		cmd.Env = os.Environ()
	}

	output, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	outputStr := string(output)
	scopesRegex := regexp.MustCompile(`Token scopes: '(.+)'`)
	scopesMatches := scopesRegex.FindStringSubmatch(outputStr)
	if len(scopesMatches) < 2 {
		return []string{}
	}

	// Split by ', ' and clean up each scope
	scopesStr := scopesMatches[1]
	scopes := strings.Split(scopesStr, "', '")

	// Clean up the scopes - remove any remaining quotes and whitespace
	for i, scope := range scopes {
		scopes[i] = strings.Trim(scope, "' ")
	}

	return scopes
}

// StartGitHubAuth starts the GitHub authentication flow
// @Summary Start GitHub authentication
// @Description Initiates GitHub device flow authentication
// @Tags auth
// @Success 200 {object} AuthStartResponse
// @Router /v1/auth/github/start [post]
func (h *AuthHandler) StartGitHubAuth(c *fiber.Ctx) error {
	h.authMutex.Lock()

	// Kill any existing auth process
	if h.activeAuth != nil && h.activeAuth.Cmd != nil && h.activeAuth.Cmd.Process != nil {
		_ = h.activeAuth.Cmd.Process.Kill()
		_ = h.activeAuth.Cmd.Wait()
		h.activeAuth = nil
	}

	// Start new auth process with workflow scope for GitHub Actions support
	cmd := exec.Command("bash", "--login", "-c", "gh auth login --web --scopes 'repo,read:org,workflow' 2>&1")

	// In containerized mode, override HOME for catnip user
	// In native mode, use the existing environment
	if config.Runtime.IsContainerized() {
		cmd.Env = append(os.Environ(),
			"HOME="+config.Runtime.HomeDir,
		)
	} else {
		cmd.Env = os.Environ()
	}

	// Set stdin to null to avoid hanging on prompts
	cmd.Stdin = nil

	// Create pipes for stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Errorf("❌ Failed to create stdout pipe: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to start authentication"})
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		logger.Errorf("❌ Failed to start auth command: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to start authentication"})
	}

	h.activeAuth = &AuthProcess{
		Cmd:       cmd,
		Status:    "pending",
		StartedAt: time.Now(),
	}

	// Parse output in goroutine
	go h.parseAuthOutput(stdout)

	// Monitor process completion in a separate goroutine that doesn't block
	go func() {
		err := h.activeAuth.Cmd.Wait()
		h.authMutex.Lock()
		defer h.authMutex.Unlock()

		// Check if activeAuth is still valid (might have been reset)
		if h.activeAuth == nil {
			return
		}

		if err != nil && h.activeAuth.Status != "success" {
			logger.Errorf("❌ Auth process error: %v", err)
			h.activeAuth.Status = "error"
			h.activeAuth.Error = fmt.Sprintf("Authentication failed: %v", err)
		} else if h.activeAuth.Status != "error" {
			logger.Info("✅ Auth process completed successfully")
			h.activeAuth.Status = "success"
		}
	}()

	// Release the mutex before entering the wait loop to avoid deadlock
	h.authMutex.Unlock()

	// Wait for code to be parsed with timeout (max 10 seconds)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var code, url string
	for {
		select {
		case <-timeout:
			// Kill the process if it's still running
			if h.activeAuth.Cmd != nil && h.activeAuth.Cmd.Process != nil {
				_ = h.activeAuth.Cmd.Process.Kill()
			}
			return c.Status(408).JSON(fiber.Map{"error": "Authentication timeout - please try again"})

		case <-ticker.C:
			h.authMutex.Lock()
			code = h.activeAuth.Code
			url = h.activeAuth.URL
			status := h.activeAuth.Status
			h.authMutex.Unlock()

			if code != "" && url != "" {
				return c.JSON(AuthStartResponse{
					Code:   code,
					URL:    url,
					Status: status,
				})
			}
		}
	}
}

// GetAuthStatus returns the current auth status
// @Summary Get authentication status
// @Description Returns the current status of the authentication flow
// @Tags auth
// @Success 200 {object} AuthStatusResponse
// @Router /v1/auth/github/status [get]
func (h *AuthHandler) GetAuthStatus(c *fiber.Ctx) error {
	h.authMutex.Lock()
	defer h.authMutex.Unlock()

	// If there's an active auth process, return its status
	if h.activeAuth != nil {
		return c.JSON(AuthStatusResponse{
			Status: h.activeAuth.Status,
			Error:  h.activeAuth.Error,
		})
	}

	// Check if user is already authenticated via GitHub CLI
	user, err := h.authChecker.CheckGitHubAuthStatus()
	if err == nil && user != nil {
		return c.JSON(AuthStatusResponse{
			Status: "authenticated",
			User:   user,
		})
	}

	// No active auth and not authenticated
	return c.JSON(AuthStatusResponse{
		Status: "none",
	})
}

// ResetAuthState resets the current authentication state
// @Summary Reset authentication state
// @Description Clears any active authentication process
// @Tags auth
// @Success 200 {object} map[string]string
// @Router /v1/auth/github/reset [post]
func (h *AuthHandler) ResetAuthState(c *fiber.Ctx) error {
	h.authMutex.Lock()
	defer h.authMutex.Unlock()

	// Kill any existing auth process
	if h.activeAuth != nil && h.activeAuth.Cmd != nil && h.activeAuth.Cmd.Process != nil {
		_ = h.activeAuth.Cmd.Process.Kill()
		_ = h.activeAuth.Cmd.Wait()
	}

	// Clear the active auth state
	h.activeAuth = nil

	return c.JSON(fiber.Map{"status": "reset"})
}

func (h *AuthHandler) parseAuthOutput(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	codeRegex := regexp.MustCompile(`! First copy your one-time code: ([A-Z0-9]{4}-[A-Z0-9]{4})`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for code
		if matches := codeRegex.FindStringSubmatch(line); len(matches) > 1 {
			h.authMutex.Lock()
			if h.activeAuth != nil {
				h.activeAuth.Code = matches[1]
				// Set the known GitHub device URL when we get the code
				h.activeAuth.URL = "https://github.com/login/device"
				h.activeAuth.Status = "waiting"
			}
			h.authMutex.Unlock()
		}

		// Check for success indicators
		if strings.Contains(line, "Logged in as") || strings.Contains(line, "✓ Logged in") {
			h.authMutex.Lock()
			if h.activeAuth != nil {
				h.activeAuth.Status = "success"
			}
			h.authMutex.Unlock()
		}
	}
}
