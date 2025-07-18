package cache

import (
	"time"
)

// Cache defines the interface for caching operations
type Cache interface {
	// Get retrieves an item from cache
	Get(key string) (interface{}, bool)

	// Set stores an item in cache
	Set(key string, value interface{})

	// SetWithTTL stores an item in cache with a custom TTL
	SetWithTTL(key string, value interface{}, ttl time.Duration)

	// Delete removes an item from cache
	Delete(key string)

	// Clear removes all items with a specific prefix
	Clear(prefix string)

	// Cleanup removes expired entries
	Cleanup(maxAge time.Duration)

	// Size returns the current cache size
	Size() int

	// Stats returns cache statistics
	Stats() Stats

	// Close properly shuts down the cache
	Close() error
}

// Stats represents cache performance metrics
type Stats struct {
	Hits        int64     `json:"hits"`
	Misses      int64     `json:"misses"`
	Evictions   int64     `json:"evictions"`
	Size        int       `json:"size"`
	MaxSize     int       `json:"max_size"`
	HitRate     float64   `json:"hit_rate"`
	LastCleanup time.Time `json:"last_cleanup"`
}

// Config defines configuration options for cache implementations
type Config struct {
	MaxSize       int           `json:"max_size"`
	DefaultTTL    time.Duration `json:"default_ttl"`
	CleanupPeriod time.Duration `json:"cleanup_period"`
	EnableStats   bool          `json:"enable_stats"`
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() Config {
	return Config{
		MaxSize:       100,
		DefaultTTL:    5 * time.Minute,
		CleanupPeriod: 5 * time.Minute,
		EnableStats:   true,
	}
}
