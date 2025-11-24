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

func TestGetSubAgents_TaskAgents(t *testing.T) {
	reader := NewSessionFileReader("testdata/task_agents.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	subAgents := reader.GetSubAgents()
	if len(subAgents) != 2 {
		t.Fatalf("Expected 2 sub-agents from Task tool calls, got %d", len(subAgents))
	}

	// Check first Task agent (Explore)
	found := false
	for _, agent := range subAgents {
		if agent.SubagentType == "Explore" && agent.Description == "Find bugs in codebase" {
			found = true
			if agent.AgentID != "toolu_task_001" {
				t.Errorf("Expected agent ID 'toolu_task_001', got %s", agent.AgentID)
			}
			break
		}
	}
	if !found {
		t.Error("Expected to find Explore sub-agent")
	}

	// Check second Task agent (code-reviewer)
	found = false
	for _, agent := range subAgents {
		if agent.SubagentType == "superpowers:code-reviewer" && agent.Description == "Review pending changes" {
			found = true
			if agent.AgentID != "toolu_task_002" {
				t.Errorf("Expected agent ID 'toolu_task_002', got %s", agent.AgentID)
			}
			break
		}
	}
	if !found {
		t.Error("Expected to find superpowers:code-reviewer sub-agent")
	}

	// Verify stats reflect the count
	stats := reader.GetStats()
	if stats.SubAgentCount != 2 {
		t.Errorf("Expected SubAgentCount to be 2, got %d", stats.SubAgentCount)
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

func TestThinkingOnlyMessage(t *testing.T) {
	// Create a temporary file with a thinking-only assistant message
	tmpFile := filepath.Join(t.TempDir(), "thinking_only.jsonl")

	content := `{"type":"user","message":{"role":"user","content":"What is 2+2?"},"uuid":"msg-001","timestamp":"2025-11-21T10:00:00.000Z","sessionId":"session-123"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Let me calculate 2+2..."}]},"uuid":"msg-002","timestamp":"2025-11-21T10:00:01.000Z","sessionId":"session-123"}
`
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	reader := NewSessionFileReader(tmpFile)

	messages, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}

	// GetLatestMessage should NOT return thinking-only messages (they have no text content)
	// Now that we have GetLatestThought(), LatestMessage should only track TEXT messages
	latestMsg := reader.GetLatestMessage()
	if latestMsg != nil {
		t.Errorf("Expected nil latest message for thinking-only session, got UUID %s", latestMsg.Uuid)
	}

	// GetLatestThought SHOULD return the thinking-only assistant message
	latestThought := reader.GetLatestThought()
	if latestThought == nil {
		t.Fatal("Expected non-nil latest thought")
	}

	// Verify it's the assistant message with thinking
	if latestThought.Type != "assistant" {
		t.Errorf("Expected latest thought type 'assistant', got '%s'", latestThought.Type)
	}

	// Verify it has no text content but has thinking
	textContent := ExtractTextContent(*latestThought)
	if textContent != "" {
		t.Errorf("Expected empty text content for thinking-only message, got '%s'", textContent)
	}

	thinkingBlocks := ExtractThinking(*latestThought)
	if len(thinkingBlocks) == 0 {
		t.Error("Expected thinking blocks in latest thought")
	}

	// Verify stats show the assistant message was counted
	stats := reader.GetStats()
	if stats.AssistantMessages != 1 {
		t.Errorf("Expected 1 assistant message in stats, got %d", stats.AssistantMessages)
	}
	if stats.ThinkingBlockCount != 1 {
		t.Errorf("Expected 1 thinking block in stats, got %d", stats.ThinkingBlockCount)
	}
}

func TestGetAllMessages_NoFilter(t *testing.T) {
	reader := NewSessionFileReader("testdata/automated_prompts.jsonl")

	messages, err := reader.GetAllMessages(MessageFilter{})
	if err != nil {
		t.Fatalf("GetAllMessages failed: %v", err)
	}

	// Without filter, should get all messages
	if len(messages) != 6 {
		t.Errorf("Expected 6 messages without filter, got %d", len(messages))
	}

	// Verify order is chronological
	if messages[0].Uuid != "msg-auto-user-001" {
		t.Errorf("Expected first message UUID 'msg-auto-user-001', got %s", messages[0].Uuid)
	}
	if messages[len(messages)-1].Uuid != "msg-auto-assistant-003" {
		t.Errorf("Expected last message UUID 'msg-auto-assistant-003', got %s", messages[len(messages)-1].Uuid)
	}
}

func TestGetAllMessages_DefaultFilter(t *testing.T) {
	reader := NewSessionFileReader("testdata/automated_prompts.jsonl")

	messages, err := reader.GetAllMessages(DefaultFilter)
	if err != nil {
		t.Fatalf("GetAllMessages failed: %v", err)
	}

	// With DefaultFilter, should skip assistant responses to automated prompts
	// Note: The automated user prompts themselves are NOT filtered, only assistant responses
	// Expected: user-001, assistant-001, user-002 (automated), user-003, assistant-003 = 5 messages
	if len(messages) != 5 {
		t.Errorf("Expected 5 messages with DefaultFilter, got %d", len(messages))
	}

	// Verify the automated assistant response is filtered out
	for _, msg := range messages {
		if msg.Uuid == "msg-auto-assistant-002" {
			t.Errorf("Unexpected message %s in filtered results (should be filtered)", msg.Uuid)
		}
	}

	// Verify the automated user prompt IS present (only responses are filtered)
	foundAutomatedUserPrompt := false
	for _, msg := range messages {
		if msg.Uuid == "msg-auto-user-002" {
			foundAutomatedUserPrompt = true
			break
		}
	}
	if !foundAutomatedUserPrompt {
		t.Error("Expected automated user prompt msg-auto-user-002 to be present (only responses are filtered)")
	}
}

func TestGetAllMessages_OnlyAssistant(t *testing.T) {
	reader := NewSessionFileReader("testdata/automated_prompts.jsonl")

	filter := MessageFilter{
		OnlyType: "assistant",
	}
	messages, err := reader.GetAllMessages(filter)
	if err != nil {
		t.Fatalf("GetAllMessages failed: %v", err)
	}

	// Should only get assistant messages
	if len(messages) != 3 {
		t.Errorf("Expected 3 assistant messages, got %d", len(messages))
	}

	for _, msg := range messages {
		if msg.Type != "assistant" {
			t.Errorf("Expected only assistant messages, got type %s", msg.Type)
		}
	}
}

func TestGetAllMessages_WithWarmup(t *testing.T) {
	reader := NewSessionFileReader("testdata/warmup.jsonl")

	// Without warmup filter
	messages, err := reader.GetAllMessages(MessageFilter{})
	if err != nil {
		t.Fatalf("GetAllMessages failed: %v", err)
	}
	totalMessages := len(messages)

	// With warmup filter
	messagesFiltered, err := reader.GetAllMessages(MessageFilter{SkipWarmup: true})
	if err != nil {
		t.Fatalf("GetAllMessages failed: %v", err)
	}

	// Should have fewer messages when filtering warmup
	if len(messagesFiltered) >= totalMessages {
		t.Errorf("Expected fewer messages with warmup filter, got %d vs %d", len(messagesFiltered), totalMessages)
	}
}

func TestGetAllMessages_NonexistentFile(t *testing.T) {
	reader := NewSessionFileReader("testdata/nonexistent.jsonl")

	messages, err := reader.GetAllMessages(DefaultFilter)
	if err != nil {
		t.Fatalf("Expected no error for nonexistent file, got: %v", err)
	}

	if messages != nil {
		t.Errorf("Expected nil messages for nonexistent file, got %d messages", len(messages))
	}
}

func TestGetLatestMessage_CyrillicLoki(t *testing.T) {
	reader := NewSessionFileReader("testdata/cyrillic_loki.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	latestMsg := reader.GetLatestMessage()
	if latestMsg == nil {
		t.Fatal("Expected latest assistant message from cyrillic_loki session, got nil")
	}

	// GetLatestMessage should return the latest assistant message
	// (not user messages or summary messages)
	if latestMsg.Type != "assistant" {
		t.Errorf("Expected latest message type 'assistant', got %s (uuid: %s)", latestMsg.Type, latestMsg.Uuid)
	}

	// Verify it has a UUID (summary messages don't have UUIDs)
	if latestMsg.Uuid == "" {
		t.Error("Expected latest message to have a UUID")
	}
}

func TestGetLatestThought(t *testing.T) {
	reader := NewSessionFileReader("testdata/thinking.jsonl")

	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	latestThought := reader.GetLatestThought()
	if latestThought == nil {
		t.Fatal("Expected latest thought message from thinking session, got nil")
	}

	// Verify it's an assistant message
	if latestThought.Type != "assistant" {
		t.Errorf("Expected latest thought type 'assistant', got %s", latestThought.Type)
	}

	// Verify it has thinking blocks
	thinkingBlocks := ExtractThinking(*latestThought)
	if len(thinkingBlocks) == 0 {
		t.Error("Expected latest thought to have thinking blocks")
	}

	// Verify it's the correct message (msg_assistant_think_001)
	if latestThought.Message == nil {
		t.Fatal("Expected message to have Message map")
	}
	if id, exists := latestThought.Message["id"]; exists {
		if idStr, ok := id.(string); ok {
			if idStr != "msg_assistant_think_001" {
				t.Errorf("Expected latest thought message ID 'msg_assistant_think_001', got %s", idStr)
			}
		}
	}
}

func TestIncrementalReplay_LatestMessageSkipsToolOnly(t *testing.T) {
	// This test simulates reading a session file incrementally (like during an active session)
	// and verifies that tool-only messages don't replace text messages as "latest"

	// Read line by line to simulate incremental updates
	lines := []string{
		// Line 1-2: User + Assistant with text
		`{"parentUuid":null,"isSidechain":false,"userType":"external","cwd":"/workspace","sessionId":"test-incremental","version":"2.0.45","type":"user","message":{"role":"user","content":"Help me analyze this code"},"uuid":"user-001","timestamp":"2025-11-21T10:00:00.000Z"}`,
		`{"parentUuid":"user-001","isSidechain":false,"userType":"external","cwd":"/workspace","sessionId":"test-incremental","version":"2.0.45","type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll analyze the code for you. Let me read it first."}],"id":"asst-001"},"uuid":"asst-001","timestamp":"2025-11-21T10:00:01.000Z"}`,
	}

	// Create temp file with first 2 lines
	tmpFile := filepath.Join(t.TempDir(), "replay.jsonl")
	if err := os.WriteFile(tmpFile, []byte(lines[0]+"\n"+lines[1]+"\n"), 0600); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	reader := NewSessionFileReader(tmpFile)
	_, err := reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed: %v", err)
	}

	// After 2 lines, latest message should be asst-001 with text
	latestMsg := reader.GetLatestMessage()
	if latestMsg == nil {
		t.Fatal("Expected latest message after 2 lines")
	}
	if latestMsg.Uuid != "asst-001" {
		t.Errorf("After 2 lines, expected latest UUID 'asst-001', got %s", latestMsg.Uuid)
	}
	textContent := ExtractTextContent(*latestMsg)
	if textContent == "" {
		t.Error("After 2 lines, expected non-empty text content")
	}

	// Add line 3: Assistant with ONLY tool_use (no text)
	toolOnlyLine := `{"parentUuid":"asst-001","isSidechain":false,"userType":"external","cwd":"/workspace","sessionId":"test-incremental","version":"2.0.45","type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_read_001","name":"Read","input":{"file_path":"/workspace/code.js"}}],"id":"asst-002"},"uuid":"asst-002","timestamp":"2025-11-21T10:00:02.000Z"}` + "\n"
	f, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	if _, err := f.WriteString(toolOnlyLine); err != nil {
		f.Close()
		t.Fatalf("Failed to append line: %v", err)
	}
	f.Close()

	// Read incrementally again
	_, err = reader.ReadIncremental()
	if err != nil {
		t.Fatalf("ReadIncremental failed on line 3: %v", err)
	}

	// After tool-only message, latest should STILL be asst-001 (with text)
	latestMsg = reader.GetLatestMessage()
	if latestMsg == nil {
		t.Fatal("Expected latest message after tool-only message")
	}
	if latestMsg.Uuid != "asst-001" {
		t.Errorf("After tool-only message, expected latest UUID 'asst-001', got %s", latestMsg.Uuid)
	}
	textContent = ExtractTextContent(*latestMsg)
	if textContent == "" {
		t.Error("After tool-only message, expected latest to still have text content")
	}
	if !contains(textContent, "I'll analyze the code") {
		t.Errorf("After tool-only message, expected text 'I'll analyze the code...', got %q", textContent)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
