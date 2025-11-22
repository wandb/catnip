package parser

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// SessionFileReader reads and parses Claude session files incrementally
type SessionFileReader struct {
	filePath    string
	lastOffset  int64
	lastModTime time.Time

	// Cached state (updated incrementally)
	todos          []models.Todo
	latestMessage  *models.ClaudeSessionMessage
	statsAgg       *StatsAggregator
	thinking       []ThinkingBlock
	subAgents      map[string]*SubAgentInfo
	userMessageMap map[string]string // For automated prompt detection

	// Thread safety
	mu sync.RWMutex
}

// NewSessionFileReader creates a new session file reader for the given file path
func NewSessionFileReader(filePath string) *SessionFileReader {
	return &SessionFileReader{
		filePath:       filePath,
		statsAgg:       NewStatsAggregator(),
		subAgents:      make(map[string]*SubAgentInfo),
		userMessageMap: make(map[string]string),
	}
}

// ReadIncremental reads new content from the file since the last read
// Returns the new messages that were added since the last read
func (r *SessionFileReader) ReadIncremental() ([]models.ClaudeSessionMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if file exists and get modification time
	info, err := os.Stat(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist yet
		}
		return nil, err
	}

	// Check if file has been modified since last read
	if !info.ModTime().After(r.lastModTime) && r.lastOffset > 0 {
		return nil, nil // No changes
	}

	// Open file
	file, err := os.Open(r.filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// If file size is smaller than last offset, file was truncated - reset
	if info.Size() < r.lastOffset {
		r.lastOffset = 0
		r.Reset()
	}

	// Seek to last read position
	if r.lastOffset > 0 {
		if _, err := file.Seek(r.lastOffset, 0); err != nil {
			return nil, err
		}
	}

	// Read and parse new messages
	decoder := json.NewDecoder(file)
	var newMessages []models.ClaudeSessionMessage

	for {
		var msg models.ClaudeSessionMessage
		if err := decoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			// Skip invalid JSON lines - just continue to next line
			continue
		}

		// Process the message to update cached state
		r.processMessage(&msg)
		newMessages = append(newMessages, msg)
	}

	// Update position tracking
	newOffset, err := file.Seek(0, io.SeekCurrent)
	if err == nil {
		r.lastOffset = newOffset
	}
	r.lastModTime = info.ModTime()

	return newMessages, nil
}

// ReadFull reads the entire file from the beginning, resetting all cached state
func (r *SessionFileReader) ReadFull() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Reset state
	r.lastOffset = 0
	r.Reset()

	// Open file
	file, err := os.Open(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet
		}
		return err
	}
	defer file.Close()

	// Get file info
	info, err := os.Stat(r.filePath)
	if err != nil {
		return err
	}

	// Read and parse all messages
	decoder := json.NewDecoder(file)

	for {
		var msg models.ClaudeSessionMessage
		if err := decoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			// Skip invalid JSON lines
			continue
		}

		// Process the message
		r.processMessage(&msg)
	}

	// Update position tracking
	newOffset, err := file.Seek(0, io.SeekCurrent)
	if err == nil {
		r.lastOffset = newOffset
	}
	r.lastModTime = info.ModTime()

	return nil
}

// processMessage updates the cached state based on a message
func (r *SessionFileReader) processMessage(msg *models.ClaudeSessionMessage) {
	// Update user message map for filtering
	if msg.Type == "user" && msg.Message != nil {
		if content, exists := msg.Message["content"]; exists {
			if contentStr, ok := content.(string); ok {
				r.userMessageMap[msg.Uuid] = contentStr
			}
		}
	}

	// Update todos
	todos := ExtractTodos(*msg)
	if len(todos) > 0 {
		r.todos = todos
	}

	// Update latest message (if not filtered)
	if !ShouldSkipMessage(*msg, DefaultFilter, r.userMessageMap) {
		// Make a copy of the message
		msgCopy := *msg
		r.latestMessage = &msgCopy
	}

	// Update statistics
	r.statsAgg.ProcessMessage(*msg)

	// Extract and store thinking blocks (keep last 10)
	thinkingBlocks := ExtractThinking(*msg)
	if len(thinkingBlocks) > 0 {
		r.thinking = append(r.thinking, thinkingBlocks...)
		// Keep only last 10 thinking blocks
		if len(r.thinking) > 10 {
			r.thinking = r.thinking[len(r.thinking)-10:]
		}
	}

	// Track sub-agents
	if msg.IsSidechain && msg.AgentID != "" {
		timestamp := parseTimestamp(msg.Timestamp)
		if agent, exists := r.subAgents[msg.AgentID]; exists {
			agent.MessageCount++
			agent.LastSeen = timestamp
		} else {
			r.subAgents[msg.AgentID] = &SubAgentInfo{
				AgentID:      msg.AgentID,
				SessionID:    msg.SessionId,
				MessageCount: 1,
				FirstSeen:    timestamp,
				LastSeen:     timestamp,
			}
		}
	}

	// Update sub-agent count in stats
	r.statsAgg.SetSubAgentCount(len(r.subAgents))
}

// GetTodos returns the current list of todos
func (r *SessionFileReader) GetTodos() []models.Todo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	todosCopy := make([]models.Todo, len(r.todos))
	copy(todosCopy, r.todos)
	return todosCopy
}

// GetLatestMessage returns the latest message that passes the default filter
func (r *SessionFileReader) GetLatestMessage() *models.ClaudeSessionMessage {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.latestMessage == nil {
		return nil
	}

	// Return a copy to prevent external modification
	msgCopy := *r.latestMessage
	return &msgCopy
}

// GetStats returns the current session statistics
func (r *SessionFileReader) GetStats() SessionStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.statsAgg.GetStats()
}

// GetThinkingOverview returns recent thinking blocks
func (r *SessionFileReader) GetThinkingOverview() []ThinkingBlock {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	thinkingCopy := make([]ThinkingBlock, len(r.thinking))
	copy(thinkingCopy, r.thinking)
	return thinkingCopy
}

// GetSubAgents returns information about all sub-agents
func (r *SessionFileReader) GetSubAgents() []*SubAgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Convert map to slice
	subAgents := make([]*SubAgentInfo, 0, len(r.subAgents))
	for _, agent := range r.subAgents {
		// Make a copy
		agentCopy := *agent
		subAgents = append(subAgents, &agentCopy)
	}

	return subAgents
}

// Reset clears all cached state (caller must hold lock)
func (r *SessionFileReader) Reset() {
	r.todos = nil
	r.latestMessage = nil
	r.statsAgg.Reset()
	r.thinking = nil
	r.subAgents = make(map[string]*SubAgentInfo)
	r.userMessageMap = make(map[string]string)
}

// GetFilePath returns the file path being monitored
func (r *SessionFileReader) GetFilePath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.filePath
}

// GetLastModTime returns the last modification time of the file
func (r *SessionFileReader) GetLastModTime() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastModTime
}
