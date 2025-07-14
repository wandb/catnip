package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// AnthropicService provides a client for the Anthropic API
type AnthropicService struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// AnthropicMessage represents a message in the Anthropic API format
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicRequest represents a request to the Anthropic API
type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
}

// AnthropicResponse represents a response from the Anthropic API
type AnthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// AnthropicError represents an error response from the Anthropic API
type AnthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// NewAnthropicService creates a new Anthropic API service
func NewAnthropicService() *AnthropicService {
	// Try to get API key from environment variables in order of preference
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("CLAUDE_API_KEY")
	}
	
	return &AnthropicService{
		apiKey:  apiKey,
		baseURL: "https://api.anthropic.com/v1",
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// IsConfigured returns true if the service has a valid API key
func (s *AnthropicService) IsConfigured() bool {
	return s.apiKey != ""
}

// CreateMessage sends a message to the Anthropic API and returns the response
func (s *AnthropicService) CreateMessage(model string, messages []AnthropicMessage, maxTokens int) (*AnthropicResponse, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("Anthropic API key not configured. Set ANTHROPIC_API_KEY or CLAUDE_API_KEY environment variable")
	}

	if maxTokens <= 0 {
		maxTokens = 4000 // Default max tokens
	}

	request := AnthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", s.baseURL+"/messages", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiError AnthropicError
		if err := json.Unmarshal(body, &apiError); err != nil {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("API error: %s", apiError.Message)
	}

	var response AnthropicResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// SimpleMessage is a convenience method for sending a single message
func (s *AnthropicService) SimpleMessage(model, userMessage string, maxTokens int) (*AnthropicResponse, error) {
	messages := []AnthropicMessage{
		{
			Role:    "user",
			Content: userMessage,
		},
	}

	return s.CreateMessage(model, messages, maxTokens)
}

// GetDefaultModel returns the default Claude model to use
func (s *AnthropicService) GetDefaultModel() string {
	return "claude-3-5-sonnet-20241022"
}