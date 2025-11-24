package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vanpelt/catnip/internal/models"
)

// BenchmarkReadIncremental benchmarks incremental reading performance
func BenchmarkReadIncremental(b *testing.B) {
	reader := NewSessionFileReader("testdata/tool_calls.jsonl")

	// Warmup
	_, _ = reader.ReadIncremental()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = reader.ReadIncremental()
	}
}

// BenchmarkReadFull benchmarks full file reading performance
func BenchmarkReadFull(b *testing.B) {
	reader := NewSessionFileReader("testdata/todos_multiple.jsonl")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reader.ReadFull()
	}
}

// BenchmarkReadFullLargeFile benchmarks reading a larger file
func BenchmarkReadFullLargeFile(b *testing.B) {
	// Create a larger test file
	tmpFile := filepath.Join(b.TempDir(), "large.jsonl")
	f, err := os.Create(tmpFile)
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}

	// Write 1000 messages
	for i := 0; i < 1000; i++ {
		_, _ = f.WriteString(`{"type":"user","message":{"role":"user","content":"Test message"},"uuid":"msg-` + string(rune(i)) + `","timestamp":"2025-11-21T10:00:00.000Z"}` + "\n")
	}
	f.Close()

	reader := NewSessionFileReader(tmpFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reader.ReadFull()
	}
}

// BenchmarkGetTodos benchmarks todo retrieval
func BenchmarkGetTodos(b *testing.B) {
	reader := NewSessionFileReader("testdata/todos_single.jsonl")
	_, _ = reader.ReadIncremental()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reader.GetTodos()
	}
}

// BenchmarkGetLatestMessage benchmarks latest message retrieval
func BenchmarkGetLatestMessage(b *testing.B) {
	reader := NewSessionFileReader("testdata/todos_single.jsonl")
	_, _ = reader.ReadIncremental()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reader.GetLatestMessage()
	}
}

// BenchmarkGetStats benchmarks statistics retrieval
func BenchmarkGetStats(b *testing.B) {
	reader := NewSessionFileReader("testdata/tool_calls.jsonl")
	_, _ = reader.ReadIncremental()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reader.GetStats()
	}
}

// BenchmarkIsAutomatedPrompt benchmarks automated prompt detection
func BenchmarkIsAutomatedPrompt(b *testing.B) {
	prompts := []string{
		"Generate a git branch name that: fixes the bug",
		"Can you help me debug this?",
		"Warmup",
		"Create a commit message that: updates docs",
		"Regular message here",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, prompt := range prompts {
			IsAutomatedPrompt(prompt)
		}
	}
}

// BenchmarkExtractToolCalls benchmarks tool call extraction
func BenchmarkExtractToolCalls(b *testing.B) {
	msg := models.ClaudeSessionMessage{
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
				map[string]any{
					"type": "tool_use",
					"id":   "toolu_002",
					"name": "Bash",
					"input": map[string]any{
						"command": "ls -la",
					},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractToolCalls(msg)
	}
}

// BenchmarkExtractThinking benchmarks thinking extraction
func BenchmarkExtractThinking(b *testing.B) {
	msg := models.ClaudeSessionMessage{
		Message: map[string]any{
			"content": []any{
				map[string]any{
					"type":     "thinking",
					"thinking": "Let me think about this problem carefully...",
				},
			},
			"id": "msg_001",
		},
		Timestamp: "2025-11-21T10:00:00.000Z",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractThinking(msg)
	}
}

// BenchmarkExtractTextContent benchmarks text content extraction
func BenchmarkExtractTextContent(b *testing.B) {
	msg := models.ClaudeSessionMessage{
		Message: map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "This is the first part of the response.",
				},
				map[string]any{
					"type": "text",
					"text": "This is the second part.",
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractTextContent(msg)
	}
}

// BenchmarkExtractTodos benchmarks todo extraction
func BenchmarkExtractTodos(b *testing.B) {
	msg := models.ClaudeSessionMessage{
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
							},
						},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractTodos(msg)
	}
}

// BenchmarkStatsAggregatorProcessMessage benchmarks message processing in stats aggregator
func BenchmarkStatsAggregatorProcessMessage(b *testing.B) {
	agg := NewStatsAggregator()
	msg := models.ClaudeSessionMessage{
		Type:      "assistant",
		Timestamp: "2025-11-21T10:00:00.000Z",
		Message: map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "Response",
				},
				map[string]any{
					"type": "tool_use",
					"name": "Read",
				},
			},
			"usage": map[string]any{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agg.ProcessMessage(msg)
	}
}

// BenchmarkShouldSkipMessage benchmarks message filtering
func BenchmarkShouldSkipMessage(b *testing.B) {
	msg := models.ClaudeSessionMessage{
		Type:       "assistant",
		ParentUuid: "parent-001",
	}
	filter := DefaultFilter
	userMsgMap := map[string]string{
		"parent-001": "Generate a git branch name that: fix bug",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ShouldSkipMessage(msg, filter, userMsgMap)
	}
}

// BenchmarkConcurrentReads benchmarks concurrent read performance
func BenchmarkConcurrentReads(b *testing.B) {
	reader := NewSessionFileReader("testdata/todos_single.jsonl")
	_, _ = reader.ReadIncremental()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = reader.GetTodos()
			_ = reader.GetLatestMessage()
			_ = reader.GetStats()
		}
	})
}
