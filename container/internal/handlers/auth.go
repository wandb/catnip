package handlers

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// AuthHandler handles authentication flows
type AuthHandler struct {
	activeAuth      *AuthProcess
	authMutex       sync.Mutex
}

// AuthProcess represents an active authentication process
type AuthProcess struct {
	Cmd         *exec.Cmd
	Code        string
	URL         string
	Status      string // "pending", "waiting", "success", "error"
	Error       string
	StartedAt   time.Time
}

// AuthStartResponse represents the auth start response
// @Description Response when starting GitHub device flow authentication
type AuthStartResponse struct {
	Code   string `json:"code" example:"1234-5678" description:"Device verification code to enter on GitHub"`
	URL    string `json:"url" example:"https://github.com/login/device" description:"GitHub device activation URL"`
	Status string `json:"status" example:"waiting" description:"Current authentication status"`
}

// AuthStatusResponse represents the auth status response  
// @Description Response containing the current authentication status
type AuthStatusResponse struct {
	Status string `json:"status" example:"success" description:"Authentication status: pending, waiting, success, or error"`
	Error  string `json:"error,omitempty" example:"authentication timeout" description:"Error message if authentication failed"`
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
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
		h.activeAuth.Cmd.Process.Kill()
		h.activeAuth.Cmd.Wait()
		h.activeAuth = nil
	}

	// Start new auth process
	cmd := exec.Command("bash", "--login", "-c", "gh auth login --web 2>&1")
	cmd.Env = append(os.Environ(),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	// Set stdin to null to avoid hanging on prompts
	cmd.Stdin = nil

	// Create pipes for stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("❌ Failed to create stdout pipe: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to start authentication"})
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("❌ Failed to start auth command: %v", err)
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
		
		if err != nil && h.activeAuth.Status != "success" {
			log.Printf("❌ Auth process error: %v", err)
			h.activeAuth.Status = "error"
			h.activeAuth.Error = fmt.Sprintf("Authentication failed: %v", err)
		} else if h.activeAuth.Status != "error" {
			log.Printf("✅ Auth process completed successfully")
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
				h.activeAuth.Cmd.Process.Kill()
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

	if h.activeAuth == nil {
		return c.JSON(AuthStatusResponse{
			Status: "none",
		})
	}

	return c.JSON(AuthStatusResponse{
		Status: h.activeAuth.Status,
		Error:  h.activeAuth.Error,
	})
}

func (h *AuthHandler) parseAuthOutput(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	codeRegex := regexp.MustCompile(`! First copy your one-time code: ([A-Z0-9]{4}-[A-Z0-9]{4})`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for code
		if matches := codeRegex.FindStringSubmatch(line); len(matches) > 1 {
			h.authMutex.Lock()
			h.activeAuth.Code = matches[1]
			// Set the known GitHub device URL when we get the code
			h.activeAuth.URL = "https://github.com/login/device"
			h.activeAuth.Status = "waiting"
			h.authMutex.Unlock()
		}

		// Check for success indicators
		if strings.Contains(line, "Logged in as") || strings.Contains(line, "✓ Logged in") {
			h.authMutex.Lock()
			h.activeAuth.Status = "success"
			h.authMutex.Unlock()
		}
	}
}

