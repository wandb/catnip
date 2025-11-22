package parser

import (
	"testing"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

func TestNewStatsAggregator(t *testing.T) {
	agg := NewStatsAggregator()

	if agg == nil {
		t.Fatal("NewStatsAggregator returned nil")
	}

	if agg.stats == nil {
		t.Fatal("Expected stats to be initialized")
	}

	if agg.stats.ActiveToolNames == nil {
		t.Fatal("Expected ActiveToolNames map to be initialized")
	}
}

func TestProcessMessage_MessageCounting(t *testing.T) {
	agg := NewStatsAggregator()

	// Process user message
	userMsg := models.ClaudeSessionMessage{
		Type:      "user",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": "Hello",
		},
	}
	agg.ProcessMessage(userMsg)

	stats := agg.GetStats()
	if stats.TotalMessages != 1 {
		t.Errorf("Expected 1 total message, got %d", stats.TotalMessages)
	}
	if stats.UserMessages != 1 {
		t.Errorf("Expected 1 user message, got %d", stats.UserMessages)
	}
	if stats.AssistantMessages != 0 {
		t.Errorf("Expected 0 assistant messages, got %d", stats.AssistantMessages)
	}

	// Process assistant message
	assistantMsg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:01.000Z",
		Message: map[string]any{
			"content": "Hi there",
		},
	}
	agg.ProcessMessage(assistantMsg)

	stats = agg.GetStats()
	if stats.TotalMessages != 2 {
		t.Errorf("Expected 2 total messages, got %d", stats.TotalMessages)
	}
	if stats.UserMessages != 1 {
		t.Errorf("Expected 1 user message, got %d", stats.UserMessages)
	}
	if stats.AssistantMessages != 1 {
		t.Errorf("Expected 1 assistant message, got %d", stats.AssistantMessages)
	}
}

func TestProcessMessage_TokenCounting(t *testing.T) {
	agg := NewStatsAggregator()

	msg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": "Response",
			"usage": map[string]any{
				"input_tokens":                100.0,
				"output_tokens":               50.0,
				"cache_read_input_tokens":     200.0,
				"cache_creation_input_tokens": 150.0,
			},
		},
	}
	agg.ProcessMessage(msg)

	stats := agg.GetStats()
	if stats.TotalInputTokens != 100 {
		t.Errorf("Expected 100 input tokens, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 50 {
		t.Errorf("Expected 50 output tokens, got %d", stats.TotalOutputTokens)
	}
	if stats.CacheReadTokens != 200 {
		t.Errorf("Expected 200 cache read tokens, got %d", stats.CacheReadTokens)
	}
	if stats.CacheCreationTokens != 150 {
		t.Errorf("Expected 150 cache creation tokens, got %d", stats.CacheCreationTokens)
	}
}

func TestProcessMessage_TokenCountingIntegers(t *testing.T) {
	agg := NewStatsAggregator()

	msg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": "Response",
			"usage": map[string]any{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		},
	}
	agg.ProcessMessage(msg)

	stats := agg.GetStats()
	if stats.TotalInputTokens != 100 {
		t.Errorf("Expected 100 input tokens, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 50 {
		t.Errorf("Expected 50 output tokens, got %d", stats.TotalOutputTokens)
	}
}

func TestProcessMessage_APICallCounting(t *testing.T) {
	agg := NewStatsAggregator()

	// Message with usage = API call
	msgWithUsage := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"usage": map[string]any{
				"input_tokens": 100,
			},
		},
	}
	agg.ProcessMessage(msgWithUsage)

	stats := agg.GetStats()
	if stats.APICallCount != 1 {
		t.Errorf("Expected 1 API call, got %d", stats.APICallCount)
	}

	// Message without usage = not an API call
	msgWithoutUsage := models.ClaudeSessionMessage{
		Type:      "user",
		Timestamp: "2025-11-21T10:00:01.000Z",
		Message: map[string]any{
			"content": "Hello",
		},
	}
	agg.ProcessMessage(msgWithoutUsage)

	stats = agg.GetStats()
	if stats.APICallCount != 1 {
		t.Errorf("Expected still 1 API call, got %d", stats.APICallCount)
	}
}

func TestProcessMessage_ToolCallCounting(t *testing.T) {
	agg := NewStatsAggregator()

	msg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"name": "Read",
				},
				map[string]any{
					"type": "tool_use",
					"name": "Bash",
				},
				map[string]any{
					"type": "tool_use",
					"name": "Read", // Duplicate
				},
			},
		},
	}
	agg.ProcessMessage(msg)

	stats := agg.GetStats()
	if stats.ToolCallCount != 3 {
		t.Errorf("Expected 3 tool calls, got %d", stats.ToolCallCount)
	}

	if stats.ActiveToolNames["Read"] != 2 {
		t.Errorf("Expected 2 Read tool calls, got %d", stats.ActiveToolNames["Read"])
	}

	if stats.ActiveToolNames["Bash"] != 1 {
		t.Errorf("Expected 1 Bash tool call, got %d", stats.ActiveToolNames["Bash"])
	}
}

func TestProcessMessage_ThinkingBlockCounting(t *testing.T) {
	agg := NewStatsAggregator()

	msg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": []any{
				map[string]any{
					"type":     "thinking",
					"thinking": "First thought",
				},
				map[string]any{
					"type":     "thinking",
					"thinking": "Second thought",
				},
			},
		},
	}
	agg.ProcessMessage(msg)

	stats := agg.GetStats()
	if stats.ThinkingBlockCount != 2 {
		t.Errorf("Expected 2 thinking blocks, got %d", stats.ThinkingBlockCount)
	}
}

func TestProcessMessage_SessionDuration(t *testing.T) {
	agg := NewStatsAggregator()

	firstMsg := models.ClaudeSessionMessage{
		Type:      "user",
		Timestamp: "2025-11-21T10:00:00.000Z",
	}
	agg.ProcessMessage(firstMsg)

	stats := agg.GetStats()
	if stats.SessionDuration != 0 {
		t.Errorf("Expected 0 duration with single message, got %v", stats.SessionDuration)
	}

	secondMsg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:05.000Z",
	}
	agg.ProcessMessage(secondMsg)

	stats = agg.GetStats()
	expectedDuration := 5 * time.Second
	if stats.SessionDuration != expectedDuration {
		t.Errorf("Expected duration %v, got %v", expectedDuration, stats.SessionDuration)
	}
}

func TestProcessMessage_FirstAndLastMessageTime(t *testing.T) {
	agg := NewStatsAggregator()

	firstMsg := models.ClaudeSessionMessage{
		Type:      "user",
		Timestamp: "2025-11-21T10:00:00.000Z",
	}
	agg.ProcessMessage(firstMsg)

	stats := agg.GetStats()
	if stats.FirstMessageTime.IsZero() {
		t.Error("Expected FirstMessageTime to be set")
	}
	if stats.LastMessageTime.IsZero() {
		t.Error("Expected LastMessageTime to be set")
	}

	firstTime := stats.FirstMessageTime
	lastTime := stats.LastMessageTime

	secondMsg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:05.000Z",
	}
	agg.ProcessMessage(secondMsg)

	stats = agg.GetStats()
	if !stats.FirstMessageTime.Equal(firstTime) {
		t.Error("FirstMessageTime should not change")
	}
	if stats.LastMessageTime.Equal(lastTime) {
		t.Error("LastMessageTime should have been updated")
	}
}

func TestStatsAggregator_Reset(t *testing.T) {
	agg := NewStatsAggregator()

	// Add some data
	msg := models.ClaudeSessionMessage{
		Type:      "user",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"name": "Read",
				},
			},
		},
	}
	agg.ProcessMessage(msg)

	stats := agg.GetStats()
	if stats.TotalMessages == 0 {
		t.Error("Expected messages before reset")
	}

	// Reset
	agg.Reset()

	stats = agg.GetStats()
	if stats.TotalMessages != 0 {
		t.Errorf("Expected 0 messages after reset, got %d", stats.TotalMessages)
	}
	if stats.ToolCallCount != 0 {
		t.Errorf("Expected 0 tool calls after reset, got %d", stats.ToolCallCount)
	}
	if len(stats.ActiveToolNames) != 0 {
		t.Errorf("Expected empty ActiveToolNames after reset, got %d entries", len(stats.ActiveToolNames))
	}
}

func TestSetSubAgentCount(t *testing.T) {
	agg := NewStatsAggregator()

	agg.SetSubAgentCount(5)

	stats := agg.GetStats()
	if stats.SubAgentCount != 5 {
		t.Errorf("Expected 5 sub-agents, got %d", stats.SubAgentCount)
	}
}

func TestGetStats_ReturnsCopy(t *testing.T) {
	agg := NewStatsAggregator()

	msg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"name": "Read",
				},
			},
		},
	}
	agg.ProcessMessage(msg)

	stats1 := agg.GetStats()
	stats1.ActiveToolNames["Bash"] = 999 // Modify the copy

	stats2 := agg.GetStats()
	if stats2.ActiveToolNames["Bash"] == 999 {
		t.Error("Modifying returned stats should not affect internal state")
	}
}

func TestProcessTokenUsage_MissingFields(t *testing.T) {
	agg := NewStatsAggregator()

	// Message with partial usage data
	msg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"usage": map[string]any{
				"input_tokens": 100.0,
				// output_tokens missing
			},
		},
	}
	agg.ProcessMessage(msg)

	stats := agg.GetStats()
	if stats.TotalInputTokens != 100 {
		t.Errorf("Expected 100 input tokens, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 0 {
		t.Errorf("Expected 0 output tokens (missing), got %d", stats.TotalOutputTokens)
	}
}

func TestProcessMessage_MultipleMessages(t *testing.T) {
	agg := NewStatsAggregator()

	// Process multiple messages and verify cumulative stats
	for i := 0; i < 10; i++ {
		msg := models.ClaudeSessionMessage{
			Type:      "assistant",
			Timestamp: "2025-11-21T10:00:00.000Z",
			Message: map[string]any{
				"usage": map[string]any{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			},
		}
		agg.ProcessMessage(msg)
	}

	stats := agg.GetStats()
	if stats.TotalMessages != 10 {
		t.Errorf("Expected 10 messages, got %d", stats.TotalMessages)
	}
	if stats.TotalInputTokens != 100 {
		t.Errorf("Expected 100 total input tokens, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 50 {
		t.Errorf("Expected 50 total output tokens, got %d", stats.TotalOutputTokens)
	}
	if stats.APICallCount != 10 {
		t.Errorf("Expected 10 API calls, got %d", stats.APICallCount)
	}
}
