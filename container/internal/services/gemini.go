package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// GeminiService manages Gemini CLI session metadata.
type GeminiService struct {
	geminiHistoryDir string
}

// NewGeminiService creates a new Gemini service.
func NewGeminiService() *GeminiService {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// On failure, we can't do much, so we'll have an empty service.
		return &GeminiService{}
	}
	return &GeminiService{
		geminiHistoryDir: filepath.Join(homeDir, ".gemini", "tmp"),
	}
}

// GetAllWorktreeSessionSummaries gets session summaries for all Gemini sessions.
func (s *GeminiService) GetAllWorktreeSessionSummaries() (map[string]*models.GeminiSessionSummary, error) {
	summaries := make(map[string]*models.GeminiSessionSummary)

	if s.geminiHistoryDir == "" {
		return summaries, nil
	}

	sessionDirs, err := os.ReadDir(s.geminiHistoryDir)
	if err != nil {
		// If the directory doesn't exist, just return empty summaries.
		if os.IsNotExist(err) {
			return summaries, nil
		}
		return nil, fmt.Errorf("failed to read gemini history directory: %w", err)
	}

	for _, dir := range sessionDirs {
		if !dir.IsDir() {
			continue
		}

		sessionHash := dir.Name()
		logPath := filepath.Join(s.geminiHistoryDir, sessionHash, "logs.json")

		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			continue
		}

		logs, err := s.readLogs(logPath)
		if err != nil {
			// Log error but continue with other sessions
			fmt.Printf("Error reading gemini log %s: %v\n", logPath, err)
			continue
		}

		if len(logs) == 0 {
			continue
		}

		// Sort logs by timestamp to be safe
		sort.Slice(logs, func(i, j int) bool {
			return logs[i].Timestamp < logs[j].Timestamp
		})

		lastLog := logs[len(logs)-1]
		lastUpdated, _ := time.Parse(time.RFC3339Nano, lastLog.Timestamp)

		summary := &models.GeminiSessionSummary{
			UUID:        lastLog.SessionID,
			Title:       s.generateTitle(logs),
			TurnCount:   len(logs),
			LastUpdated: lastUpdated,
			Worktree:    sessionHash, // Using hash as placeholder for worktree
		}
		summaries[sessionHash] = summary
	}

	return summaries, nil
}

// GetSessionByUUID gets a single Gemini session by its UUID.
func (s *GeminiService) GetSessionByUUID(sessionUUID string) (*models.GeminiSessionSummary, error) {
	summaries, err := s.GetAllWorktreeSessionSummaries()
	if err != nil {
		return nil, err
	}

	for _, summary := range summaries {
		if summary.UUID == sessionUUID {
			return summary, nil
		}
	}

	return nil, fmt.Errorf("gemini session not found")
}

// GetSessionMessages gets all messages for a given Gemini session.
func (s *GeminiService) GetSessionMessages(sessionUUID string) ([]models.GeminiSessionMessage, error) {
	sessionDir, err := s.findSessionDir(sessionUUID)
	if err != nil {
		return nil, err
	}

	logPath := filepath.Join(s.geminiHistoryDir, sessionDir, "logs.json")
	logs, err := s.readLogs(logPath)
	if err != nil {
		return nil, err
	}

	var messages []models.GeminiSessionMessage
	for _, log := range logs {
		ts, _ := time.Parse(time.RFC3339Nano, log.Timestamp)
		messages = append(messages, models.GeminiSessionMessage{
			Role:      log.Type, // Assuming "user" maps to a role
			Content:   log.Message,
			Timestamp: ts,
		})
	}

	return messages, nil
}

// findSessionDir scans the history directory to find the directory corresponding to a session UUID.
func (s *GeminiService) findSessionDir(sessionUUID string) (string, error) {
	if s.geminiHistoryDir == "" {
		return "", fmt.Errorf("gemini history directory not configured")
	}

	sessionDirs, err := os.ReadDir(s.geminiHistoryDir)
	if err != nil {
		return "", err
	}

	for _, dir := range sessionDirs {
		if !dir.IsDir() {
			continue
		}

		sessionHash := dir.Name()
		logPath := filepath.Join(s.geminiHistoryDir, sessionHash, "logs.json")

		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			continue
		}

		// Quick check of first line to see if it's the right session
		// This is an optimization to avoid parsing the whole file every time.
		f, err := os.Open(logPath)
		if err != nil {
			continue
		}
		defer f.Close()

		var logs []models.GeminiLogEntry
		// We only need to check the first entry to find the session ID
		if err := json.NewDecoder(f).Decode(&logs); err != nil || len(logs) == 0 {
			continue
		}

		if logs[0].SessionID == sessionUUID {
			return sessionHash, nil
		}
	}

	return "", fmt.Errorf("session directory not found for uuid: %s", sessionUUID)
}

func (s *GeminiService) readLogs(path string) ([]models.GeminiLogEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var logs []models.GeminiLogEntry
	if err := json.Unmarshal(data, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *GeminiService) generateTitle(logs []models.GeminiLogEntry) string {
	for _, log := range logs {
		if log.Type == "user" && log.Message != "" {
			// Return the first user message as the title
			return log.Message
		}
	}
	return "Untitled Gemini Session"
}
