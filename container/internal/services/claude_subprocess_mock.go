package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vanpelt/catnip/internal/models"
)

// MockClaudeSubprocessWrapper is a mock implementation for testing
type MockClaudeSubprocessWrapper struct {
	// Configure mock behavior
	ShouldFail        bool
	FailureError      string
	MockResponse      string
	StreamingResponse string
}

// NewMockClaudeSubprocessWrapper creates a new mock wrapper for testing
func NewMockClaudeSubprocessWrapper() *MockClaudeSubprocessWrapper {
	return &MockClaudeSubprocessWrapper{
		ShouldFail:        false,
		MockResponse:      "Mock claude response",
		StreamingResponse: "Mock streaming response",
	}
}

// CreateCompletion mock implementation
func (m *MockClaudeSubprocessWrapper) CreateCompletion(ctx context.Context, opts *ClaudeSubprocessOptions) (*models.CreateCompletionResponse, error) {
	if m.ShouldFail {
		errorMsg := m.FailureError
		if errorMsg == "" {
			errorMsg = "mock claude CLI failure"
		}
		return &models.CreateCompletionResponse{
			Error: errorMsg,
		}, fmt.Errorf("claude command failed: %s", errorMsg)
	}

	return &models.CreateCompletionResponse{
		Response: m.MockResponse,
		IsChunk:  false,
		IsLast:   true,
	}, nil
}

// CreateStreamingCompletion mock implementation
func (m *MockClaudeSubprocessWrapper) CreateStreamingCompletion(ctx context.Context, opts *ClaudeSubprocessOptions, responseWriter io.Writer) error {
	if m.ShouldFail {
		errorMsg := m.FailureError
		if errorMsg == "" {
			errorMsg = "mock claude CLI failure"
		}

		// Write error response as JSON
		errorResponse := &models.CreateCompletionResponse{
			Error:   errorMsg,
			IsChunk: true,
			IsLast:  true,
		}

		responseJSON, _ := json.Marshal(errorResponse)
		if _, err := responseWriter.Write(append(responseJSON, '\n')); err != nil {
			return fmt.Errorf("failed to write error response: %w", err)
		}

		return fmt.Errorf("claude command failed: %s", errorMsg)
	}

	// Write successful streaming response
	response := &models.CreateCompletionResponse{
		Response: m.StreamingResponse,
		IsChunk:  true,
		IsLast:   false,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if _, err := responseWriter.Write(append(responseJSON, '\n')); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	// Write final chunk
	finalResponse := &models.CreateCompletionResponse{
		IsChunk: true,
		IsLast:  true,
	}

	finalJSON, err := json.Marshal(finalResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal final response: %w", err)
	}

	if _, err := responseWriter.Write(append(finalJSON, '\n')); err != nil {
		return fmt.Errorf("failed to write final response: %w", err)
	}

	return nil
}
