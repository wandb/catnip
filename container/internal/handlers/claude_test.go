package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

func TestNewClaudeHandler(t *testing.T) {
	mockWrapper := services.NewMockClaudeSubprocessWrapper()
	service := services.NewClaudeServiceWithWrapper(mockWrapper)
	handler := NewClaudeHandler(service, nil)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.claudeService)
}

func TestClaudeHandler_CreateCompletion_InvalidJSON(t *testing.T) {
	mockWrapper := services.NewMockClaudeSubprocessWrapper()
	service := services.NewClaudeServiceWithWrapper(mockWrapper)
	handler := NewClaudeHandler(service, nil)

	app := fiber.New()
	app.Post("/test", handler.CreateCompletion)

	// Test with invalid JSON
	req := httptest.NewRequest("POST", "/test", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestClaudeHandler_CreateCompletion_EmptyPrompt(t *testing.T) {
	mockWrapper := services.NewMockClaudeSubprocessWrapper()
	service := services.NewClaudeServiceWithWrapper(mockWrapper)
	handler := NewClaudeHandler(service, nil)

	app := fiber.New()
	app.Post("/test", handler.CreateCompletion)

	// Test with empty prompt
	reqBody := &models.CreateCompletionRequest{
		Prompt: "", // Empty prompt should be rejected
	}

	jsonBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	// Check response body
	var responseBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	require.NoError(t, err)
	assert.Contains(t, responseBody["error"], "Prompt is required")
}

func TestClaudeHandler_CreateCompletion_ValidRequest(t *testing.T) {
	mockWrapper := services.NewMockClaudeSubprocessWrapper()
	service := services.NewClaudeServiceWithWrapper(mockWrapper)
	handler := NewClaudeHandler(service, nil)

	app := fiber.New()
	app.Post("/test", handler.CreateCompletion)

	// Test with valid request (non-streaming)
	reqBody := &models.CreateCompletionRequest{
		Prompt:           "Hello, world!",
		Stream:           false,
		SystemPrompt:     "You are helpful",
		Model:            "claude-3-5-sonnet-20241022",
		MaxTurns:         5,
		WorkingDirectory: "/tmp",
		Resume:           false,
	}

	jsonBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1) // -1 timeout for long-running test
	require.NoError(t, err)

	// Should return 200 with mock success
	assert.Equal(t, 200, resp.StatusCode)

	var responseBody models.CreateCompletionResponse
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	require.NoError(t, err)

	// Should get the mock response
	assert.Equal(t, "Mock claude response", responseBody.Response)
	assert.False(t, responseBody.IsChunk)
	assert.True(t, responseBody.IsLast)
}

func TestClaudeHandler_CreateCompletion_StreamingRequest(t *testing.T) {
	mockWrapper := services.NewMockClaudeSubprocessWrapper()
	service := services.NewClaudeServiceWithWrapper(mockWrapper)
	handler := NewClaudeHandler(service, nil)

	app := fiber.New()
	app.Post("/test", handler.CreateCompletion)

	// Test with streaming request
	reqBody := &models.CreateCompletionRequest{
		Prompt:           "Stream this response",
		Stream:           true, // Enable streaming
		SystemPrompt:     "You are helpful",
		WorkingDirectory: "/tmp",
	}

	jsonBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1) // -1 timeout for long-running test
	require.NoError(t, err)

	// Should return 200 with streaming mock success
	assert.Equal(t, 200, resp.StatusCode)

	// Check that streaming headers are set correctly
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestClaudeHandler_CreateCompletion_ResumeRequest(t *testing.T) {
	mockWrapper := services.NewMockClaudeSubprocessWrapper()
	service := services.NewClaudeServiceWithWrapper(mockWrapper)
	handler := NewClaudeHandler(service, nil)

	app := fiber.New()
	app.Post("/test", handler.CreateCompletion)

	// Test with resume request
	reqBody := &models.CreateCompletionRequest{
		Prompt: "Continue our conversation",
		Resume: true, // Enable resume
	}

	jsonBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1) // -1 timeout for long-running test
	require.NoError(t, err)

	// Should return 200 with resume mock success
	assert.Equal(t, 200, resp.StatusCode)
}

func TestClaudeHandler_CreateCompletion_MissingContentType(t *testing.T) {
	mockWrapper := services.NewMockClaudeSubprocessWrapper()
	service := services.NewClaudeServiceWithWrapper(mockWrapper)
	handler := NewClaudeHandler(service, nil)

	app := fiber.New()
	app.Post("/test", handler.CreateCompletion)

	reqBody := &models.CreateCompletionRequest{
		Prompt: "Hello",
	}

	jsonBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader(jsonBytes))
	// Missing Content-Type header

	resp, err := app.Test(req)
	require.NoError(t, err)

	// Fiber should still be able to parse JSON even without explicit Content-Type
	// With mock, should succeed or fail validation, but not call real CLI
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 400)
}
