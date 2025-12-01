package paths

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test IsValidSessionUUID
func TestIsValidSessionUUID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid UUID", "cf568042-7147-4fba-a2ca-c6a646581260", true},
		{"valid UUID all same", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", true},
		{"agent file", "agent-d221d088", false},
		{"too short", "abc-123", false},
		{"too long", "cf568042-7147-4fba-a2ca-c6a646581260-extra", false},
		{"wrong number of dashes", "cf5680427147-4fba-a2ca-c6a646581260", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidSessionUUID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test EncodePathForClaude
func TestEncodePathForClaude(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "/workspaces/myproject", "-workspaces-myproject"},
		{"path with dots", "/home/user/my.project", "-home-user-my-project"},
		{"already has leading dash", "-foo/bar", "-foo-bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodePathForClaude(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test FindBestSessionFile with multiple files - selects largest valid UUID file
func TestFindBestSessionFile_SelectsLargestValidUUID(t *testing.T) {
	testDir := t.TempDir()

	// Create a valid UUID session file (large, with conversation content)
	validUUID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl"
	validContent := []byte(`{"type":"user","message":"hello"}` + "\n" + `{"type":"assistant","message":"hi"}`)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, validUUID), validContent, 0644))

	// Create an agent file (should be ignored even if larger)
	agentFile := "agent-d221d088.jsonl"
	agentContent := make([]byte, 50000) // Much larger
	copy(agentContent, []byte(`{"type":"user","message":"agent task"}`))
	require.NoError(t, os.WriteFile(filepath.Join(testDir, agentFile), agentContent, 0644))

	// FindBestSessionFile should return the valid UUID file, not the agent file
	bestFile, err := FindBestSessionFile(testDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, validUUID), bestFile)
}

// Test FindBestSessionFile with no valid files
func TestFindBestSessionFile_NoValidFiles(t *testing.T) {
	testDir := t.TempDir()

	// Create only agent files (no valid UUIDs)
	agentFile := "agent-abc123.jsonl"
	require.NoError(t, os.WriteFile(filepath.Join(testDir, agentFile), []byte("content"), 0644))

	// Should return error
	bestFile, err := FindBestSessionFile(testDir)
	assert.Error(t, err)
	assert.Empty(t, bestFile)
}

// Test FindBestSessionFile with empty directory
func TestFindBestSessionFile_EmptyDirectory(t *testing.T) {
	testDir := t.TempDir()

	bestFile, err := FindBestSessionFile(testDir)
	assert.Error(t, err)
	assert.Empty(t, bestFile)
}

// Test FindBestSessionFile with nonexistent directory
func TestFindBestSessionFile_NonexistentDirectory(t *testing.T) {
	bestFile, err := FindBestSessionFile("/nonexistent/path")
	assert.Error(t, err)
	assert.Empty(t, bestFile)
}

// Test FindBestSessionFile prefers most recently modified
func TestFindBestSessionFile_PrefersRecentlyModified(t *testing.T) {
	testDir := t.TempDir()

	// Create an older file
	olderUUID := "11111111-1111-1111-1111-111111111111.jsonl"
	olderContent := []byte(`{"type":"user","message":"old"}` + "\n" + `{"type":"assistant","message":"old reply"}`)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, olderUUID), olderContent, 0644))

	// Wait a bit to ensure different mod times
	time.Sleep(50 * time.Millisecond)

	// Create a newer file
	newerUUID := "22222222-2222-2222-2222-222222222222.jsonl"
	newerContent := []byte(`{"type":"user","message":"new"}` + "\n" + `{"type":"assistant","message":"new reply"}`)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, newerUUID), newerContent, 0644))

	// Should prefer the newer file
	bestFile, err := FindBestSessionFile(testDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, newerUUID), bestFile)
}

// Test FindBestSessionFile skips snapshot-only files
func TestFindBestSessionFile_SkipsSnapshotOnlyFiles(t *testing.T) {
	testDir := t.TempDir()

	// Create a snapshot-only file (no user/assistant messages)
	snapshotUUID := "33333333-3333-3333-3333-333333333333.jsonl"
	snapshotContent := []byte(`{"type":"file-history-snapshot","data":{}}` + "\n" + `{"type":"summary","content":""}`)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, snapshotUUID), snapshotContent, 0644))

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Create a valid conversation file
	validUUID := "44444444-4444-4444-4444-444444444444.jsonl"
	validContent := []byte(`{"type":"user","message":"hello"}` + "\n" + `{"type":"assistant","message":"hi"}`)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, validUUID), validContent, 0644))

	// Should return the valid conversation file, not the snapshot-only file
	bestFile, err := FindBestSessionFile(testDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, validUUID), bestFile)
}

// Test FindBestSessionFile skips forked sessions (queue-operation)
func TestFindBestSessionFile_SkipsForkedSessions(t *testing.T) {
	testDir := t.TempDir()

	// Create a forked session file (starts with queue-operation)
	forkedUUID := "55555555-5555-5555-5555-555555555555.jsonl"
	forkedContent := []byte(`{"type":"queue-operation","task":"branch-naming"}` + "\n" + `{"type":"user","message":"name this"}`)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, forkedUUID), forkedContent, 0644))

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Create a valid main session file
	validUUID := "66666666-6666-6666-6666-666666666666.jsonl"
	validContent := []byte(`{"type":"user","message":"real task"}` + "\n" + `{"type":"assistant","message":"working on it"}`)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, validUUID), validContent, 0644))

	// Should return the valid main session, not the forked one
	bestFile, err := FindBestSessionFile(testDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, validUUID), bestFile)
}

// Test FindBestSessionFile uses size as tie-breaker when mod times are equal
func TestFindBestSessionFile_UsesSizeAsTieBreaker(t *testing.T) {
	testDir := t.TempDir()

	// Create two files with same mod time but different sizes
	smallUUID := "77777777-7777-7777-7777-777777777777.jsonl"
	smallContent := []byte(`{"type":"user","message":"hi"}` + "\n" + `{"type":"assistant","message":"hi"}`)

	largeUUID := "88888888-8888-8888-8888-888888888888.jsonl"
	largeContent := []byte(`{"type":"user","message":"hello world this is a longer message"}` + "\n" + `{"type":"assistant","message":"hello there, this is also a longer response message"}`)

	// Write both files
	require.NoError(t, os.WriteFile(filepath.Join(testDir, smallUUID), smallContent, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(testDir, largeUUID), largeContent, 0644))

	// Set same mod time on both
	now := time.Now()
	require.NoError(t, os.Chtimes(filepath.Join(testDir, smallUUID), now, now))
	require.NoError(t, os.Chtimes(filepath.Join(testDir, largeUUID), now, now))

	// Should prefer the larger file as tie-breaker
	bestFile, err := FindBestSessionFile(testDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, largeUUID), bestFile)
}

// Test that agent files are never selected even with valid conversation content
func TestFindBestSessionFile_NeverSelectsAgentFiles(t *testing.T) {
	testDir := t.TempDir()

	// Create multiple agent files with valid conversation content
	for _, name := range []string{"agent-abc123.jsonl", "agent-def456.jsonl", "agent-ghi789.jsonl"} {
		content := []byte(`{"type":"user","message":"agent task"}` + "\n" + `{"type":"assistant","message":"doing it"}`)
		require.NoError(t, os.WriteFile(filepath.Join(testDir, name), content, 0644))
	}

	// Should return error since no valid UUID files exist
	bestFile, err := FindBestSessionFile(testDir)
	assert.Error(t, err)
	assert.Empty(t, bestFile)
}

// Test hasConversationContent helper
func TestHasConversationContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "has user and assistant",
			content:  `{"type":"user","message":"hi"}` + "\n" + `{"type":"assistant","message":"hello"}`,
			expected: true,
		},
		{
			name:     "only user message",
			content:  `{"type":"user","message":"hi"}`,
			expected: true,
		},
		{
			name:     "only assistant message",
			content:  `{"type":"assistant","message":"hi"}`,
			expected: true,
		},
		{
			name:     "only snapshots",
			content:  `{"type":"file-history-snapshot","data":{}}` + "\n" + `{"type":"summary","content":""}`,
			expected: false,
		},
		{
			name:     "starts with queue-operation (forked)",
			content:  `{"type":"queue-operation","task":"branch"}` + "\n" + `{"type":"user","message":"hi"}`,
			expected: false,
		},
		{
			name:     "empty file",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			testFile := filepath.Join(testDir, "test.jsonl")
			require.NoError(t, os.WriteFile(testFile, []byte(tt.content), 0644))

			result := hasConversationContent(testFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}
