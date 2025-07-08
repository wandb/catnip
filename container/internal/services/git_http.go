package services

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/models"
)

// GitHTTPService provides Git HTTP protocol endpoints for repository access
// This allows users to fetch changes from the container's bare repository
type GitHTTPService struct {
	gitService *GitService
}

// NewGitHTTPService creates a new Git HTTP service
func NewGitHTTPService(gitService *GitService) *GitHTTPService {
	return &GitHTTPService{
		gitService: gitService,
	}
}

// RegisterRoutes registers Git HTTP protocol routes
func (ghs *GitHTTPService) RegisterRoutes(app *fiber.App) {
	// Handle .git URLs - this catches repo.git/path patterns
	app.Use("/*.git/*", ghs.handleGitHTTP)
	app.Use("/*.git", ghs.handleGitHTTP)
}

// handleGitHTTP handles Git HTTP protocol requests using git http-backend
func (ghs *GitHTTPService) handleGitHTTP(c *fiber.Ctx) error {
	path := c.Path()
	
	// Git HTTP request received
	
	// Extract repo name from path (e.g., "/repo.git" or "/repo.git/info/refs")
	pathParts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(pathParts) == 0 {
		return c.Status(404).SendString("Repository not found")
	}
	
	repoWithGit := pathParts[0] // e.g., "repo.git"
	if !strings.HasSuffix(repoWithGit, ".git") {
		return c.Status(404).SendString("Invalid repository URL")
	}
	
	repoName := strings.TrimSuffix(repoWithGit, ".git")
	
	// Find the repository by name from all loaded repositories
	status := ghs.gitService.GetStatus()
	var targetRepo *models.Repository
	
	// Search through all repositories to find one matching the requested repo name
	for _, repo := range status.Repositories {
		// Extract repo name from repository ID (e.g., "owner/repo" -> "repo")
		repoParts := strings.Split(repo.ID, "/")
		actualRepoName := repoParts[len(repoParts)-1]
		
		if repoName == actualRepoName {
			targetRepo = repo
			break
		}
	}
	
	if targetRepo == nil {
		return c.Status(404).SendString("Repository not found")
	}
	
	bareRepoPath := targetRepo.Path
	
	// Create a unique temporary symlink structure for git http-backend
	// git http-backend expects the repository at $GIT_PROJECT_ROOT/$PATH_INFO
	// Use repo name to avoid conflicts between different repositories
	tempDir := fmt.Sprintf("/tmp/git-http-%s", repoName)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("❌ Failed to create temp directory: %v", err)
		return c.Status(500).SendString("Internal server error")
	}
	
	symlinkPath := filepath.Join(tempDir, repoWithGit)
	
	// Remove existing symlink if it exists
	os.Remove(symlinkPath)
	
	// Create symlink to bare repository
	if err := os.Symlink(bareRepoPath, symlinkPath); err != nil {
		log.Printf("❌ Failed to create symlink: %v", err)
		return c.Status(500).SendString("Internal server error")
	}
	
	// Clean up symlink after request
	defer os.Remove(symlinkPath)
	
	// Set up CGI environment for git http-backend
	env := append(os.Environ(),
		fmt.Sprintf("GIT_PROJECT_ROOT=%s", tempDir),
		"GIT_HTTP_EXPORT_ALL=1",
		fmt.Sprintf("PATH_INFO=%s", path),
		fmt.Sprintf("REQUEST_METHOD=%s", c.Method()),
		fmt.Sprintf("QUERY_STRING=%s", c.Context().QueryArgs().String()),
		fmt.Sprintf("REQUEST_URI=%s", c.OriginalURL()),
		fmt.Sprintf("CONTENT_TYPE=%s", c.Get("Content-Type")),
		fmt.Sprintf("CONTENT_LENGTH=%d", len(c.Body())),
		"SERVER_SOFTWARE=catnip-git-server/1.0",
		"GATEWAY_INTERFACE=CGI/1.1",
		"SERVER_PROTOCOL=HTTP/1.1",
		fmt.Sprintf("SERVER_NAME=%s", c.Hostname()),
		"SERVER_PORT=8080",
		fmt.Sprintf("REMOTE_ADDR=%s", c.IP()),
		"HOME=/home/catnip",
		"USER=catnip",
	)
	
	// Add HTTP headers as environment variables
	c.Request().Header.VisitAll(func(key, value []byte) {
		headerKey := fmt.Sprintf("HTTP_%s", strings.ToUpper(strings.ReplaceAll(string(key), "-", "_")))
		env = append(env, fmt.Sprintf("%s=%s", headerKey, string(value)))
	})
	
	// Check what refs exist in the bare repository (silent)

	// Execute git http-backend
	cmd := exec.Command("git", "http-backend")
	cmd.Env = env
	cmd.Dir = tempDir
	
	// Set up I/O
	cmd.Stdin = bytes.NewReader(c.Body())
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		log.Printf("❌ Git http-backend error: %v, stderr: %s", err, stderr.String())
		return c.Status(500).SendString("Git operation failed")
	}
	
	// Parse CGI response (work with bytes to preserve binary data)
	responseBytes := stdout.Bytes()
	// Git response processed
	
	// Find the end of headers (double CRLF)
	headerEndIndex := bytes.Index(responseBytes, []byte("\r\n\r\n"))
	if headerEndIndex != -1 {
		// Split headers and body
		headerBytes := responseBytes[:headerEndIndex]
		bodyBytes := responseBytes[headerEndIndex+4:] // Skip the \r\n\r\n
		
		// Parse and set headers
		headerLines := strings.Split(string(headerBytes), "\r\n")
		for _, line := range headerLines {
			if line == "" {
				continue
			}
			headerParts := strings.SplitN(line, ": ", 2)
			if len(headerParts) == 2 {
				c.Set(headerParts[0], headerParts[1])
			}
		}
		
		// Send binary body (preserves Git protocol data)
		return c.Send(bodyBytes)
	} else {
		// No proper CGI response, check if it's just headers
		if len(responseBytes) > 0 {
			// Might be headers only, try to parse
			headerLines := strings.Split(string(responseBytes), "\r\n")
			hasValidHeaders := false
			for _, line := range headerLines {
				if line == "" {
					continue
				}
				headerParts := strings.SplitN(line, ": ", 2)
				if len(headerParts) == 2 {
					c.Set(headerParts[0], headerParts[1])
					hasValidHeaders = true
				}
			}
			
			if hasValidHeaders {
				return c.SendStatus(200) // Headers only response
			}
		}
		
		// Fallback: send as binary
		c.Set("Content-Type", "application/octet-stream")
		return c.Send(responseBytes)
	}
}

// GetRepositoryCloneURL returns the HTTP clone URL for a specific repository
func (ghs *GitHTTPService) GetRepositoryCloneURL(baseURL, repoID string) string {
	repo := ghs.gitService.GetRepositoryByID(repoID)
	if repo == nil {
		return ""
	}
	
	// Extract repo name from repository ID
	repoParts := strings.Split(repo.ID, "/")
	repoName := repoParts[len(repoParts)-1]
	
	return fmt.Sprintf("%s/%s.git", baseURL, repoName)
}

// GetAllRepositoryCloneURLs returns HTTP clone URLs for all loaded repositories
func (ghs *GitHTTPService) GetAllRepositoryCloneURLs(baseURL string) map[string]string {
	status := ghs.gitService.GetStatus()
	urls := make(map[string]string)
	
	for repoID, repo := range status.Repositories {
		repoParts := strings.Split(repo.ID, "/")
		repoName := repoParts[len(repoParts)-1]
		urls[repoID] = fmt.Sprintf("%s/%s.git", baseURL, repoName)
	}
	
	return urls
}