package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// ClaudeSubprocessInterface defines the interface for claude CLI subprocess execution
type ClaudeSubprocessInterface interface {
	CreateCompletion(ctx context.Context, opts *ClaudeSubprocessOptions) (*models.CreateCompletionResponse, error)
	CreateStreamingCompletion(ctx context.Context, opts *ClaudeSubprocessOptions, responseWriter io.Writer) error
}

// ClaudeSubprocessWrapper handles calling the claude CLI tool as a subprocess
type ClaudeSubprocessWrapper struct {
	claudePath string
}

// NewClaudeSubprocessWrapper creates a new subprocess wrapper
func NewClaudeSubprocessWrapper() *ClaudeSubprocessWrapper {
	// Default to "claude" in PATH, but could be configurable
	claudePath := "claude"

	// Check if claude is available in PATH
	if _, err := exec.LookPath(claudePath); err != nil {
		// Could fall back to specific paths or return error
		claudePath = "claude" // Still try the default
	}

	return &ClaudeSubprocessWrapper{
		claudePath: claudePath,
	}
}

// ClaudeSubprocessOptions represents options for the claude subprocess call
type ClaudeSubprocessOptions struct {
	Prompt           string
	SystemPrompt     string
	Model            string
	MaxTurns         int
	WorkingDirectory string
	Resume           bool
}

// CreateCompletion executes claude CLI and returns the response (always uses streaming internally)
func (w *ClaudeSubprocessWrapper) CreateCompletion(ctx context.Context, opts *ClaudeSubprocessOptions) (*models.CreateCompletionResponse, error) {
	// Always use streaming internally and accumulate the response
	return w.createSyncCompletion(ctx, opts)
}

// CreateStreamingCompletion executes claude CLI with streaming output
func (w *ClaudeSubprocessWrapper) CreateStreamingCompletion(ctx context.Context, opts *ClaudeSubprocessOptions, responseWriter io.Writer) error {
	// Build command arguments
	args := []string{"-p"}

	// Always use stream-json for both input and output
	args = append(args, "--output-format=stream-json")
	args = append(args, "--input-format=stream-json")
	args = append(args, "--verbose")

	// Add optional parameters
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}

	// Add continue flag if resume is requested
	if opts.Resume {
		args = append(args, "--continue")
	}

	// Note: Prompt is sent via stdin, not as command argument

	// Create the command
	cmd := exec.CommandContext(ctx, w.claudePath, args...)

	// Set working directory if specified, resolving symlinks
	if opts.WorkingDirectory != "" {
		// Resolve symlinks to get the actual directory path
		resolvedDir, err := filepath.EvalSymlinks(opts.WorkingDirectory)
		if err != nil {
			cmd.Dir = opts.WorkingDirectory
		} else {
			cmd.Dir = resolvedDir
		}
	}

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start claude command: %w", err)
	}

	// Send prompt via stdin as JSON
	go func() {
		defer stdin.Close()
		// Create message structure matching claude CLI expected format
		message := map[string]interface{}{
			"type": "user",
			"message": map[string]string{
				"role":    "user",
				"content": opts.Prompt,
			},
		}
		if messageJSON, err := json.Marshal(message); err == nil {
			if _, writeErr := stdin.Write(messageJSON); writeErr != nil {
				log.Printf("[ERROR] Failed to write message to stdin: %v", writeErr)
				return
			}
			if _, writeErr := stdin.Write([]byte("\n")); writeErr != nil {
				log.Printf("[ERROR] Failed to write newline to stdin: %v", writeErr)
				return
			}
		} else {
			log.Printf("[ERROR] Failed to marshal message: %v", err)
		}
	}()

	// Create a scanner to read line by line
	scanner := bufio.NewScanner(stdout)

	// Process output line by line and stream only response lines
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse JSON line
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &jsonData); err != nil {
			continue // Skip invalid JSON lines
		}

		// Look for assistant messages and stream them
		if msgType, ok := jsonData["type"].(string); ok && msgType == "assistant" {
			// Parse and extract just the text content
			var responseText string
			if message, ok := jsonData["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].([]interface{}); ok && len(content) > 0 {
					if textBlock, ok := content[0].(map[string]interface{}); ok {
						if text, ok := textBlock["text"].(string); ok {
							responseText = text
						}
					}
				}
			}

			if responseText == "" {
				responseText = "No text content found in assistant response"
			}

			// Create a clean response chunk with just the text
			response := &models.CreateCompletionResponse{
				Response: responseText,
				IsChunk:  true,
				IsLast:   false,
			}

			// Marshal and stream the clean response
			responseJSON, err := json.Marshal(response)
			if err != nil {
				continue // Skip this chunk if we can't marshal it
			}

			if _, err := responseWriter.Write(append(responseJSON, '\n')); err != nil {
				return fmt.Errorf("failed to write response chunk: %w", err)
			}

			// Flush if possible (for Server-Sent Events)
			if flusher, ok := responseWriter.(interface{ Flush() }); ok {
				flusher.Flush()
			}
		}
	}

	// Read stderr in background to avoid blocking
	var stderrBuffer strings.Builder
	go func() {
		if _, err := io.Copy(&stderrBuffer, stderr); err != nil {
			log.Printf("[WARNING] Failed to read stderr: %v", err)
		}
	}()

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		// Give a moment for stderr goroutine to finish
		time.Sleep(50 * time.Millisecond)

		errorMsg := strings.TrimSpace(stderrBuffer.String())
		if errorMsg == "" {
			errorMsg = err.Error()
		}

		// Log the full error details
		log.Printf("[ERROR] Claude CLI failed - Exit error: %v", err)
		log.Printf("[ERROR] Claude CLI stderr: '%s'", errorMsg)
		log.Printf("[ERROR] Full command was: %s %v", w.claudePath, args)

		// Send error as final chunk
		errorResponse := &models.CreateCompletionResponse{
			Error:   errorMsg,
			IsChunk: true,
			IsLast:  true,
		}

		responseJSON, _ := json.Marshal(errorResponse)
		if _, err := responseWriter.Write(append(responseJSON, '\n')); err != nil {
			log.Printf("[WARNING] Failed to write response: %v", err)
		}

		return fmt.Errorf("claude command failed: %s", errorMsg)
	}

	// Send final "end" chunk
	finalResponse := &models.CreateCompletionResponse{
		IsChunk: true,
		IsLast:  true,
	}

	responseJSON, err := json.Marshal(finalResponse)
	if err == nil {
		if _, err := responseWriter.Write(append(responseJSON, '\n')); err != nil {
			log.Printf("[WARNING] Failed to write response: %v", err)
		}
	}

	return nil
}

// createSyncCompletion executes claude CLI with streaming and accumulates the response
func (w *ClaudeSubprocessWrapper) createSyncCompletion(ctx context.Context, opts *ClaudeSubprocessOptions) (*models.CreateCompletionResponse, error) {
	// Build command arguments
	args := []string{"-p"}

	// Always use stream-json for both input and output
	args = append(args, "--output-format=stream-json")
	args = append(args, "--input-format=stream-json")
	args = append(args, "--verbose")

	// Add optional parameters
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}

	// Add continue flag if resume is requested
	if opts.Resume {
		args = append(args, "--continue")
	}

	// Note: Prompt is sent via stdin, not as command argument

	// Create the command with timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, w.claudePath, args...)

	// Set working directory if specified, resolving symlinks
	if opts.WorkingDirectory != "" {
		// Resolve symlinks to get the actual directory path
		resolvedDir, err := filepath.EvalSymlinks(opts.WorkingDirectory)
		if err != nil {
			cmd.Dir = opts.WorkingDirectory
		} else {
			cmd.Dir = resolvedDir
		}
	}

	// Set environment
	cmd.Env = os.Environ()

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start claude command: %w", err)
	}

	// Send prompt via stdin as JSON synchronously
	message := map[string]interface{}{
		"type": "user",
		"message": map[string]string{
			"role":    "user",
			"content": opts.Prompt,
		},
	}
	messageJSON, err := json.Marshal(message)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal message: %v", err)
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	if _, err := stdin.Write(messageJSON); err != nil {
		return nil, fmt.Errorf("failed to write to stdin: %w", err)
	}
	if _, err := stdin.Write([]byte("\n")); err != nil {
		return nil, fmt.Errorf("failed to write newline to stdin: %w", err)
	}

	// Close stdin immediately to signal EOF
	if err := stdin.Close(); err != nil {
		log.Printf("[WARNING] Failed to close stdin: %v", err)
	}

	// Process streaming response
	scanner := bufio.NewScanner(stdout)

	// Process output line by line and find assistant message
	var assistantLine string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse JSON line to check if it's an assistant message
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &jsonData); err != nil {
			continue // Skip invalid JSON lines
		}

		// Look for assistant messages
		if msgType, ok := jsonData["type"].(string); ok && msgType == "assistant" {
			assistantLine = line
		}

	}

	// Read stderr in background to avoid blocking
	var stderrBuffer strings.Builder
	stderrDone := make(chan bool)
	go func() {
		defer close(stderrDone)
		if _, err := io.Copy(&stderrBuffer, stderr); err != nil {
			log.Printf("[WARNING] Failed to read stderr: %v", err)
		}
	}()

	// Wait for command to complete
	waitErr := cmd.Wait()

	// Wait for stderr to finish
	<-stderrDone

	// If we got an assistant response, don't fail even if exit code is non-zero
	if waitErr != nil && assistantLine == "" {
		errorMsg := strings.TrimSpace(stderrBuffer.String())
		if errorMsg == "" {
			errorMsg = waitErr.Error()
		}

		return &models.CreateCompletionResponse{
			Error: errorMsg,
		}, fmt.Errorf("claude command failed: %s", errorMsg)
	}

	// Check if we found an assistant response
	if assistantLine == "" {
		return &models.CreateCompletionResponse{
			Response: "No assistant response found in Claude output",
			IsChunk:  false,
			IsLast:   true,
		}, nil
	}

	// Parse the assistant line to extract the actual content
	var assistantData map[string]interface{}
	if err := json.Unmarshal([]byte(assistantLine), &assistantData); err != nil {
		return &models.CreateCompletionResponse{
			Response: "Failed to parse assistant response",
			IsChunk:  false,
			IsLast:   true,
		}, nil
	}

	// Extract the text content from message.content[0].text
	var responseText string
	if message, ok := assistantData["message"].(map[string]interface{}); ok {
		if content, ok := message["content"].([]interface{}); ok && len(content) > 0 {
			if textBlock, ok := content[0].(map[string]interface{}); ok {
				if text, ok := textBlock["text"].(string); ok {
					responseText = text
				}
			}
		}
	}

	if responseText == "" {
		responseText = "No text content found in assistant response"
	}

	// Return just the text content
	return &models.CreateCompletionResponse{
		Response: responseText,
		IsChunk:  false,
		IsLast:   true,
	}, nil
}
