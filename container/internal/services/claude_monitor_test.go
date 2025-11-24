package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vanpelt/catnip/internal/claude/parser"
)

// TestLastMessageNotReplacedByEmptyContent tests that tool-only messages
// don't replace text messages as the "latest message" displayed to users
func TestLastMessageNotReplacedByEmptyContent(t *testing.T) {
	// Create a temporary test session file
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "test-session.jsonl")

	// Write test messages:
	// 1. User message
	// 2. Assistant message with text (should be latest)
	// 3. Assistant message with only tool_use (should NOT replace #2)
	messages := []string{
		`{"type":"user","uuid":"user-1","timestamp":"2024-01-01T10:00:00Z","message":{"content":"Hello"}}`,
		`{"type":"assistant","uuid":"asst-1","timestamp":"2024-01-01T10:00:01Z","message":{"content":[{"type":"text","text":"I'll help you with that!"}]}}`,
		`{"type":"assistant","uuid":"asst-2","timestamp":"2024-01-01T10:00:02Z","message":{"content":[{"type":"tool_use","id":"tool1","name":"Read","input":{}}]}}`,
	}

	if err := os.WriteFile(sessionFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Test incremental processing
	reader := parser.NewSessionFileReader(sessionFile)

	for i, msg := range messages {
		// Append message to file
		f, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(msg + "\n"); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()

		// Update file mod time to trigger incremental read
		time.Sleep(10 * time.Millisecond)
		if err := os.Chtimes(sessionFile, time.Now(), time.Now()); err != nil {
			t.Fatalf("Failed to update file times: %v", err)
		}

		// Read incrementally
		if _, err := reader.ReadIncremental(); err != nil {
			t.Fatalf("Failed to read incremental at message %d: %v", i, err)
		}

		latestMsg := reader.GetLatestMessage()

		switch i {
		case 0:
			// After user message, no assistant message yet
			if latestMsg != nil {
				t.Errorf("After message %d: Expected nil latest message, got %v", i, latestMsg.Uuid)
			}
		case 1:
			// After first assistant message with text
			if latestMsg == nil {
				t.Fatalf("After message %d: Expected latest message, got nil", i)
			}
			textContent := parser.ExtractTextContent(*latestMsg)
			if textContent == "" {
				t.Errorf("After message %d: Expected non-empty text content, got empty", i)
			}
			if latestMsg.Uuid != "asst-1" {
				t.Errorf("After message %d: Expected uuid=asst-1, got %s", i, latestMsg.Uuid)
			}
		case 2:
			// THIS IS THE CRITICAL TEST:
			// After tool-only message, latest should STILL be the text message from step 1
			// Currently this FAILS because parser updates latestMessage to asst-2
			if latestMsg == nil {
				t.Fatalf("After message %d: Expected latest message, got nil", i)
			}

			textContent := parser.ExtractTextContent(*latestMsg)

			// BUG: Currently latestMsg.Uuid == "asst-2" and textContent == ""
			// EXPECTED: latestMsg.Uuid should be "asst-1" (the previous text message)
			// OR textContent should never be empty for display purposes

			if textContent == "" {
				t.Errorf("After message %d: Latest message has empty text content (UUID: %s). "+
					"Tool-only messages should not replace text messages as 'latest message' for display.",
					i, latestMsg.Uuid)
			}
		}
	}
}
