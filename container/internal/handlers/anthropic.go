package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// AnthropicHandler handles Anthropic API requests
type AnthropicHandler struct {
	anthropicService *services.AnthropicService
}

// NewAnthropicHandler creates a new Anthropic handler
func NewAnthropicHandler(anthropicService *services.AnthropicService) *AnthropicHandler {
	return &AnthropicHandler{
		anthropicService: anthropicService,
	}
}

// CreateMessageRequest represents a request to create a message
// @Description Request to create a message using the Anthropic API
type CreateMessageRequest struct {
	// The model to use for the request
	Model string `json:"model,omitempty" example:"claude-3-5-sonnet-20241022"`
	// Maximum number of tokens to generate
	MaxTokens int `json:"max_tokens,omitempty" example:"4000"`
	// Array of messages in the conversation
	Messages []services.AnthropicMessage `json:"messages"`
}

// CreateMessage creates a message using the Anthropic API
// @Summary Create a message
// @Description Send a message to the Anthropic API and get a response
// @Tags anthropic
// @Accept json
// @Produce json
// @Param request body CreateMessageRequest true "Message request"
// @Success 200 {object} services.AnthropicResponse
// @Failure 400 {object} fiber.Map
// @Failure 500 {object} fiber.Map
// @Router /v1/anthropic/messages [post]
func (h *AnthropicHandler) CreateMessage(c *fiber.Ctx) error {
	var req CreateMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if len(req.Messages) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Messages array cannot be empty",
		})
	}

	// Use default model if not specified
	if req.Model == "" {
		req.Model = h.anthropicService.GetDefaultModel()
	}

	// Use default max tokens if not specified
	if req.MaxTokens == 0 {
		req.MaxTokens = 4000
	}

	response, err := h.anthropicService.CreateMessage(req.Model, req.Messages, req.MaxTokens)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(response)
}