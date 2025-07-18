package cache

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

// LRUEntry represents an entry in the LRU cache
type LRUEntry struct {
	Key       string
	Value     interface{}
	CreatedAt time.Time
	TTL       time.Duration
}

// IsExpired checks if the entry has expired
func (e *LRUEntry) IsExpired() bool {
	if e.TTL == 0 {
		return false
	}
	return time.Since(e.CreatedAt) > e.TTL
}

// LRUCache implements the Cache interface using LRU eviction policy
type LRUCache struct {
	config       Config
	items        map[string]*list.Element
	evictionList *list.List
	stats        Stats
	mu           sync.RWMutex
	stopCleanup  chan struct{}
	cleanupDone  chan struct{}
}

// NewLRUCache creates a new LRU cache with the given configuration
func NewLRUCache(config Config) *LRUCache {
	cache := &LRUCache{
		config:       config,
		items:        make(map[string]*list.Element),
		evictionList: list.New(),
		stats: Stats{
			MaxSize:     config.MaxSize,
			LastCleanup: time.Now(),
		},
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start background cleanup if enabled
	if config.CleanupPeriod > 0 {
		go cache.backgroundCleanup()
	}

	return cache
}

// NewLRUCacheWithDefaults creates a new LRU cache with default configuration
func NewLRUCacheWithDefaults() *LRUCache {
	return NewLRUCache(DefaultConfig())
}

// Get retrieves an item from cache
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, exists := c.items[key]
	if !exists {
		if c.config.EnableStats {
			c.stats.Misses++
		}
		return nil, false
	}

	entry := element.Value.(*LRUEntry)

	// Check if expired
	if entry.IsExpired() {
		c.removeElementUnsafe(element)
		if c.config.EnableStats {
			c.stats.Misses++
		}
		return nil, false
	}

	// Move to front (most recently used)
	c.evictionList.MoveToFront(element)

	if c.config.EnableStats {
		c.stats.Hits++
	}

	return entry.Value, true
}

// Set stores an item in cache with default TTL
func (c *LRUCache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.config.DefaultTTL)
}

// SetWithTTL stores an item in cache with custom TTL
func (c *LRUCache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if element, exists := c.items[key]; exists {
		// Update existing entry
		entry := element.Value.(*LRUEntry)
		entry.Value = value
		entry.CreatedAt = time.Now()
		entry.TTL = ttl
		c.evictionList.MoveToFront(element)
		return
	}

	// Add new entry
	entry := &LRUEntry{
		Key:       key,
		Value:     value,
		CreatedAt: time.Now(),
		TTL:       ttl,
	}

	element := c.evictionList.PushFront(entry)
	c.items[key] = element

	// Evict if over capacity
	if c.evictionList.Len() > c.config.MaxSize {
		c.evictOldestUnsafe()
	}
}

// Delete removes an item from cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, exists := c.items[key]; exists {
		c.removeElementUnsafe(element)
	}
}

// Clear removes all items with a specific prefix
func (c *LRUCache) Clear(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Collect keys to remove (to avoid modifying map during iteration)
	keysToRemove := make([]string, 0)
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			keysToRemove = append(keysToRemove, key)
		}
	}

	// Remove collected keys
	for _, key := range keysToRemove {
		if element, exists := c.items[key]; exists {
			c.removeElementUnsafe(element)
		}
	}
}

// Cleanup removes expired entries
func (c *LRUCache) Cleanup(maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	keysToRemove := make([]string, 0)

	// Find expired entries
	for key, element := range c.items {
		entry := element.Value.(*LRUEntry)

		// Check both custom TTL and maxAge
		isExpired := entry.IsExpired() || (maxAge > 0 && now.Sub(entry.CreatedAt) > maxAge)

		if isExpired {
			keysToRemove = append(keysToRemove, key)
		}
	}

	// Remove expired entries
	for _, key := range keysToRemove {
		if element, exists := c.items[key]; exists {
			c.removeElementUnsafe(element)
		}
	}

	c.stats.LastCleanup = now
}

// Size returns the current cache size
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats returns cache statistics
func (c *LRUCache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := c.stats
	stats.Size = len(c.items)

	// Calculate hit rate under lock to avoid race conditions
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRate = float64(stats.Hits) / float64(total)
	} else {
		stats.HitRate = 0.0
	}

	return stats
}

// Close properly shuts down the cache
func (c *LRUCache) Close() error {
	// Only signal and wait if background cleanup was started
	if c.config.CleanupPeriod > 0 {
		// Signal cleanup goroutine to stop
		close(c.stopCleanup)

		// Wait for cleanup to finish
		<-c.cleanupDone
	}

	// Clear all items
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictionList = list.New()

	return nil
}

// removeElementUnsafe removes an element from cache (caller must hold lock)
func (c *LRUCache) removeElementUnsafe(element *list.Element) {
	entry := element.Value.(*LRUEntry)
	delete(c.items, entry.Key)
	c.evictionList.Remove(element)
}

// evictOldestUnsafe evicts the oldest entry (caller must hold lock)
func (c *LRUCache) evictOldestUnsafe() {
	oldest := c.evictionList.Back()
	if oldest != nil {
		c.removeElementUnsafe(oldest)
		if c.config.EnableStats {
			c.stats.Evictions++
		}
	}
}

// backgroundCleanup runs periodic cleanup in a separate goroutine
func (c *LRUCache) backgroundCleanup() {
	defer close(c.cleanupDone)

	ticker := time.NewTicker(c.config.CleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.Cleanup(0) // Use entry TTL for expiration
		case <-c.stopCleanup:
			return
		}
	}
}
