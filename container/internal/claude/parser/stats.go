package parser

import (
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// StatsAggregator aggregates statistics from session messages
type StatsAggregator struct {
	stats             *SessionStats
	lastUserTime      time.Time // Track last user message time for active duration
	lastAssistantTime time.Time // Track last assistant message time for active duration
}

// NewStatsAggregator creates a new statistics aggregator
func NewStatsAggregator() *StatsAggregator {
	return &StatsAggregator{
		stats: &SessionStats{
			ActiveToolNames: make(map[string]int),
		},
	}
}

// ProcessMessage updates statistics based on a message
func (a *StatsAggregator) ProcessMessage(msg models.ClaudeSessionMessage) {
	a.stats.TotalMessages++

	// Count message types
	switch msg.Type {
	case "user":
		a.stats.UserMessages++
		// Count human prompts (user messages with string content, not tool results)
		if msg.Message != nil {
			if content, exists := msg.Message["content"]; exists {
				// Tool results have array content, human prompts have string content
				if _, isString := content.(string); isString {
					a.stats.HumanPromptCount++
				}
			}
		}
	case "assistant":
		a.stats.AssistantMessages++
	case "system":
		// Check for compaction messages
		if msg.Subtype == "compact_boundary" {
			a.stats.CompactionCount++
		}
	}

	// Parse timestamp
	timestamp := parseTimestamp(msg.Timestamp)
	if !timestamp.IsZero() {
		// Update first message time
		if a.stats.FirstMessageTime.IsZero() || timestamp.Before(a.stats.FirstMessageTime) {
			a.stats.FirstMessageTime = timestamp
		}

		// Update last message time
		if timestamp.After(a.stats.LastMessageTime) {
			a.stats.LastMessageTime = timestamp
		}

		// Calculate active duration (time spent working, excluding idle time)
		switch msg.Type {
		case "user":
			// If there was a previous turn (user -> assistant), add that duration
			if !a.lastUserTime.IsZero() && !a.lastAssistantTime.IsZero() {
				turnDuration := a.lastAssistantTime.Sub(a.lastUserTime)
				if turnDuration > 0 {
					a.stats.ActiveDuration += turnDuration
				}
			}
			// Start new turn
			a.lastUserTime = timestamp
		case "assistant":
			// Track the latest assistant response
			a.lastAssistantTime = timestamp
		}
	}

	// Process message content
	if msg.Message != nil {
		// Extract token usage
		if usage, exists := msg.Message["usage"]; exists {
			if usageMap, ok := usage.(map[string]interface{}); ok {
				a.processTokenUsage(usageMap)
			}
		}

		// Count API calls (messages with usage data are API responses)
		if _, hasUsage := msg.Message["usage"]; hasUsage {
			a.stats.APICallCount++
		}

		// Extract and count tool calls
		toolCalls := ExtractToolCalls(msg)
		a.stats.ToolCallCount += len(toolCalls)
		for _, toolCall := range toolCalls {
			a.stats.ActiveToolNames[toolCall.Name]++
		}

		// Count thinking blocks
		thinkingBlocks := ExtractThinking(msg)
		a.stats.ThinkingBlockCount += len(thinkingBlocks)

		// Count images in content blocks
		if content, exists := msg.Message["content"]; exists {
			if contentArray, ok := content.([]interface{}); ok {
				for _, block := range contentArray {
					if blockMap, ok := block.(map[string]interface{}); ok {
						if blockType, _ := blockMap["type"].(string); blockType == "image" {
							a.stats.ImageCount++
						}
					}
				}
			}
		}
	}

	// Update session duration
	if !a.stats.FirstMessageTime.IsZero() && !a.stats.LastMessageTime.IsZero() {
		a.stats.SessionDuration = a.stats.LastMessageTime.Sub(a.stats.FirstMessageTime)
	}
}

// processTokenUsage extracts and aggregates token usage from a usage map
func (a *StatsAggregator) processTokenUsage(usageMap map[string]interface{}) {
	if inputTokens, exists := usageMap["input_tokens"]; exists {
		if tokensFloat, ok := inputTokens.(float64); ok {
			a.stats.TotalInputTokens += int64(tokensFloat)
		} else if tokensInt, ok := inputTokens.(int); ok {
			a.stats.TotalInputTokens += int64(tokensInt)
		}
	}

	if outputTokens, exists := usageMap["output_tokens"]; exists {
		if tokensFloat, ok := outputTokens.(float64); ok {
			a.stats.TotalOutputTokens += int64(tokensFloat)
		} else if tokensInt, ok := outputTokens.(int); ok {
			a.stats.TotalOutputTokens += int64(tokensInt)
		}
	}

	if cacheReadTokens, exists := usageMap["cache_read_input_tokens"]; exists {
		if tokensFloat, ok := cacheReadTokens.(float64); ok {
			cacheRead := int64(tokensFloat)
			a.stats.CacheReadTokens += cacheRead
			// Track last cache_read as context size (non-zero values only)
			if cacheRead > 0 {
				a.stats.LastContextSizeTokens = cacheRead
			}
		} else if tokensInt, ok := cacheReadTokens.(int); ok {
			cacheRead := int64(tokensInt)
			a.stats.CacheReadTokens += cacheRead
			// Track last cache_read as context size (non-zero values only)
			if cacheRead > 0 {
				a.stats.LastContextSizeTokens = cacheRead
			}
		}
	}

	if cacheCreationTokens, exists := usageMap["cache_creation_input_tokens"]; exists {
		if tokensFloat, ok := cacheCreationTokens.(float64); ok {
			a.stats.CacheCreationTokens += int64(tokensFloat)
		} else if tokensInt, ok := cacheCreationTokens.(int); ok {
			a.stats.CacheCreationTokens += int64(tokensInt)
		}
	}
}

// GetStats returns a copy of the current statistics
func (a *StatsAggregator) GetStats() SessionStats {
	// Create a copy of the map to avoid concurrent modification
	toolNamesCopy := make(map[string]int, len(a.stats.ActiveToolNames))
	for k, v := range a.stats.ActiveToolNames {
		toolNamesCopy[k] = v
	}

	// Calculate final active duration (include the last turn if it hasn't been counted)
	activeDuration := a.stats.ActiveDuration
	if !a.lastUserTime.IsZero() && !a.lastAssistantTime.IsZero() {
		// Check if the last turn needs to be added
		// (if lastAssistantTime is after the last calculated turn)
		if a.lastAssistantTime.After(a.lastUserTime) {
			finalTurnDuration := a.lastAssistantTime.Sub(a.lastUserTime)
			if finalTurnDuration > 0 {
				activeDuration += finalTurnDuration
			}
		}
	}

	return SessionStats{
		TotalMessages:         a.stats.TotalMessages,
		UserMessages:          a.stats.UserMessages,
		AssistantMessages:     a.stats.AssistantMessages,
		HumanPromptCount:      a.stats.HumanPromptCount,
		ToolCallCount:         a.stats.ToolCallCount,
		TotalInputTokens:      a.stats.TotalInputTokens,
		TotalOutputTokens:     a.stats.TotalOutputTokens,
		CacheReadTokens:       a.stats.CacheReadTokens,
		CacheCreationTokens:   a.stats.CacheCreationTokens,
		LastContextSizeTokens: a.stats.LastContextSizeTokens,
		APICallCount:          a.stats.APICallCount,
		SessionDuration:       a.stats.SessionDuration,
		ActiveDuration:        activeDuration,
		ThinkingBlockCount:    a.stats.ThinkingBlockCount,
		SubAgentCount:         a.stats.SubAgentCount,
		CompactionCount:       a.stats.CompactionCount,
		ImageCount:            a.stats.ImageCount,
		FirstMessageTime:      a.stats.FirstMessageTime,
		LastMessageTime:       a.stats.LastMessageTime,
		ActiveToolNames:       toolNamesCopy,
	}
}

// Reset clears all statistics
func (a *StatsAggregator) Reset() {
	a.stats = &SessionStats{
		ActiveToolNames: make(map[string]int),
	}
}

// SetSubAgentCount updates the sub-agent count
func (a *StatsAggregator) SetSubAgentCount(count int) {
	a.stats.SubAgentCount = count
}
