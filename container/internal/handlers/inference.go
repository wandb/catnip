package handlers

import (
	"fmt"
	"runtime"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/services"
)

// InferenceHandler handles local GGUF model inference requests
type InferenceHandler struct {
	service *services.InferenceService
}

// NewInferenceHandler creates a new inference handler
func NewInferenceHandler(service *services.InferenceService) *InferenceHandler {
	return &InferenceHandler{
		service: service,
	}
}

// SummarizeRequest represents a summarization request
// @Description Request to summarize a task and generate a branch name
type SummarizeRequest struct {
	// Task description or code changes to summarize
	Prompt string `json:"prompt" example:"Add user authentication with OAuth2"`
}

// SummarizeResponse represents a summarization response
// @Description Response containing task summary and suggested branch name
type SummarizeResponse struct {
	// 2-4 word summary in Title Case
	Summary string `json:"summary" example:"Add User Auth"`
	// Git branch name in kebab-case with category prefix
	BranchName string `json:"branchName" example:"feat/add-user-auth"`
}

// InferenceStatusResponse represents the inference service status
// @Description Status of the local inference service
type InferenceStatusResponse struct {
	// Whether inference is available on this platform
	Available bool `json:"available" example:"true"`
	// Platform name (darwin, linux, windows)
	Platform string `json:"platform" example:"darwin"`
	// Architecture (amd64, arm64)
	Architecture string `json:"architecture" example:"arm64"`
	// Model path if loaded
	ModelPath string `json:"modelPath,omitempty" example:"/Users/user/.catnip/models/gemma3-270m-summarizer-Q4_K_M.gguf"`
	// Error message if initialization failed
	Error string `json:"error,omitempty" example:"model not found"`
}

// HandleSummarize godoc
// @Summary Summarize task and generate branch name
// @Description Generate a short task summary and git branch name using local GGUF model
// @Tags inference
// @Accept json
// @Produce json
// @Param request body SummarizeRequest true "Summarization request"
// @Success 200 {object} SummarizeResponse "Successfully generated summary and branch name"
// @Failure 400 {object} fiber.Map "Invalid request"
// @Failure 500 {object} fiber.Map "Inference error"
// @Failure 503 {object} fiber.Map "Inference not available on this platform"
// @Router /v1/inference/summarize [post]
func (h *InferenceHandler) HandleSummarize(c *fiber.Ctx) error {
	// Check if service is available
	if h.service == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Inference service not available on this platform",
		})
	}

	// Parse request
	var req SummarizeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate prompt
	if req.Prompt == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Prompt is required",
		})
	}

	logger.Debugf("🧠 Inference request: %s", req.Prompt)

	// Generate summary
	result, err := h.service.Summarize(req.Prompt)
	if err != nil {
		logger.Errorf("Inference error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to generate summary: %v", err),
		})
	}

	logger.Debugf("✅ Inference result: summary=%s, branch=%s", result.Summary, result.BranchName)

	return c.JSON(SummarizeResponse{
		Summary:    result.Summary,
		BranchName: result.BranchName,
	})
}

// HandleInferenceStatus godoc
// @Summary Get inference service status
// @Description Check if local inference is available and get service information
// @Tags inference
// @Produce json
// @Success 200 {object} InferenceStatusResponse "Inference service status"
// @Router /v1/inference/status [get]
func (h *InferenceHandler) HandleInferenceStatus(c *fiber.Ctx) error {
	status := InferenceStatusResponse{
		Available:    h.service != nil,
		Platform:     runtime.GOOS,
		Architecture: runtime.GOARCH,
	}

	if h.service != nil {
		// Try to get model path (implementation would need to expose this)
		status.ModelPath = "~/.catnip/models/gemma3-270m-summarizer-Q4_K_M.gguf"
	} else {
		status.Error = "Inference only available on macOS currently"
	}

	return c.JSON(status)
}
