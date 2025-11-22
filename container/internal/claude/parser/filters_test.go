package parser

import (
	"testing"

	"github.com/vanpelt/catnip/internal/models"
)

func TestIsAutomatedPrompt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Warmup", "Warmup", true},
		{"Branch naming", "Generate a git branch name that: fixes the bug", true},
		{"Session title", "Based on this coding session title: implement feature", true},
		{"PR generation", "Generate a pull request title and description that: adds new API", true},
		{"Commit message", "Create a commit message that: updates documentation", true},
		{"Regular user message", "Can you help me debug this?", false},
		{"Empty string", "", false},
		{"Partial match", "I want to generate something", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAutomatedPrompt(tt.input)
			if result != tt.expected {
				t.Errorf("IsAutomatedPrompt(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsWarmupMessage(t *testing.T) {
	tests := []struct {
		name       string
		msg        models.ClaudeSessionMessage
		userMsgMap map[string]string
		expected   bool
	}{
		{
			name: "Warmup user message",
			msg: models.ClaudeSessionMessage{
				Type:        "user",
				IsSidechain: true,
				Message: map[string]any{
					"content": "Warmup",
				},
			},
			userMsgMap: make(map[string]string),
			expected:   true,
		},
		{
			name: "Warmup assistant message with warmup parent",
			msg: models.ClaudeSessionMessage{
				Type:        "assistant",
				IsSidechain: true,
				ParentUuid:  "parent-001",
			},
			userMsgMap: map[string]string{
				"parent-001": "Warmup",
			},
			expected: true,
		},
		{
			name: "Non-warmup sidechain message",
			msg: models.ClaudeSessionMessage{
				Type:        "assistant",
				IsSidechain: true,
				ParentUuid:  "parent-001",
			},
			userMsgMap: map[string]string{
				"parent-001": "Regular message",
			},
			expected: false,
		},
		{
			name: "Non-sidechain message",
			msg: models.ClaudeSessionMessage{
				Type:        "user",
				IsSidechain: false,
				Message: map[string]any{
					"content": "Warmup",
				},
			},
			userMsgMap: make(map[string]string),
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWarmupMessage(tt.msg, tt.userMsgMap)
			if result != tt.expected {
				t.Errorf("IsWarmupMessage() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestShouldSkipMessage(t *testing.T) {
	tests := []struct {
		name       string
		msg        models.ClaudeSessionMessage
		filter     MessageFilter
		userMsgMap map[string]string
		expected   bool
	}{
		{
			name: "Skip warmup when filter enabled",
			msg: models.ClaudeSessionMessage{
				Type:        "user",
				IsSidechain: true,
				Message:     map[string]any{"content": "Warmup"},
			},
			filter:     MessageFilter{SkipWarmup: true},
			userMsgMap: make(map[string]string),
			expected:   true,
		},
		{
			name: "Don't skip warmup when filter disabled",
			msg: models.ClaudeSessionMessage{
				Type:        "user",
				IsSidechain: true,
				Message:     map[string]any{"content": "Warmup"},
			},
			filter:     MessageFilter{SkipWarmup: false},
			userMsgMap: make(map[string]string),
			expected:   false,
		},
		{
			name: "Skip automated prompt response",
			msg: models.ClaudeSessionMessage{
				Type:       "assistant",
				ParentUuid: "parent-001",
			},
			filter: MessageFilter{SkipAutomated: true},
			userMsgMap: map[string]string{
				"parent-001": "Generate a git branch name that: fix bug",
			},
			expected: true,
		},
		{
			name: "Skip error messages when filter enabled",
			msg: models.ClaudeSessionMessage{
				Type: "error",
			},
			filter:     MessageFilter{SkipErrors: true},
			userMsgMap: make(map[string]string),
			expected:   true,
		},
		{
			name: "Filter by type - match",
			msg: models.ClaudeSessionMessage{
				Type: "user",
			},
			filter:     MessageFilter{OnlyType: "user"},
			userMsgMap: make(map[string]string),
			expected:   false, // Should NOT skip
		},
		{
			name: "Filter by type - no match",
			msg: models.ClaudeSessionMessage{
				Type: "assistant",
			},
			filter:     MessageFilter{OnlyType: "user"},
			userMsgMap: make(map[string]string),
			expected:   true, // Should skip
		},
		{
			name: "No filters - don't skip",
			msg: models.ClaudeSessionMessage{
				Type: "user",
			},
			filter:     MessageFilter{},
			userMsgMap: make(map[string]string),
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldSkipMessage(tt.msg, tt.filter, tt.userMsgMap)
			if result != tt.expected {
				t.Errorf("ShouldSkipMessage() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestExtractToolCalls(t *testing.T) {
	tests := []struct {
		name          string
		msg           models.ClaudeSessionMessage
		expectedCount int
		expectedNames []string
	}{
		{
			name: "Single tool call",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "tool_use",
							"id":   "toolu_001",
							"name": "Read",
							"input": map[string]any{
								"file_path": "/test.txt",
							},
						},
					},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"Read"},
		},
		{
			name: "Multiple tool calls",
			msg: models.ClaudeSessionMessage{
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
					},
				},
			},
			expectedCount: 2,
			expectedNames: []string{"Read", "Bash"},
		},
		{
			name: "Mixed content types",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "Some text",
						},
						map[string]any{
							"type": "tool_use",
							"name": "TodoWrite",
						},
					},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"TodoWrite"},
		},
		{
			name: "No tool calls",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "Just text",
						},
					},
				},
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
		{
			name:          "Nil message",
			msg:           models.ClaudeSessionMessage{},
			expectedCount: 0,
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCalls := ExtractToolCalls(tt.msg)

			if len(toolCalls) != tt.expectedCount {
				t.Errorf("Expected %d tool calls, got %d", tt.expectedCount, len(toolCalls))
			}

			for i, expectedName := range tt.expectedNames {
				if i >= len(toolCalls) {
					t.Errorf("Missing tool call %d", i)
					continue
				}
				if toolCalls[i].Name != expectedName {
					t.Errorf("Tool call %d: expected name %s, got %s", i, expectedName, toolCalls[i].Name)
				}
			}
		})
	}
}

func TestExtractThinking(t *testing.T) {
	tests := []struct {
		name          string
		msg           models.ClaudeSessionMessage
		expectedCount int
	}{
		{
			name: "Single thinking block",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type":     "thinking",
							"thinking": "Let me think about this...",
						},
					},
					"id": "msg_001",
				},
				Timestamp: "2025-11-21T10:00:00.000Z",
			},
			expectedCount: 1,
		},
		{
			name: "Multiple thinking blocks",
			msg: models.ClaudeSessionMessage{
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
					"id": "msg_002",
				},
				Timestamp: "2025-11-21T10:00:00.000Z",
			},
			expectedCount: 2,
		},
		{
			name: "No thinking blocks",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "Just text",
						},
					},
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thinking := ExtractThinking(tt.msg)

			if len(thinking) != tt.expectedCount {
				t.Errorf("Expected %d thinking blocks, got %d", tt.expectedCount, len(thinking))
			}

			for i, block := range thinking {
				if block.Content == "" {
					t.Errorf("Thinking block %d has empty content", i)
				}
			}
		})
	}
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		name     string
		msg      models.ClaudeSessionMessage
		expected string
	}{
		{
			name: "String content",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": "Simple text message",
				},
			},
			expected: "Simple text message",
		},
		{
			name: "Array with single text block",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "Text from array",
						},
					},
				},
			},
			expected: "Text from array",
		},
		{
			name: "Array with multiple text blocks",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "First part",
						},
						map[string]any{
							"type": "text",
							"text": "Second part",
						},
					},
				},
			},
			expected: "First part\nSecond part",
		},
		{
			name: "Array with mixed content types",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "Text part",
						},
						map[string]any{
							"type": "tool_use",
							"name": "SomeTool",
						},
						map[string]any{
							"type": "text",
							"text": "More text",
						},
					},
				},
			},
			expected: "Text part\nMore text",
		},
		{
			name:     "Nil message",
			msg:      models.ClaudeSessionMessage{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTextContent(tt.msg)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractTodos(t *testing.T) {
	tests := []struct {
		name          string
		msg           models.ClaudeSessionMessage
		expectedCount int
	}{
		{
			name: "TodoWrite with multiple todos",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "tool_use",
							"name": "TodoWrite",
							"input": map[string]any{
								"todos": []any{
									map[string]any{
										"content":    "Task 1",
										"status":     "pending",
										"activeForm": "Doing task 1",
									},
									map[string]any{
										"content":    "Task 2",
										"status":     "in_progress",
										"activeForm": "Doing task 2",
										"priority":   "high",
									},
								},
							},
						},
					},
				},
			},
			expectedCount: 2,
		},
		{
			name: "Non-TodoWrite tool",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "tool_use",
							"name": "Read",
							"input": map[string]any{
								"file_path": "/test.txt",
							},
						},
					},
				},
			},
			expectedCount: 0,
		},
		{
			name: "Empty todos array",
			msg: models.ClaudeSessionMessage{
				Message: map[string]any{
					"content": []any{
						map[string]any{
							"type": "tool_use",
							"name": "TodoWrite",
							"input": map[string]any{
								"todos": []any{},
							},
						},
					},
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			todos := ExtractTodos(tt.msg)

			if len(todos) != tt.expectedCount {
				t.Errorf("Expected %d todos, got %d", tt.expectedCount, len(todos))
			}

			// Verify todo fields are populated
			for i, todo := range todos {
				if todo.Content == "" {
					t.Errorf("Todo %d has empty content", i)
				}
				if todo.Status == "" {
					t.Errorf("Todo %d has empty status", i)
				}
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		wantZero  bool
	}{
		{
			name:      "Valid RFC3339 timestamp",
			timestamp: "2025-11-21T10:00:00.000Z",
			wantZero:  false,
		},
		{
			name:      "Invalid timestamp",
			timestamp: "not-a-timestamp",
			wantZero:  true,
		},
		{
			name:      "Empty timestamp",
			timestamp: "",
			wantZero:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimestamp(tt.timestamp)
			if tt.wantZero && !result.IsZero() {
				t.Errorf("Expected zero time for %q, got %v", tt.timestamp, result)
			}
			if !tt.wantZero && result.IsZero() {
				t.Errorf("Expected non-zero time for %q, got zero", tt.timestamp)
			}
		})
	}
}
