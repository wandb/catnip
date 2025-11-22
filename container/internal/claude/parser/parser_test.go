package parser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSessionFileReader(t *testing.T) {
	reader := NewSessionFileReader("testdata/minimal.jsonl")

	if reader == nil {
		t.Fatal("NewSessionFileReader returned nil")
	}

	if reader.filePath != "testdata/minimal.jsonl" {
		t.Errorf("Expected filePath to be 'testdata/minimal.jsonl', got %s", reader.filePath)
	}

	if reader.statsAgg == nil {
		t.Error("Expected statsAgg to be initialized")
	}

	if reader.subAgents == nil {
		t.Error("Expected subAgents map to be initialized")
	}

	if reader.userMessageMap == nil {
		t.Error("Expected userMessageMap to be initialized")
	}
}

func TestReadIncremental_Minimal(t *testing.T) {
	reader := NewSessionFileReader("testdata/minimal.jsonl")

	messages, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Type != "user" {
		t.Errorf("Expected message type 'user', got %s", messages[0].Type)
	}

	// Second read should return nothing (no changes)
	messages2, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("Second ReadIncremental failed: %v", err)
	}

	if len(messages2) != 0 {
		t.Errorf("Expected 0 messages on second read, got %d", len(messages2))
	}
}

func TestReadIncremental_NoFile(t *testing.T) {
	reader := NewSessionFileReader("testdata/nonexistent.jsonl")

	messages, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("Expected no error for nonexistent file, got: %v", err)
	}

	if messages != nil {
		t.Errorf("Expected nil messages for nonexistent file, got %d messages", len(messages))
	}
}

func TestReadFull(t *testing.T) {
	reader := NewSessionFileReader("testdata/minimal.jsonl")

	err := reader.ReadFull()
	if err != nil {
		t.Fatalf("ReadFull failed: %v", err)
	}

	stats := reader.GetStats()
	if stats.TotalMessages != 1 {
		t.Errorf("Expected 1 total message, got %d", stats.TotalMessages)
	}

	if stats.UserMessages != 1 {
		t.Errorf("Expected 1 user message, got %d", stats.UserMessages)
	}
}

func TestGetTodos_Single(t *testing.T) {
	reader := NewSessionFileReader("testdata/todos_single.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	todos := reader.GetTodos()
	if len(todos) != 3 {
		t.Fatalf("Expected 3 todos, got %d", len(todos))
	}

	// Verify todo contents
	expectedContents := []string{"Implement feature X", "Write tests", "Update documentation"}
	for i, todo := range todos {
		if todo.Content != expectedContents[i] {
			t.Errorf("Todo %d: expected content %q, got %q", i, expectedContents[i], todo.Content)
		}
		if todo.Status != "pending" {
			t.Errorf("Todo %d: expected status 'pending', got %q", i, todo.Status)
		}
	}
}

func TestGetTodos_Multiple(t *testing.T) {
	reader := NewSessionFileReader("testdata/todos_multiple.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	todos := reader.GetTodos()
	if len(todos) != 2 {
		t.Fatalf("Expected 2 todos, got %d", len(todos))
	}

	// The latest TodoWrite should have Task 1 completed and Task 2 in_progress
	if todos[0].Content != "Task 1" {
		t.Errorf("Expected first todo to be 'Task 1', got %q", todos[0].Content)
	}
	if todos[0].Status != "completed" {
		t.Errorf("Expected first todo status 'completed', got %q", todos[0].Status)
	}

	if todos[1].Content != "Task 2" {
		t.Errorf("Expected second todo to be 'Task 2', got %q", todos[1].Content)
	}
	if todos[1].Status != "in_progress" {
		t.Errorf("Expected second todo status 'in_progress', got %q", todos[1].Status)
	}
}

func TestGetLatestMessage(t *testing.T) {
	reader := NewSessionFileReader("testdata/todos_single.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	latestMsg := reader.GetLatestMessage()
	if latestMsg == nil {
		t.Fatal("Expected latest message, got nil")
	}

	if latestMsg.Type != "assistant" {
		t.Errorf("Expected latest message type 'assistant', got %s", latestMsg.Type)
	}
}

func TestGetLatestMessage_FilteringAutomated(t *testing.T) {
	reader := NewSessionFileReader("testdata/automated_prompts.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	latestMsg := reader.GetLatestMessage()
	if latestMsg == nil {
		t.Fatal("Expected latest message, got nil")
	}

	// The automated branch naming response should be filtered out
	// So the latest message should be "You're welcome!" (msg-auto-assistant-003)
	if latestMsg.Uuid != "msg-auto-assistant-003" {
		t.Errorf("Expected latest message UUID 'msg-auto-assistant-003', got %s", latestMsg.Uuid)
	}

	textContent := ExtractTextContent(*latestMsg)
	if textContent != "You're welcome!" {
		t.Errorf("Expected latest message text 'You're welcome!', got %q", textContent)
	}
}

func TestGetLatestMessage_FilteringWarmup(t *testing.T) {
	reader := NewSessionFileReader("testdata/warmup.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	latestMsg := reader.GetLatestMessage()
	// Warmup messages should be filtered, so no latest message
	if latestMsg != nil {
		t.Errorf("Expected no latest message (warmup filtered), got message with UUID %s", latestMsg.Uuid)
	}
}

func TestGetStats_TokenCounts(t *testing.T) {
	reader := NewSessionFileReader("testdata/thinking.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	stats := reader.GetStats()

	if stats.TotalMessages != 2 {
		t.Errorf("Expected 2 total messages, got %d", stats.TotalMessages)
	}

	if stats.UserMessages != 1 {
		t.Errorf("Expected 1 user message, got %d", stats.UserMessages)
	}

	if stats.AssistantMessages != 1 {
		t.Errorf("Expected 1 assistant message, got %d", stats.AssistantMessages)
	}

	if stats.TotalInputTokens != 100 {
		t.Errorf("Expected 100 input tokens, got %d", stats.TotalInputTokens)
	}

	if stats.TotalOutputTokens != 200 {
		t.Errorf("Expected 200 output tokens, got %d", stats.TotalOutputTokens)
	}

	if stats.APICallCount != 1 {
		t.Errorf("Expected 1 API call, got %d", stats.APICallCount)
	}
}

func TestGetStats_ToolCalls(t *testing.T) {
	reader := NewSessionFileReader("testdata/tool_calls.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	stats := reader.GetStats()

	if stats.ToolCallCount != 2 {
		t.Errorf("Expected 2 tool calls, got %d", stats.ToolCallCount)
	}

	if stats.ActiveToolNames["Read"] != 1 {
		t.Errorf("Expected 1 Read tool call, got %d", stats.ActiveToolNames["Read"])
	}

	if stats.ActiveToolNames["Bash"] != 1 {
		t.Errorf("Expected 1 Bash tool call, got %d", stats.ActiveToolNames["Bash"])
	}
}

func TestGetStats_SessionDuration(t *testing.T) {
	reader := NewSessionFileReader("testdata/todos_multiple.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	stats := reader.GetStats()

	// First message at 10:00:00, last at 10:00:03 = 3 second duration
	expectedDuration := 3 * time.Second
	if stats.SessionDuration != expectedDuration {
		t.Errorf("Expected session duration %v, got %v", expectedDuration, stats.SessionDuration)
	}

	if stats.FirstMessageTime.IsZero() {
		t.Error("Expected FirstMessageTime to be set")
	}

	if stats.LastMessageTime.IsZero() {
		t.Error("Expected LastMessageTime to be set")
	}
}

func TestGetThinkingOverview(t *testing.T) {
	reader := NewSessionFileReader("testdata/thinking.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	thinking := reader.GetThinkingOverview()
	if len(thinking) != 1 {
		t.Fatalf("Expected 1 thinking block, got %d", len(thinking))
	}

	if thinking[0].Content == "" {
		t.Error("Expected thinking content to be non-empty")
	}

	if len(thinking[0].Content) < 20 {
		t.Error("Expected thinking content to be at least 20 characters")
	} else if thinking[0].Content[:20] != "Let me think about t" {
		t.Errorf("Unexpected thinking content start: %s", thinking[0].Content[:20])
	}

	if thinking[0].MessageID != "msg_assistant_think_001" {
		t.Errorf("Expected message ID 'msg_assistant_think_001', got %s", thinking[0].MessageID)
	}
}

func TestGetSubAgents(t *testing.T) {
	reader := NewSessionFileReader("testdata/warmup.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	subAgents := reader.GetSubAgents()
	if len(subAgents) != 1 {
		t.Fatalf("Expected 1 sub-agent, got %d", len(subAgents))
	}

	if subAgents[0].AgentID != "67734906" {
		t.Errorf("Expected agent ID '67734906', got %s", subAgents[0].AgentID)
	}

	if subAgents[0].MessageCount != 2 {
		t.Errorf("Expected 2 messages from agent, got %d", subAgents[0].MessageCount)
	}

	if subAgents[0].SessionID != "test-session-warmup" {
		t.Errorf("Expected session ID 'test-session-warmup', got %s", subAgents[0].SessionID)
	}
}

func TestIncrementalParsing(t *testing.T) {
	// Create a temporary file for this test
	tmpFile := filepath.Join(t.TempDir(), "incremental.jsonl")

	// Write first message
	line1 := `{"type":"user","message":{"role":"user","content":"Message 1"},"uuid":"msg-001","timestamp":"2025-11-21T10:00:00.000Z"}` + "\n"
	err := os.WriteFile(tmpFile, []byte(line1), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	reader := NewSessionFileReader(tmpFile)

	// First read
	messages, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("First ReadIncremental failed: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message on first read, got %d", len(messages))
	}

	stats := reader.GetStats()
	if stats.TotalMessages != 1 {
		t.Errorf("Expected 1 total message, got %d", stats.TotalMessages)
	}

	// Append second message
	time.Sleep(10 * time.Millisecond) // Ensure modification time changes
	line2 := `{"type":"user","message":{"role":"user","content":"Message 2"},"uuid":"msg-002","timestamp":"2025-11-21T10:00:01.000Z"}` + "\n"
	f, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("Failed to open temp file for append: %v", err)
	}
	_, err = f.WriteString(line2)
	f.Close()
	if err != nil {
		t.Fatalf("Failed to append to temp file: %v", err)
	}

	// Second read should only get the new message
	messages, err = reader.ReadIncremental()
	if err != nil {
		t.Fatalf("Second ReadIncremental failed: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 new message on second read, got %d", len(messages))
	}

	if messages[0].Uuid != "msg-002" {
		t.Errorf("Expected new message UUID 'msg-002', got %s", messages[0].Uuid)
	}

	stats = reader.GetStats()
	if stats.TotalMessages != 2 {
		t.Errorf("Expected 2 total messages after second read, got %d", stats.TotalMessages)
	}
}

func TestFileTruncation(t *testing.T) {
	// Create a temporary file for this test
	tmpFile := filepath.Join(t.TempDir(), "truncate.jsonl")

	// Write multiple messages
	content := `{"type":"user","message":{"role":"user","content":"Message 1"},"uuid":"msg-001","timestamp":"2025-11-21T10:00:00.000Z"}
{"type":"user","message":{"role":"user","content":"Message 2"},"uuid":"msg-002","timestamp":"2025-11-21T10:00:01.000Z"}
`
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	reader := NewSessionFileReader(tmpFile)

	// First read
	messages, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("First ReadIncremental failed: %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages on first read, got %d", len(messages))
	}

	// Truncate file (simulate file being replaced)
	time.Sleep(10 * time.Millisecond)
	newContent := `{"type":"user","message":{"role":"user","content":"New Message"},"uuid":"msg-new","timestamp":"2025-11-21T10:00:02.000Z"}
`
	err = os.WriteFile(tmpFile, []byte(newContent), 0644)
	if err != nil {
		t.Fatalf("Failed to truncate temp file: %v", err)
	}

	// Second read should detect truncation and reset
	messages, err = reader.ReadIncremental()
	if err != nil {
		t.Fatalf("Second ReadIncremental failed: %v", err)
	}

	// Should read the new content
	if len(messages) < 1 {
		t.Errorf("Expected at least 1 message after truncation, got %d", len(messages))
	}

	stats := reader.GetStats()
	// After reset, should have only the new message
	if stats.TotalMessages != 1 {
		t.Errorf("Expected 1 total message after truncation reset, got %d", stats.TotalMessages)
	}
}

func TestConcurrentAccess(t *testing.T) {
	reader := NewSessionFileReader("testdata/minimal.jsonl")

	// Read once to initialize
	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("Initial ReadIncremental failed: %v", err)
	}

	// Concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_ = reader.GetTodos()
			_ = reader.GetLatestMessage()
			_ = reader.GetStats()
			_ = reader.GetThinkingOverview()
			_ = reader.GetSubAgents()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without panicking, concurrent access is safe
}

func TestReset(t *testing.T) {
	reader := NewSessionFileReader("testdata/todos_single.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	// Verify we have data
	if len(reader.GetTodos()) == 0 {
		t.Error("Expected todos before reset")
	}

	if reader.GetLatestMessage() == nil {
		t.Error("Expected latest message before reset")
	}

	// Reset (note: Reset requires lock, so we call ReadFull which resets)
	err = reader.ReadFull()
	if err != nil {
		t.Fatalf("ReadFull failed: %v", err)
	}

	// Data should still be there after ReadFull (it re-reads)
	if len(reader.GetTodos()) == 0 {
		t.Error("Expected todos after ReadFull")
	}
}

func TestGetFilePath(t *testing.T) {
	expectedPath := "testdata/minimal.jsonl"
	reader := NewSessionFileReader(expectedPath)

	if reader.GetFilePath() != expectedPath {
		t.Errorf("Expected file path %s, got %s", expectedPath, reader.GetFilePath())
	}
}

func TestGetLastModTime(t *testing.T) {
	reader := NewSessionFileReader("testdata/minimal.jsonl")

	// Before reading, mod time should be zero
	if !reader.GetLastModTime().IsZero() {
		t.Error("Expected zero mod time before reading")
	}

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	// After reading, mod time should be set
	if reader.GetLastModTime().IsZero() {
		t.Error("Expected non-zero mod time after reading")
	}
}
