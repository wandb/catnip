package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClaudeSubprocessWrapper(t *testing.T) {
	wrapper := NewClaudeSubprocessWrapper()
	require.NotNil(t, wrapper)
	assert.Equal(t, "claude", wrapper.claudePath)
}

func TestClaudeSubprocessOptions(t *testing.T) {
	opts := &ClaudeSubprocessOptions{
		Prompt:           "Test prompt",
		SystemPrompt:     "You are a test assistant",
		Model:            "claude-3-5-sonnet-20241022",
		MaxTurns:         5,
		WorkingDirectory: "/test/workspace",
		Resume:           true,
	}

	assert.Equal(t, "Test prompt", opts.Prompt)
	assert.Equal(t, "You are a test assistant", opts.SystemPrompt)
	assert.Equal(t, "claude-3-5-sonnet-20241022", opts.Model)
	assert.Equal(t, 5, opts.MaxTurns)
	assert.Equal(t, "/test/workspace", opts.WorkingDirectory)
	assert.True(t, opts.Resume)
}

func TestClaudeSubprocessWrapper_Interface(t *testing.T) {
	wrapper := NewClaudeSubprocessWrapper()

	// Test that the wrapper implements the interface
	var _ ClaudeSubprocessInterface = wrapper

	assert.NotNil(t, wrapper)
}

func TestMockClaudeSubprocessWrapper(t *testing.T) {
	mockWrapper := NewMockClaudeSubprocessWrapper()
	ctx := context.Background()

	// Test that mock implements the interface
	var _ ClaudeSubprocessInterface = mockWrapper

	t.Run("successful completion", func(t *testing.T) {
		opts := &ClaudeSubprocessOptions{
			Prompt: "test prompt",
		}

		response, err := mockWrapper.CreateCompletion(ctx, opts)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "Mock claude response", response.Response)
		assert.False(t, response.IsChunk)
		assert.True(t, response.IsLast)
	})

	t.Run("failure simulation", func(t *testing.T) {
		mockWrapper.ShouldFail = true
		mockWrapper.FailureError = "mock error"

		opts := &ClaudeSubprocessOptions{
			Prompt: "test prompt",
		}

		response, err := mockWrapper.CreateCompletion(ctx, opts)
		assert.Error(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, "mock error", response.Error)
		assert.Contains(t, err.Error(), "mock error")
	})
}
