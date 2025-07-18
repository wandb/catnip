// Package cache provides caching functionality for Git operations
package cache

import (
	"log"
	"time"

	"github.com/vanpelt/catnip/internal/models"
)

// ConflictCache provides type-safe operations for conflict checking cache
type ConflictCache struct {
	cache Cache
}

// ConflictCacheEntry represents a cached conflict check result
type ConflictCacheEntry struct {
	WorktreeCommit string
	SourceCommit   string
	Result         interface{} // Can be *models.MergeConflictError or *CombinedConflictCheckResult
	CheckedAt      time.Time
}

// CombinedConflictCheckResult represents results for both sync and merge operations
type CombinedConflictCheckResult struct {
	WorktreeID    string                     `json:"worktree_id"`
	WorktreeName  string                     `json:"worktree_name"`
	SyncConflict  *models.MergeConflictError `json:"sync_conflict"`
	MergeConflict *models.MergeConflictError `json:"merge_conflict"`
	CheckedAt     time.Time                  `json:"checked_at"`
}

// NewConflictCache creates a new conflict cache with the given base cache
func NewConflictCache(cache Cache) *ConflictCache {
	return &ConflictCache{
		cache: cache,
	}
}

// NewConflictCacheWithDefaults creates a new conflict cache with default LRU cache
func NewConflictCacheWithDefaults() *ConflictCache {
	return NewConflictCache(NewLRUCacheWithDefaults())
}

// GetConflictResult retrieves a conflict check result from cache
func (c *ConflictCache) GetConflictResult(key string) (*models.MergeConflictError, bool) {
	entry, exists := c.getEntry(key)
	if !exists {
		return nil, false
	}

	// Type assert to the expected type
	if result, ok := entry.Result.(*models.MergeConflictError); ok {
		return result, true
	}

	return nil, false
}

// GetCombinedResult retrieves a combined conflict check result from cache
func (c *ConflictCache) GetCombinedResult(key string) (*CombinedConflictCheckResult, bool) {
	entry, exists := c.getEntry(key)
	if !exists {
		return nil, false
	}

	// Type assert to the expected type
	if result, ok := entry.Result.(*CombinedConflictCheckResult); ok {
		return result, true
	}

	return nil, false
}

// SetConflictResult stores a conflict check result in cache
func (c *ConflictCache) SetConflictResult(key string, worktreeCommit, sourceCommit string, result *models.MergeConflictError) {
	entry := &ConflictCacheEntry{
		WorktreeCommit: worktreeCommit,
		SourceCommit:   sourceCommit,
		Result:         result,
		CheckedAt:      time.Now(),
	}

	c.cache.Set(key, entry)
}

// SetCombinedResult stores a combined conflict check result in cache
func (c *ConflictCache) SetCombinedResult(key string, worktreeCommit, sourceCommit string, result *CombinedConflictCheckResult) {
	entry := &ConflictCacheEntry{
		WorktreeCommit: worktreeCommit,
		SourceCommit:   sourceCommit,
		Result:         result,
		CheckedAt:      time.Now(),
	}

	c.cache.Set(key, entry)
}

// IsValid checks if a cache entry is still valid based on commits
func (c *ConflictCache) IsValid(key string, worktreeCommit, sourceCommit string) bool {
	entry, exists := c.getEntry(key)
	if !exists {
		return false
	}

	// Check if commits have changed
	return entry.WorktreeCommit == worktreeCommit && entry.SourceCommit == sourceCommit
}

// Clear removes all entries with a specific prefix
func (c *ConflictCache) Clear(prefix string) {
	c.cache.Clear(prefix)
}

// Delete removes a specific entry
func (c *ConflictCache) Delete(key string) {
	c.cache.Delete(key)
}

// Cleanup removes expired entries
func (c *ConflictCache) Cleanup(maxAge time.Duration) {
	c.cache.Cleanup(maxAge)
}

// Size returns the current cache size
func (c *ConflictCache) Size() int {
	return c.cache.Size()
}

// Stats returns cache statistics
func (c *ConflictCache) Stats() Stats {
	return c.cache.Stats()
}

// Close properly shuts down the cache
func (c *ConflictCache) Close() error {
	return c.cache.Close()
}

// getEntry retrieves a cache entry with proper type assertion
func (c *ConflictCache) getEntry(key string) (*ConflictCacheEntry, bool) {
	value, exists := c.cache.Get(key)
	if !exists {
		return nil, false
	}

	entry, ok := value.(*ConflictCacheEntry)
	if !ok {
		// Invalid entry type, remove it and log the issue
		// This could indicate a bug in the cache implementation
		log.Printf("⚠️ Invalid cache entry type for key %s, removing corrupted entry", key)
		c.cache.Delete(key)
		return nil, false
	}

	return entry, true
}

// FormatKey generates a cache key for conflict checks
func (c *ConflictCache) FormatKey(worktreeID, operation string) string {
	return worktreeID + ":" + operation
}

// FormatCombinedKey generates a cache key for combined conflict checks
func (c *ConflictCache) FormatCombinedKey(worktreeID string) string {
	return worktreeID + ":combined"
}
