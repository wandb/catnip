package handlers

import (
	"fmt"
	"log"
	
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// GitHandler handles Git-related API endpoints
type GitHandler struct {
	gitService     *services.GitService
	gitHTTPService *services.GitHTTPService
}

// NewGitHandler creates a new Git handler
func NewGitHandler(gitService *services.GitService, gitHTTPService *services.GitHTTPService) *GitHandler {
	return &GitHandler{
		gitService:     gitService,
		gitHTTPService: gitHTTPService,
	}
}

// CheckoutRepository handles repository checkout requests
// @Summary Checkout a GitHub repository
// @Description Clones a GitHub repository as a bare repo and creates initial worktree
// @Tags git
// @Accept json
// @Produce json
// @Param org path string true "Organization name"
// @Param repo path string true "Repository name"
// @Param branch query string false "Branch name (optional)"
// @Success 200 {object} map[string]interface{}
// @Router /v1/git/checkout/{org}/{repo} [post]
func (h *GitHandler) CheckoutRepository(c *fiber.Ctx) error {
	org := c.Params("org")
	repo := c.Params("repo")
	branch := c.Query("branch", "")
	
	log.Printf("📦 Checkout request: %s/%s (branch: %s)", org, repo, branch)
	
	repository, worktree, err := h.gitService.CheckoutRepository(org, repo, branch)
	if err != nil {
		log.Printf("❌ Checkout failed: %v", err)
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

// GetStatus returns the current Git status
// @Summary Get Git status
// @Description Returns the current repository and worktree status
// @Tags git
// @Produce json
// @Success 200 {object} models.GitStatus
// @Router /v1/git/status [get]
func (h *GitHandler) GetStatus(c *fiber.Ctx) error {
	status := h.gitService.GetStatus()
	return c.JSON(status)
}

// CreateWorktree creates a new worktree
// @Summary Create a new worktree
// @Description Creates a new worktree from a branch or commit
// @Tags git
// @Accept json
// @Produce json
// @Param request body models.WorktreeCreateRequest true "Worktree creation request"
// @Success 200 {object} models.Worktree
// @Router /v1/git/worktrees [post]
func (h *GitHandler) CreateWorktree(c *fiber.Ctx) error {
	var req models.WorktreeCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	if req.Source == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Source branch or commit is required",
		})
	}
	
	if req.Name == "" {
		req.Name = req.Source // Use source as name if not provided
	}
	
	worktree, err := h.gitService.CreateWorktree(req.Source, req.Name)
	if err != nil {
		log.Printf("❌ Failed to create worktree: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	log.Printf("✅ Created worktree: %s", worktree.Name)
	return c.JSON(worktree)
}

// ListWorktrees returns all worktrees
// @Summary List all worktrees
// @Description Returns a list of all worktrees for the current repository
// @Tags git
// @Produce json
// @Success 200 {array} models.Worktree
// @Router /v1/git/worktrees [get]
func (h *GitHandler) ListWorktrees(c *fiber.Ctx) error {
	worktrees := h.gitService.ListWorktrees()
	return c.JSON(worktrees)
}

// ActivateWorktree switches to a different worktree
// @Summary Activate a worktree
// @Description Switches the active worktree
// @Tags git
// @Produce json
// @Param id path string true "Worktree ID"
// @Success 200 {object} map[string]string
// @Router /v1/git/worktrees/{id}/activate [post]
func (h *GitHandler) ActivateWorktree(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	
	if err := h.gitService.ActivateWorktree(worktreeID); err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	return c.JSON(fiber.Map{
		"message": "Worktree activated successfully",
		"id":      worktreeID,
	})
}

// TriggerSync manually triggers commit synchronization
// @Summary Trigger commit sync
// @Description Manually triggers synchronization of commits from worktrees to bare repository
// @Tags git
// @Produce json
// @Success 200 {object} map[string]string
// @Router /v1/git/sync [post]
func (h *GitHandler) TriggerSync(c *fiber.Ctx) error {
	// Trigger manual sync via the git service
	err := h.gitService.TriggerManualSync()
	if err != nil {
		return c.JSON(fiber.Map{
			"message": fmt.Sprintf("Sync failed: %v", err),
			"status":  "error",
		})
	}
	
	return c.JSON(fiber.Map{
		"message": "Manual sync triggered successfully",
		"status":  "running",
	})
}

// GetCloneURL returns the Git clone URL for the current repository
// @Summary Get Git clone URL
// @Description Returns the HTTP clone URL for the current repository
// @Tags git
// @Produce json
// @Success 200 {object} map[string]string
// @Router /v1/git/clone-url [get]
func (h *GitHandler) GetCloneURL(c *fiber.Ctx) error {
	baseURL := fmt.Sprintf("%s://%s", c.Protocol(), c.Get("Host"))
	cloneURLs := h.gitHTTPService.GetAllRepositoryCloneURLs(baseURL)
	
	if len(cloneURLs) == 0 {
		return c.Status(404).JSON(fiber.Map{
			"error": "No repositories currently loaded",
		})
	}
	
	return c.JSON(fiber.Map{
		"clone_urls": cloneURLs,
		"message":    "Use these URLs to clone repositories locally",
	})
}