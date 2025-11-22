package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/claude/parser"
	"github.com/vanpelt/catnip/internal/config"
	"github.com/vanpelt/catnip/internal/logger"
)

// ParserService manages Claude session file parser instances as a singleton per session file
// It provides centralized access to the robust parser, eliminating duplicate manual parsing logic
type ParserService struct {
	parsers       map[string]*parserInstance // key: session file path
	parsersMutex  sync.RWMutex
	claudeService *ClaudeService // For finding project directories
	maxParsers    int            // Maximum number of parsers to keep in memory (LRU eviction)
	stopCh        chan struct{}
}

// parserInstance wraps a parser with its metadata for lifecycle management
type parserInstance struct {
	reader       *parser.SessionFileReader
	lastAccess   time.Time
	filePath     string
	worktreePath string
}

// NewParserService creates a new parser service
func NewParserService() *ParserService {
	return &ParserService{
		parsers:    make(map[string]*parserInstance),
		maxParsers: 100, // Reasonable default: support 100 concurrent worktrees
		stopCh:     make(chan struct{}),
	}
}

// SetClaudeService sets the Claude service reference for finding project directories
func (s *ParserService) SetClaudeService(claudeService *ClaudeService) {
	s.parsersMutex.Lock()
	defer s.parsersMutex.Unlock()
	s.claudeService = claudeService
}

// Start begins the parser service lifecycle (periodic cleanup)
func (s *ParserService) Start() {
	logger.Info("🔧 Starting Claude session parser service")

	// Start periodic cleanup of stale parsers
	go s.cleanupLoop()
}

// Stop stops the parser service and cleans up all parsers
func (s *ParserService) Stop() {
	logger.Info("🛑 Stopping Claude session parser service")
	close(s.stopCh)

	s.parsersMutex.Lock()
	defer s.parsersMutex.Unlock()

	// Clear all parsers
	s.parsers = make(map[string]*parserInstance)
}

// GetOrCreateParser returns a parser for the given worktree, creating one if needed
// This is the primary method for accessing parsers throughout the codebase
func (s *ParserService) GetOrCreateParser(worktreePath string) (*parser.SessionFileReader, error) {
	// Find the session file for this worktree
	sessionFile, err := s.findSessionFile(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to find session file for %s: %w", worktreePath, err)
	}

	s.parsersMutex.Lock()
	defer s.parsersMutex.Unlock()

	// Check if parser already exists
	if instance, exists := s.parsers[sessionFile]; exists {
		instance.lastAccess = time.Now()
		return instance.reader, nil
	}

	// Create new parser
	reader := parser.NewSessionFileReader(sessionFile)

	// Do initial read to populate cache
	if _, err := reader.ReadIncremental(); err != nil {
		logger.Warnf("⚠️  Failed initial read for parser %s: %v", sessionFile, err)
		// Continue anyway - parser will retry on next access
	}

	instance := &parserInstance{
		reader:       reader,
		lastAccess:   time.Now(),
		filePath:     sessionFile,
		worktreePath: worktreePath,
	}

	s.parsers[sessionFile] = instance
	logger.Debugf("📖 Created parser for session file: %s", sessionFile)

	// Check if we need to evict old parsers (LRU)
	s.evictIfNeeded()

	return reader, nil
}

// RefreshParser forces a refresh of the parser for the given worktree
// Useful when you know the file has changed and want immediate update
func (s *ParserService) RefreshParser(worktreePath string) error {
	reader, err := s.GetOrCreateParser(worktreePath)
	if err != nil {
		return err
	}

	// Force an incremental read
	if _, err := reader.ReadIncremental(); err != nil {
		return fmt.Errorf("failed to refresh parser: %w", err)
	}

	return nil
}

// RemoveParser removes a parser for the given worktree
// Call this when a worktree is deleted or its session is no longer needed
func (s *ParserService) RemoveParser(worktreePath string) {
	sessionFile, err := s.findSessionFile(worktreePath)
	if err != nil {
		// If we can't find the session file, try to find by worktreePath in existing parsers
		s.parsersMutex.Lock()
		defer s.parsersMutex.Unlock()

		for filePath, instance := range s.parsers {
			if instance.worktreePath == worktreePath {
				delete(s.parsers, filePath)
				logger.Debugf("🗑️  Removed parser for worktree: %s", worktreePath)
				return
			}
		}
		return
	}

	s.parsersMutex.Lock()
	defer s.parsersMutex.Unlock()

	if _, exists := s.parsers[sessionFile]; exists {
		delete(s.parsers, sessionFile)
		logger.Debugf("🗑️  Removed parser for session file: %s", sessionFile)
	}
}

// findSessionFile finds the best session file for a given worktree
// Uses the same logic as ClaudeService to ensure consistency
func (s *ParserService) findSessionFile(worktreePath string) (string, error) {
	projectDirName := WorktreePathToProjectDir(worktreePath)

	// Check local directory first
	homeDir := config.Runtime.HomeDir
	localDir := filepath.Join(homeDir, ".claude", "projects", projectDirName)
	logger.Debugf("🔍 Looking for session file in: %s", localDir)

	if sessionFile := s.findBestSessionInDir(localDir); sessionFile != "" {
		logger.Debugf("✅ Found session file: %s", sessionFile)
		return sessionFile, nil
	}

	// Check volume directory
	volumeDir := config.Runtime.VolumeDir
	volumeProjectDir := filepath.Join(volumeDir, ".claude", ".claude", "projects", projectDirName)
	logger.Debugf("🔍 Looking for session file in volume dir: %s", volumeProjectDir)

	if sessionFile := s.findBestSessionInDir(volumeProjectDir); sessionFile != "" {
		logger.Debugf("✅ Found session file in volume: %s", sessionFile)
		return sessionFile, nil
	}

	logger.Warnf("❌ No session file found for worktree: %s (checked: %s, %s)", worktreePath, localDir, volumeProjectDir)
	return "", fmt.Errorf("no session file found for worktree: %s", worktreePath)
}

// findBestSessionInDir finds the best (largest, most recent) session file in a directory
func (s *ParserService) findBestSessionInDir(dir string) string {
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Debugf("📁 Directory does not exist: %s", dir)
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Debugf("❌ Failed to read directory %s: %v", dir, err)
		return ""
	}

	var bestFile string
	var bestSize int64
	var bestModTime time.Time
	var skippedCount int
	var jsonlCount int

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		jsonlCount++

		filePath := filepath.Join(dir, entry.Name())
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Skip very small files (likely warmup-only sessions)
		if info.Size() < 10000 {
			skippedCount++
			continue
		}

		// Prefer larger files (more content), use mod time as tie-breaker
		if info.Size() > bestSize || (info.Size() == bestSize && info.ModTime().After(bestModTime)) {
			bestFile = filePath
			bestSize = info.Size()
			bestModTime = info.ModTime()
		}
	}

	logger.Debugf("📊 Scanned %d .jsonl files in %s, skipped %d small files, best file: %s (%d bytes)",
		jsonlCount, dir, skippedCount, bestFile, bestSize)

	return bestFile
}

// evictIfNeeded evicts least recently used parsers if we exceed maxParsers
// Must be called with parsersMutex held
func (s *ParserService) evictIfNeeded() {
	if len(s.parsers) <= s.maxParsers {
		return
	}

	// Find least recently used parser
	var oldestKey string
	var oldestTime time.Time

	for key, instance := range s.parsers {
		if oldestKey == "" || instance.lastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = instance.lastAccess
		}
	}

	if oldestKey != "" {
		delete(s.parsers, oldestKey)
		logger.Debugf("🗑️  Evicted LRU parser: %s (last access: %v ago)", oldestKey, time.Since(oldestTime))
	}
}

// cleanupLoop periodically cleans up stale parsers (not accessed in 1 hour)
func (s *ParserService) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupStaleParsers()
		case <-s.stopCh:
			return
		}
	}
}

// cleanupStaleParsers removes parsers that haven't been accessed in a while
func (s *ParserService) cleanupStaleParsers() {
	s.parsersMutex.Lock()
	defer s.parsersMutex.Unlock()

	staleThreshold := 1 * time.Hour
	now := time.Now()
	staleParsers := []string{}

	for key, instance := range s.parsers {
		if now.Sub(instance.lastAccess) > staleThreshold {
			staleParsers = append(staleParsers, key)
		}
	}

	for _, key := range staleParsers {
		delete(s.parsers, key)
	}

	if len(staleParsers) > 0 {
		logger.Debugf("🧹 Cleaned up %d stale parsers", len(staleParsers))
	}
}

// GetStats returns statistics about the parser service
func (s *ParserService) GetStats() map[string]interface{} {
	s.parsersMutex.RLock()
	defer s.parsersMutex.RUnlock()

	return map[string]interface{}{
		"active_parsers": len(s.parsers),
		"max_parsers":    s.maxParsers,
	}
}
