package handlers

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// TestHandler exposes test-only helpers when CATNIP_TEST_MODE=1
type TestHandler struct {
	gitService    *services.GitService
	claudeMonitor *services.ClaudeMonitorService
}

func NewTestHandler(git *services.GitService, monitor *services.ClaudeMonitorService) *TestHandler {
	return &TestHandler{gitService: git, claudeMonitor: monitor}
}

type simulateTitleRequest struct {
	Title string        `json:"title"`
	Todos []models.Todo `json:"todos"`
}

// SimulateTitle applies a session title and optional todos to a worktree
func (h *TestHandler) SimulateTitle(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	if worktreeID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "worktree id is required"})
	}

	var req simulateTitleRequest
	if err := c.BodyParser(&req); err != nil || req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "title is required"})
	}

	wt, ok := h.gitService.GetWorktree(worktreeID)
	if !ok || wt == nil {
		return c.Status(404).JSON(fiber.Map{"error": "worktree not found"})
	}

	// Notify title change (updates session service and may trigger rename)
	h.claudeMonitor.NotifyTitleChange(wt.Path, req.Title)

	// Optionally set todos directly in state for UI
	if len(req.Todos) > 0 {
		_ = h.gitService.UpdateWorktreeFields(worktreeID, map[string]interface{}{
			"todos": req.Todos,
		})
	}

	return c.JSON(fiber.Map{"ok": true})
}

type simulateFileRequest struct {
	RelPath string `json:"path"`
	Content string `json:"content"`
}

// SimulateFile writes a file within the worktree to create a change
func (h *TestHandler) SimulateFile(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	if worktreeID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "worktree id is required"})
	}
	var req simulateFileRequest
	if err := c.BodyParser(&req); err != nil || req.RelPath == "" {
		return c.Status(400).JSON(fiber.Map{"error": "path is required"})
	}
	wt, ok := h.gitService.GetWorktree(worktreeID)
	if !ok || wt == nil {
		return c.Status(404).JSON(fiber.Map{"error": "worktree not found"})
	}

	// Ensure directory exists and write file
	target := filepath.Join(wt.Path, req.RelPath)
	if err := os.MkdirAll(filepath.Dir(target), fs.FileMode(0755)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if err := os.WriteFile(target, []byte(req.Content), 0644); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Refresh status so UI picks up dirtiness and diffs
	_ = h.gitService.RefreshWorktreeStatusByID(worktreeID)

	return c.JSON(fiber.Map{"ok": true})
}

type simulateRenameRequest struct {
	Branch string `json:"branch"`
}

// SimulateRename triggers automatic or custom branch rename
func (h *TestHandler) SimulateRename(c *fiber.Ctx) error {
	worktreeID := c.Params("id")
	if worktreeID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "worktree id is required"})
	}
	var req simulateRenameRequest
	_ = c.BodyParser(&req)

	wt, ok := h.gitService.GetWorktree(worktreeID)
	if !ok || wt == nil {
		return c.Status(404).JSON(fiber.Map{"error": "worktree not found"})
	}

	if err := h.claudeMonitor.TriggerBranchRename(wt.Path, req.Branch); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}
