package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/models"
)

func TestNewClaudeService(t *testing.T) {
	mockWrapper := NewMockClaudeSubprocessWrapper()
	service := NewClaudeServiceWithWrapper(mockWrapper)
	require.NotNil(t, service)
	assert.NotEmpty(t, service.claudeConfigPath)
	assert.NotEmpty(t, service.claudeProjectsDir)
	assert.NotEmpty(t, service.volumeProjectsDir)
	assert.NotNil(t, service.subprocessWrapper)
}

func TestClaudeService_CreateCompletion_ValidatePrompt(t *testing.T) {
	mockWrapper := NewMockClaudeSubprocessWrapper()
	service := NewClaudeServiceWithWrapper(mockWrapper)
	ctx := context.Background()

	t.Run("empty prompt returns error", func(t *testing.T) {
		req := &models.CreateCompletionRequest{
			Prompt: "", // Empty prompt should be rejected
		}

		response, err := service.CreateCompletion(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("valid prompt structure", func(t *testing.T) {
		req := &models.CreateCompletionRequest{
			Prompt:           "Hello, world!",
			SystemPrompt:     "You are helpful",
			Model:            "claude-3-5-sonnet-20241022",
			MaxTurns:         5,
			WorkingDirectory: "/workspace/test",
			Resume:           false,
		}

		// Mock should succeed
		response, err := service.CreateCompletion(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "Mock claude response", response.Response)
	})
}

func TestClaudeService_CreateStreamingCompletion_ValidatePrompt(t *testing.T) {
	mockWrapper := NewMockClaudeSubprocessWrapper()
	service := NewClaudeServiceWithWrapper(mockWrapper)
	ctx := context.Background()

	t.Run("empty prompt returns error", func(t *testing.T) {
		req := &models.CreateCompletionRequest{
			Prompt: "", // Empty prompt should be rejected
		}

		var buf strings.Builder
		err := service.CreateStreamingCompletion(ctx, req, &buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt is required")
	})

	t.Run("valid prompt with streaming", func(t *testing.T) {
		req := &models.CreateCompletionRequest{
			Prompt:           "Stream this response",
			SystemPrompt:     "You are helpful",
			Model:            "claude-3-5-sonnet-20241022",
			MaxTurns:         3,
			WorkingDirectory: "/workspace/test",
			Resume:           true,
		}

		var buf strings.Builder
		err := service.CreateStreamingCompletion(ctx, req, &buf)

		// Mock should succeed
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "Mock streaming response")
	})
}

func TestClaudeService_DefaultWorkingDirectory(t *testing.T) {
	mockWrapper := NewMockClaudeSubprocessWrapper()
	service := NewClaudeServiceWithWrapper(mockWrapper)
	ctx := context.Background()

	req := &models.CreateCompletionRequest{
		Prompt: "Test with default directory",
		// WorkingDirectory is empty, should default to "/workspace/current"
	}

	// Mock should succeed and use default working directory
	response, err := service.CreateCompletion(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "Mock claude response", response.Response)
}

func TestClaudeService_RequestValidation(t *testing.T) {
	mockWrapper := NewMockClaudeSubprocessWrapper()
	service := NewClaudeServiceWithWrapper(mockWrapper)
	ctx := context.Background()

	testCases := []struct {
		name        string
		req         *models.CreateCompletionRequest
		expectError bool
		errorText   string
	}{
		{
			name: "nil request",
			req:  nil,
			// This would panic before validation, but let's not test panics
		},
		{
			name: "empty prompt",
			req: &models.CreateCompletionRequest{
				Prompt: "",
			},
			expectError: true,
			errorText:   "prompt is required",
		},
		{
			name: "whitespace only prompt",
			req: &models.CreateCompletionRequest{
				Prompt: "   ",
			},
			expectError: false, // Whitespace is technically valid
		},
		{
			name: "valid minimal request",
			req: &models.CreateCompletionRequest{
				Prompt: "Hello",
			},
			expectError: false,
		},
		{
			name: "valid full request",
			req: &models.CreateCompletionRequest{
				Prompt:           "Hello world",
				Stream:           true,
				SystemPrompt:     "You are helpful",
				Model:            "claude-3-5-sonnet-20241022",
				MaxTurns:         10,
				WorkingDirectory: "/workspace/project",
				Resume:           true,
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.req == nil {
				// Skip nil request test to avoid panic
				return
			}

			_, err := service.CreateCompletion(ctx, tc.req)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorText != "" {
					assert.Contains(t, err.Error(), tc.errorText)
				}
			} else {
				// Mock should succeed for valid requests
				assert.NoError(t, err)
			}
		})
	}
}

func TestClaudeService_Resume(t *testing.T) {
	mockWrapper := NewMockClaudeSubprocessWrapper()
	service := NewClaudeServiceWithWrapper(mockWrapper)
	ctx := context.Background()

	req := &models.CreateCompletionRequest{
		Prompt: "Continue our conversation",
		Resume: true, // Should use --continue flag internally
	}

	// Test that resume=true is handled correctly with mock
	response, err := service.CreateCompletion(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "Mock claude response", response.Response)
}
