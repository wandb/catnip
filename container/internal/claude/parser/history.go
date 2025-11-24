package parser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vanpelt/catnip/internal/models"
)

// HistoryReader reads user prompt history from Claude configuration files
// Supports both legacy ~/.claude.json and new ~/.claude/history.jsonl formats
type HistoryReader struct {
	claudeConfigPath string
	historyJSONLPath string
}

// NewHistoryReader creates a new history reader with paths to config files
func NewHistoryReader(homeDir string) *HistoryReader {
	return &HistoryReader{
		claudeConfigPath: homeDir + "/.claude.json",
		historyJSONLPath: homeDir + "/.claude/history.jsonl",
	}
}

// HistoryEntry represents a single entry in the user prompt history
type HistoryEntry struct {
	Display        string         `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
	Project        string         `json:"project"`
	SessionID      string         `json:"sessionId"`
	Timestamp      int64          `json:"timestamp"`
}

// GetUserPrompts returns all user prompts for a specific project/worktree
// Reads from both ~/.claude/history.jsonl (preferred) and ~/.claude.json (legacy fallback)
func (h *HistoryReader) GetUserPrompts(projectPath string) ([]models.ClaudeHistoryEntry, error) {
	// Try reading from history.jsonl first (new format)
	prompts, err := h.readFromHistoryJSONL(projectPath)
	if err == nil && len(prompts) > 0 {
		return prompts, nil
	}

	// Fallback to reading from .claude.json (legacy format)
	return h.readFromClaudeConfig(projectPath)
}

// GetLatestUserPrompt returns the most recent user prompt for a project
func (h *HistoryReader) GetLatestUserPrompt(projectPath string) (string, error) {
	prompts, err := h.GetUserPrompts(projectPath)
	if err != nil {
		return "", err
	}

	if len(prompts) == 0 {
		return "", nil
	}

	return prompts[len(prompts)-1].Display, nil
}

// readFromHistoryJSONL reads user prompts from ~/.claude/history.jsonl
func (h *HistoryReader) readFromHistoryJSONL(projectPath string) ([]models.ClaudeHistoryEntry, error) {
	file, err := os.Open(h.historyJSONLPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist, return empty
		}
		return nil, fmt.Errorf("failed to open history.jsonl: %w", err)
	}
	defer file.Close()

	var entries []models.ClaudeHistoryEntry
	decoder := json.NewDecoder(file)

	for {
		var entry HistoryEntry
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			// Skip invalid JSON lines
			continue
		}

		// Only include entries for this project
		if entry.Project == projectPath {
			entries = append(entries, models.ClaudeHistoryEntry{
				Display:        entry.Display,
				PastedContents: entry.PastedContents,
			})
		}
	}

	return entries, nil
}

// readFromClaudeConfig reads user prompts from ~/.claude.json (legacy format)
func (h *HistoryReader) readFromClaudeConfig(projectPath string) ([]models.ClaudeHistoryEntry, error) {
	data, err := os.ReadFile(h.claudeConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.ClaudeHistoryEntry{}, nil
		}
		return nil, fmt.Errorf("failed to read .claude.json: %w", err)
	}

	var config struct {
		Projects map[string]struct {
			History []models.ClaudeHistoryEntry `json:"history"`
		} `json:"projects"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse .claude.json: %w", err)
	}

	if project, exists := config.Projects[projectPath]; exists {
		return project.History, nil
	}

	return []models.ClaudeHistoryEntry{}, nil
}

// GetAllHistory returns all history entries from history.jsonl regardless of project
// Useful for debugging or administrative purposes
func (h *HistoryReader) GetAllHistory() ([]HistoryEntry, error) {
	file, err := os.Open(h.historyJSONLPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open history.jsonl: %w", err)
	}
	defer file.Close()

	var entries []HistoryEntry
	decoder := json.NewDecoder(file)

	for {
		var entry HistoryEntry
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			// Skip invalid JSON lines but continue reading
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// CorrelateHistoryWithSession finds history entries that match a specific session
// This helps identify which user prompts led to which sessions
func (h *HistoryReader) CorrelateHistoryWithSession(projectPath, sessionID string) ([]models.ClaudeHistoryEntry, error) {
	allEntries, err := h.readFromHistoryJSONLFull(projectPath)
	if err != nil {
		return nil, err
	}

	var matchedEntries []models.ClaudeHistoryEntry
	for _, entry := range allEntries {
		if entry.SessionID == sessionID {
			matchedEntries = append(matchedEntries, models.ClaudeHistoryEntry{
				Display:        entry.Display,
				PastedContents: entry.PastedContents,
			})
		}
	}

	return matchedEntries, nil
}

// readFromHistoryJSONLFull reads full history entries including sessionID
func (h *HistoryReader) readFromHistoryJSONLFull(projectPath string) ([]HistoryEntry, error) {
	file, err := os.Open(h.historyJSONLPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open history.jsonl: %w", err)
	}
	defer file.Close()

	var entries []HistoryEntry
	decoder := json.NewDecoder(file)

	for {
		var entry HistoryEntry
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			continue
		}

		if entry.Project == projectPath {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// NormalizePath normalizes a project path for history lookups
// Claude may store paths with or without trailing slashes
func NormalizePath(path string) string {
	return strings.TrimSuffix(path, "/")
}
