package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/config"
)

// setupTestParserService creates a test ParserService with test directories
func setupTestParserService(t *testing.T) *ParserService {
	t.Helper()

	// Create temporary directories for testing
	tempDir := t.TempDir()

	// Set up config paths
	originalHomeDir := config.Runtime.HomeDir
	originalVolumeDir := config.Runtime.VolumeDir

	config.Runtime.HomeDir = tempDir
	config.Runtime.VolumeDir = tempDir

	t.Cleanup(func() {
		config.Runtime.HomeDir = originalHomeDir
		config.Runtime.VolumeDir = originalVolumeDir
	})

	return NewParserService()
}

// copyTestFile copies a file from the parser testdata directory
func copyTestFile(t *testing.T, testdataFile, destDir string) string {
	t.Helper()

	// Read from parser testdata
	srcPath := filepath.Join("../../internal/claude/parser/testdata", testdataFile)
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err, "Failed to read test file: %s", srcPath)

	// Write to destination
	destPath := filepath.Join(destDir, testdataFile)
	err = os.WriteFile(destPath, data, 0644)
	require.NoError(t, err)

	return destPath
}

// setupTestSession creates a test session file for the given worktree path
func setupTestSession(t *testing.T, worktreePath, testdataFile string) {
	t.Helper()

	projectDirName := WorktreePathToProjectDir(worktreePath)
	projectDir := filepath.Join(config.Runtime.HomeDir, ".claude", "projects", projectDirName)
	err := os.MkdirAll(projectDir, 0755)
	require.NoError(t, err)

	sessionFile := copyTestFile(t, testdataFile, projectDir)

	// Ensure file is large enough (>10KB threshold)
	// Pad with newlines instead of zeros so JSON decoder doesn't block
	data, _ := os.ReadFile(sessionFile)
	if len(data) < 10240 {
		padding := make([]byte, 10240-len(data))
		for i := range padding {
			padding[i] = '\n'
		}
		data = append(data, padding...)
		err = os.WriteFile(sessionFile, data, 0644)
		require.NoError(t, err)
	}
}

// Test ParserService Creation
func TestNewParserService(t *testing.T) {
	service := NewParserService()

	assert.NotNil(t, service)
	assert.NotNil(t, service.parsers)
	assert.Equal(t, 100, service.maxParsers)
	assert.NotNil(t, service.stopCh)
}

// Test ParserService Start/Stop lifecycle
func TestParserService_StartStop(t *testing.T) {
	service := NewParserService()

	// Start service
	service.Start()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Stop service
	service.Stop()

	// Verify parsers are cleared
	assert.Empty(t, service.parsers)
}

// Test SetClaudeService
func TestParserService_SetClaudeService(t *testing.T) {
	parserService := NewParserService()
	claudeService := NewClaudeService()

	parserService.SetClaudeService(claudeService)

	assert.Equal(t, claudeService, parserService.claudeService)
}

// Test GetOrCreateParser creates new parser
func TestParserService_GetOrCreateParser_NewParser(t *testing.T) {
	service := setupTestParserService(t)
	claudeService := NewClaudeService()
	service.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	reader, err := service.GetOrCreateParser(worktreePath)

	assert.NoError(t, err)
	assert.NotNil(t, reader)
	assert.Len(t, service.parsers, 1)
}

// Test GetOrCreateParser returns cached parser
func TestParserService_GetOrCreateParser_CachedParser(t *testing.T) {
	service := setupTestParserService(t)
	claudeService := NewClaudeService()
	service.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// First call creates parser
	reader1, err := service.GetOrCreateParser(worktreePath)
	require.NoError(t, err)

	// Second call should return same parser
	reader2, err := service.GetOrCreateParser(worktreePath)
	require.NoError(t, err)

	assert.Equal(t, reader1, reader2)
	assert.Len(t, service.parsers, 1)
}

// Test GetOrCreateParser with missing session file
func TestParserService_GetOrCreateParser_NoSessionFile(t *testing.T) {
	service := setupTestParserService(t)
	claudeService := NewClaudeService()
	service.SetClaudeService(claudeService)

	worktreePath := "/nonexistent/worktree"

	reader, err := service.GetOrCreateParser(worktreePath)

	assert.Error(t, err)
	assert.Nil(t, reader)
	assert.Contains(t, err.Error(), "no session file found")
}

// Test RefreshParser
func TestParserService_RefreshParser(t *testing.T) {
	service := setupTestParserService(t)
	claudeService := NewClaudeService()
	service.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// Create parser
	_, err := service.GetOrCreateParser(worktreePath)
	require.NoError(t, err)

	// Refresh parser
	err = service.RefreshParser(worktreePath)
	assert.NoError(t, err)
}

// Test RemoveParser
func TestParserService_RemoveParser(t *testing.T) {
	service := setupTestParserService(t)
	claudeService := NewClaudeService()
	service.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// Create parser
	_, err := service.GetOrCreateParser(worktreePath)
	require.NoError(t, err)
	assert.Len(t, service.parsers, 1)

	// Remove parser
	service.RemoveParser(worktreePath)

	// Verify parser was removed
	assert.Empty(t, service.parsers)
}

// Test LRU eviction
func TestParserService_LRUEviction(t *testing.T) {
	service := setupTestParserService(t)
	service.maxParsers = 2 // Set low limit for testing

	claudeService := NewClaudeService()
	service.SetClaudeService(claudeService)

	// Create 3 different worktrees with session files
	for i := 1; i <= 3; i++ {
		worktreePath := "/test/worktree" + string(rune('0'+i))
		setupTestSession(t, worktreePath, "todos_single.jsonl")
	}

	// Create 3 parsers (should trigger eviction)
	for i := 1; i <= 3; i++ {
		worktreePath := "/test/worktree" + string(rune('0'+i))
		_, err := service.GetOrCreateParser(worktreePath)
		require.NoError(t, err)

		// Small delay to ensure different access times
		time.Sleep(10 * time.Millisecond)
	}

	// Should only have 2 parsers (max limit)
	assert.LessOrEqual(t, len(service.parsers), 2)
}

// Test GetStats
func TestParserService_GetStats(t *testing.T) {
	service := setupTestParserService(t)

	stats := service.GetStats()

	assert.Equal(t, 0, stats["active_parsers"])
	assert.Equal(t, 100, stats["max_parsers"])
}

// Test findBestSessionInDir with multiple files
func TestParserService_FindBestSessionInDir(t *testing.T) {
	service := setupTestParserService(t)

	testDir := t.TempDir()

	// Create multiple session files with different sizes
	smallFile := filepath.Join(testDir, "small.jsonl")
	err := os.WriteFile(smallFile, []byte("small"), 0644)
	require.NoError(t, err)

	largeFile := filepath.Join(testDir, "large.jsonl")
	largeData := make([]byte, 20000)
	err = os.WriteFile(largeFile, largeData, 0644)
	require.NoError(t, err)

	// findBestSessionInDir should return the larger file
	bestFile := service.findBestSessionInDir(testDir)

	assert.Equal(t, largeFile, bestFile)
}

// Test findBestSessionInDir with no valid files
func TestParserService_FindBestSessionInDir_NoFiles(t *testing.T) {
	service := setupTestParserService(t)

	testDir := t.TempDir()

	bestFile := service.findBestSessionInDir(testDir)

	assert.Empty(t, bestFile)
}

// Test findBestSessionInDir skips small files
func TestParserService_FindBestSessionInDir_SkipsSmallFiles(t *testing.T) {
	service := setupTestParserService(t)

	testDir := t.TempDir()

	// Create only small files (< 10KB)
	smallFile := filepath.Join(testDir, "small.jsonl")
	err := os.WriteFile(smallFile, []byte("tiny"), 0644)
	require.NoError(t, err)

	bestFile := service.findBestSessionInDir(testDir)

	assert.Empty(t, bestFile)
}

// Test ClaudeService.GetLatestTodos integration
func TestClaudeService_GetLatestTodos_Integration(t *testing.T) {
	// Setup
	parserService := setupTestParserService(t)
	claudeService := NewClaudeService()
	claudeService.SetParserService(parserService)
	parserService.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// Get todos
	todos, err := claudeService.GetLatestTodos(worktreePath)

	assert.NoError(t, err)
	assert.NotEmpty(t, todos)
}

// Test ClaudeService.GetLatestTodos with nil parserService
func TestClaudeService_GetLatestTodos_NilParserService(t *testing.T) {
	claudeService := NewClaudeService()
	// Don't set parserService

	worktreePath := "/test/worktree"

	todos, err := claudeService.GetLatestTodos(worktreePath)

	assert.Error(t, err)
	assert.Nil(t, todos)
	assert.Contains(t, err.Error(), "parser service not initialized")
}

// Test ClaudeService.GetLatestAssistantMessage integration
func TestClaudeService_GetLatestAssistantMessage_Integration(t *testing.T) {
	// Setup
	parserService := setupTestParserService(t)
	claudeService := NewClaudeService()
	claudeService.SetParserService(parserService)
	parserService.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// Get latest message
	message, err := claudeService.GetLatestAssistantMessage(worktreePath)

	assert.NoError(t, err)
	assert.NotEmpty(t, message)
}

// Test ClaudeService.GetLatestAssistantMessage with nil parserService
func TestClaudeService_GetLatestAssistantMessage_NilParserService(t *testing.T) {
	claudeService := NewClaudeService()

	worktreePath := "/test/worktree"

	message, err := claudeService.GetLatestAssistantMessage(worktreePath)

	assert.Error(t, err)
	assert.Empty(t, message)
	assert.Contains(t, err.Error(), "parser service not initialized")
}

// Test ClaudeService.GetLatestAssistantMessageOrError integration
func TestClaudeService_GetLatestAssistantMessageOrError_Integration(t *testing.T) {
	// Setup
	parserService := setupTestParserService(t)
	claudeService := NewClaudeService()
	claudeService.SetParserService(parserService)
	parserService.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// Get latest message or error
	content, isError, err := claudeService.GetLatestAssistantMessageOrError(worktreePath)

	assert.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.False(t, isError)
}

// Test ClaudeService.GetLatestAssistantMessageOrError with nil parserService
func TestClaudeService_GetLatestAssistantMessageOrError_NilParserService(t *testing.T) {
	claudeService := NewClaudeService()

	worktreePath := "/test/worktree"

	content, isError, err := claudeService.GetLatestAssistantMessageOrError(worktreePath)

	assert.Error(t, err)
	assert.Empty(t, content)
	assert.False(t, isError)
	assert.Contains(t, err.Error(), "parser service not initialized")
}

// Test ClaudeService.GetLatestAssistantMessageOrError with no message
func TestClaudeService_GetLatestAssistantMessageOrError_NoMessage(t *testing.T) {
	// Setup
	parserService := setupTestParserService(t)
	claudeService := NewClaudeService()
	claudeService.SetParserService(parserService)
	parserService.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "minimal.jsonl")

	// Get latest message or error
	content, isError, err := claudeService.GetLatestAssistantMessageOrError(worktreePath)

	assert.NoError(t, err)
	assert.Empty(t, content)
	assert.False(t, isError)
}

// Test WorktreeTodoMonitor.readTodosFromEnd integration
func TestWorkTreeTodoMonitor_ReadTodosFromEnd_Integration(t *testing.T) {
	// Setup
	parserService := setupTestParserService(t)
	claudeService := NewClaudeService()
	parserService.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// Create WorktreeTodoMonitor
	monitor := &WorktreeTodoMonitor{
		workDir:       worktreePath,
		projectDir:    WorktreePathToProjectDir(worktreePath),
		parserService: parserService,
	}

	// Read todos (filePath parameter is ignored, workDir is used)
	todos, err := monitor.readTodosFromEnd("")

	assert.NoError(t, err)
	assert.NotEmpty(t, todos)
}

// Test WorktreeTodoMonitor.readTodosFromEnd with nil parserService
func TestWorkTreeTodoMonitor_ReadTodosFromEnd_NilParserService(t *testing.T) {
	monitor := &WorktreeTodoMonitor{
		workDir:       "/test/worktree",
		projectDir:    "test-worktree",
		parserService: nil,
	}

	todos, err := monitor.readTodosFromEnd("/fake/path.jsonl")

	assert.Error(t, err)
	assert.Nil(t, todos)
	assert.Contains(t, err.Error(), "parser service not initialized")
}

// Test cleanupStaleParsers
func TestParserService_CleanupStaleParsers(t *testing.T) {
	service := setupTestParserService(t)
	claudeService := NewClaudeService()
	service.SetClaudeService(claudeService)

	worktreePath := "/test/worktree"
	setupTestSession(t, worktreePath, "todos_single.jsonl")

	// Create parser
	_, err := service.GetOrCreateParser(worktreePath)
	require.NoError(t, err)
	assert.Len(t, service.parsers, 1)

	// Manually set last access time to old value
	for _, instance := range service.parsers {
		instance.lastAccess = time.Now().Add(-2 * time.Hour)
	}

	// Run cleanup
	service.cleanupStaleParsers()

	// Verify stale parser was removed
	assert.Empty(t, service.parsers)
}
