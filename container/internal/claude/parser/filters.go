package parser

import (
	"strings"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// parseTimestamp parses a timestamp string into a time.Time
func parseTimestamp(timestamp string) time.Time {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

// IsAutomatedPrompt checks if a user message is one of the known automated prompts
// that should be filtered out from the "latest message" display
func IsAutomatedPrompt(userMessage string) bool {
	// Known automated prompt patterns we send
	automatedMarkers := []string{
		"Warmup",                                              // Agent warmup messages
		"Generate a git branch name that:",                    // Branch renaming
		"Based on this coding session title:",                 // Branch renaming alternative
		"Generate a pull request title and description that:", // PR generation
		"Create a commit message that:",                       // Commit message generation
	}

	for _, marker := range automatedMarkers {
		if strings.Contains(userMessage, marker) {
			return true
		}
	}

	return false
}

// IsWarmupMessage checks if a message is a warmup message that should be skipped
func IsWarmupMessage(msg models.ClaudeSessionMessage, userMsgMap map[string]string) bool {
	// Skip warmup-related sidechain messages
	if msg.IsSidechain {
		// For sidechain messages, check if they're warmup-related
		if msg.Type == "assistant" && msg.ParentUuid != "" {
			if parentContent, exists := userMsgMap[msg.ParentUuid]; exists {
				// Only skip if parent is "Warmup" prompt
				if parentContent == "Warmup" {
					return true
				}
			}
		}
		// For sidechain user messages, skip if it's "Warmup"
		if msg.Type == "user" && msg.Message != nil {
			if content, exists := msg.Message["content"]; exists {
				if contentStr, ok := content.(string); ok && contentStr == "Warmup" {
					return true
				}
			}
		}
	}

	return false
}

// ShouldSkipMessage determines if a message should be filtered based on the provided filter
func ShouldSkipMessage(msg models.ClaudeSessionMessage, filter MessageFilter, userMsgMap map[string]string) bool {
	// Check warmup filter
	if filter.SkipWarmup && IsWarmupMessage(msg, userMsgMap) {
		return true
	}

	// Check automated prompt filter
	if filter.SkipAutomated {
		// Skip assistant messages that are responses to automated prompts
		if msg.Type == "assistant" && msg.ParentUuid != "" {
			if parentContent, exists := userMsgMap[msg.ParentUuid]; exists {
				if IsAutomatedPrompt(parentContent) {
					return true
				}
			}
		}
	}

	// Check sidechain filter (but not warmup, which is handled above)
	if filter.SkipSidechain && msg.IsSidechain && !IsWarmupMessage(msg, userMsgMap) {
		return true
	}

	// Check error filter
	if filter.SkipErrors && msg.Type == "error" {
		return true
	}

	// Check type filter
	if filter.OnlyType != "" && msg.Type != filter.OnlyType {
		return true
	}

	// Check content type filter
	if filter.OnlyContentType != "" {
		if msg.Message != nil {
			if content, exists := msg.Message["content"]; exists {
				if contentArray, ok := content.([]interface{}); ok {
					hasMatchingContentType := false
					for _, contentItem := range contentArray {
						if contentMap, ok := contentItem.(map[string]interface{}); ok {
							if contentType, exists := contentMap["type"]; exists {
								if contentType == filter.OnlyContentType {
									hasMatchingContentType = true
									break
								}
							}
						}
					}
					if !hasMatchingContentType {
						return true
					}
				}
			}
		}
	}

	return false
}

// ExtractToolCalls extracts all tool_use blocks from a message
func ExtractToolCalls(msg models.ClaudeSessionMessage) []ToolUseBlock {
	var toolCalls []ToolUseBlock

	if msg.Message == nil {
		return toolCalls
	}

	content, exists := msg.Message["content"]
	if !exists {
		return toolCalls
	}

	contentArray, ok := content.([]interface{})
	if !ok {
		return toolCalls
	}

	for _, contentItem := range contentArray {
		contentMap, ok := contentItem.(map[string]interface{})
		if !ok {
			continue
		}

		contentType, exists := contentMap["type"]
		if !exists || contentType != "tool_use" {
			continue
		}

		toolUse := ToolUseBlock{
			Type: "tool_use",
		}

		if id, exists := contentMap["id"]; exists {
			if idStr, ok := id.(string); ok {
				toolUse.ID = idStr
			}
		}

		if name, exists := contentMap["name"]; exists {
			if nameStr, ok := name.(string); ok {
				toolUse.Name = nameStr
			}
		}

		if input, exists := contentMap["input"]; exists {
			if inputMap, ok := input.(map[string]interface{}); ok {
				toolUse.Input = inputMap
			}
		}

		toolCalls = append(toolCalls, toolUse)
	}

	return toolCalls
}

// ExtractThinking extracts thinking blocks from a message
func ExtractThinking(msg models.ClaudeSessionMessage) []ThinkingBlock {
	var thinkingBlocks []ThinkingBlock

	if msg.Message == nil {
		return thinkingBlocks
	}

	content, exists := msg.Message["content"]
	if !exists {
		return thinkingBlocks
	}

	contentArray, ok := content.([]interface{})
	if !ok {
		return thinkingBlocks
	}

	// Parse timestamp
	timestamp := parseTimestamp(msg.Timestamp)

	// Get message ID
	messageID := ""
	if id, exists := msg.Message["id"]; exists {
		if idStr, ok := id.(string); ok {
			messageID = idStr
		}
	}

	for _, contentItem := range contentArray {
		contentMap, ok := contentItem.(map[string]interface{})
		if !ok {
			continue
		}

		contentType, exists := contentMap["type"]
		if !exists || contentType != "thinking" {
			continue
		}

		thinkingBlock := ThinkingBlock{
			Timestamp: timestamp,
			MessageID: messageID,
		}

		if thinking, exists := contentMap["thinking"]; exists {
			if thinkingStr, ok := thinking.(string); ok {
				thinkingBlock.Content = thinkingStr
			}
		}

		thinkingBlocks = append(thinkingBlocks, thinkingBlock)
	}

	return thinkingBlocks
}

// ExtractTextContent extracts all text content from a message
func ExtractTextContent(msg models.ClaudeSessionMessage) string {
	if msg.Message == nil {
		return ""
	}

	content, exists := msg.Message["content"]
	if !exists {
		return ""
	}

	// Handle string content
	if contentStr, ok := content.(string); ok {
		return contentStr
	}

	// Handle array content
	contentArray, ok := content.([]interface{})
	if !ok {
		return ""
	}

	var textParts []string
	for _, contentItem := range contentArray {
		contentMap, ok := contentItem.(map[string]interface{})
		if !ok {
			continue
		}

		contentType, exists := contentMap["type"]
		if !exists {
			continue
		}

		// Extract text from text blocks
		if contentType == "text" {
			if text, exists := contentMap["text"]; exists {
				if textStr, ok := text.(string); ok {
					textParts = append(textParts, textStr)
				}
			}
		}
	}

	return strings.Join(textParts, "\n")
}

// ExtractTodos extracts todos from a TodoWrite tool_use message
func ExtractTodos(msg models.ClaudeSessionMessage) []models.Todo {
	var todos []models.Todo

	toolCalls := ExtractToolCalls(msg)
	for _, toolCall := range toolCalls {
		if toolCall.Name != "TodoWrite" {
			continue
		}

		if todosData, exists := toolCall.Input["todos"]; exists {
			if todosArray, ok := todosData.([]interface{}); ok {
				for _, todoItem := range todosArray {
					if todoMap, ok := todoItem.(map[string]interface{}); ok {
						todo := models.Todo{}

						if id, exists := todoMap["id"]; exists {
							if idStr, ok := id.(string); ok {
								todo.ID = idStr
							}
						}

						if content, exists := todoMap["content"]; exists {
							if contentStr, ok := content.(string); ok {
								todo.Content = contentStr
							}
						}

						if status, exists := todoMap["status"]; exists {
							if statusStr, ok := status.(string); ok {
								todo.Status = statusStr
							}
						}

						if priority, exists := todoMap["priority"]; exists {
							if priorityStr, ok := priority.(string); ok {
								todo.Priority = priorityStr
							}
						}

						todos = append(todos, todo)
					}
				}
			}
		}
	}

	return todos
}
