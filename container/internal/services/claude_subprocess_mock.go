package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/vanpelt/catnip/internal/models"
)

// MockClaudeSubprocessWrapper is a mock implementation for testing
type MockClaudeSubprocessWrapper struct {
	// Configure mock behavior
	ShouldFail        bool
	FailureError      string
	MockResponse      string
	StreamingResponse string
	// Track branch rename requests for testing
	BranchRenameRequests []string
}

// NewMockClaudeSubprocessWrapper creates a new mock wrapper for testing
func NewMockClaudeSubprocessWrapper() *MockClaudeSubprocessWrapper {
	return &MockClaudeSubprocessWrapper{
		ShouldFail:           false,
		MockResponse:         "Mock claude response",
		StreamingResponse:    "Mock streaming response",
		BranchRenameRequests: make([]string, 0),
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

	// Check if this is a branch naming request
	if strings.Contains(opts.Prompt, "Generate a git branch name") {
		// Extract the title from the prompt to generate a meaningful branch name
		titleStart := strings.Index(opts.Prompt, `Based on this coding session title: "`)
		if titleStart != -1 {
			titleStart += len(`Based on this coding session title: "`)
			titleEnd := strings.Index(opts.Prompt[titleStart:], `"`)
			if titleEnd != -1 {
				title := opts.Prompt[titleStart : titleStart+titleEnd]
				branchName := m.generateBranchName(title)

				// Track this request for testing verification
				m.BranchRenameRequests = append(m.BranchRenameRequests, title)

				return &models.CreateCompletionResponse{
					Response: branchName,
					IsChunk:  false,
					IsLast:   true,
				}, nil
			}
		}

		// Fallback branch name if we can't parse the title
		return &models.CreateCompletionResponse{
			Response: "feature/mock-branch",
			IsChunk:  false,
			IsLast:   true,
		}, nil
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

// generateBranchName creates a realistic branch name from a title
func (m *MockClaudeSubprocessWrapper) generateBranchName(title string) string {
	// Convert title to lowercase and replace spaces with hyphens
	branchName := strings.ToLower(title)
	branchName = strings.ReplaceAll(branchName, " ", "-")
	branchName = strings.ReplaceAll(branchName, "_", "-")

	// Remove special characters
	var cleaned strings.Builder
	for _, r := range branchName {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			cleaned.WriteRune(r)
		}
	}
	branchName = cleaned.String()

	// Trim excessive hyphens and limit length
	branchName = strings.Trim(branchName, "-")
	if len(branchName) > 50 {
		branchName = branchName[:50]
		branchName = strings.Trim(branchName, "-")
	}

	// Add appropriate prefix based on content
	if strings.Contains(strings.ToLower(title), "fix") || strings.Contains(strings.ToLower(title), "bug") {
		return "fix/" + branchName
	} else if strings.Contains(strings.ToLower(title), "test") {
		return "test/" + branchName
	} else if strings.Contains(strings.ToLower(title), "refactor") {
		return "refactor/" + branchName
	} else if strings.Contains(strings.ToLower(title), "update") || strings.Contains(strings.ToLower(title), "upgrade") {
		return "chore/" + branchName
	} else {
		return "feature/" + branchName
	}
}
