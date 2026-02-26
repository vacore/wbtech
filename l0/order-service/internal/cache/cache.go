package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"order-service/internal/config"
	"order-service/internal/metrics"
	"order-service/internal/models"
)

// cacheEntry describes a single cache item's data
type cacheEntry struct {
	order      *models.Order
	accessedAt time.Time     // Last access time (for sliding TTL)
	element    *list.Element // Pointer to item in the LRU list
}

// Cache is a thread-safe LRU cache with sliding-window TTL
type Cache struct {
	mu      sync.RWMutex
	items   map[string]*cacheEntry
	lruList *list.List
	config  config.CacheConfig
	stats   Stats
	cancel  context.CancelFunc
}

// Stats contains cache statistics
type Stats struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	Expirations int64
}

// New creates a new LRU+TTL cache instance
func New(config config.CacheConfig) *Cache {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Cache{
		items:   make(map[string]*cacheEntry),
		lruList: list.New(),
		config:  config,
		cancel:  cancel,
	}

	if config.TTL > 0 {
		go c.cleanupLoop(ctx)
	}

	return c
}

// Set adds or updates a cache item, resetting its TTL
func (c *Cache) Set(order *models.Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check if an order with this uid is present in the cache - update and return
	if entry, exists := c.items[order.OrderUID]; exists {
		// Replace present order with same order_uid with the updated one
		entry.order = order
		// Update access time
		entry.accessedAt = now
		// Make it the most recent (front of the list)
		c.lruList.MoveToFront(entry.element)
		return
	}

	// Check if the limit is reached
	if c.config.MaxItems > 0 && len(c.items) >= c.config.MaxItems {
		c.evictOldest()
	}

	// Add new cache element
	entry := &cacheEntry{
		order:      order,
		accessedAt: now,
	}

	// Add to the front (this is the most recent now)
	entry.element = c.lruList.PushFront(order.OrderUID)
	c.items[order.OrderUID] = entry

	metrics.CacheSize.Set(float64(len(c.items)))
}

// Get returns a cached order. Each successful Get resets the TTL (sliding window).
func (c *Cache) Get(orderUID string) (*models.Order, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[orderUID]
	if !exists {
		c.stats.Misses++
		metrics.CacheOps.WithLabelValues("miss").Inc()
		return nil, false
	}

	// Check TTL
	if c.config.TTL > 0 && time.Since(entry.accessedAt) > c.config.TTL {
		// The entry has expired - remove it
		c.removeEntry(orderUID, entry)
		c.stats.Expirations++
		metrics.CacheOps.WithLabelValues("expiration").Inc()
		c.stats.Misses++
		metrics.CacheOps.WithLabelValues("miss").Inc()
		return nil, false
	}

	// Update access time and make it the most recent (move to front of the list)
	entry.accessedAt = time.Now()
	c.lruList.MoveToFront(entry.element)
	c.stats.Hits++
	metrics.CacheOps.WithLabelValues("hit").Inc()

	return entry.order, true
}

// Size returns current number of cached items
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Delete removes an item from the cache
func (c *Cache) Delete(orderUID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.items[orderUID]; exists {
		c.removeEntry(orderUID, entry)
	}
}

// LoadFromSlice bulk-loads orders into the cache.
// Duplicates within the input and against existing entries are skipped.
func (c *Cache) LoadFromSlice(orders []*models.Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Trim input to fit within limit
	if c.config.MaxItems > 0 {
		available := c.config.MaxItems - len(c.items)
		if available <= 0 {
			return
		}
		if len(orders) > available {
			orders = orders[:available]
		}
	}

	for _, order := range orders {
		// Skip duplicates (both within input and against existing entries)
		if _, exists := c.items[order.OrderUID]; exists {
			continue
		}

		entry := &cacheEntry{
			order:      order,
			accessedAt: now,
		}
		entry.element = c.lruList.PushBack(order.OrderUID)
		c.items[order.OrderUID] = entry
	}
}

// GetStats returns cache statistics
func (c *Cache) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// GetConfig returns cache configuration
func (c *Cache) GetConfig() config.CacheConfig {
	return c.config
}

// Exists checks if an order is cached without touching the TTL
func (c *Cache) Exists(orderUID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.items[orderUID]
	if !exists {
		return false
	}

	// Check TTL
	if c.config.TTL > 0 && time.Since(entry.accessedAt) > c.config.TTL {
		return false
	}

	return true
}

// Stop terminates the background cleanup goroutine
func (c *Cache) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// evictOldest removes the least recently used cached element
func (c *Cache) evictOldest() {
	oldest := c.lruList.Back()
	if oldest == nil {
		return
	}

	orderUID := oldest.Value.(string)
	if entry, exists := c.items[orderUID]; exists {
		c.removeEntry(orderUID, entry)
		c.stats.Evictions++
		metrics.CacheOps.WithLabelValues("eviction").Inc()
	}
}

// removeEntry removes cached element
func (c *Cache) removeEntry(orderUID string, entry *cacheEntry) {
	c.lruList.Remove(entry.element)
	delete(c.items, orderUID)
}

// cleanupLoop checks for expired elements
func (c *Cache) cleanupLoop(ctx context.Context) {
	interval := c.config.TTL / 2
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanupExpired()
		}
	}
}

// cleanupExpired removes all expired elements
func (c *Cache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.TTL == 0 {
		return
	}

	now := time.Now()
	var toDelete []string

	for orderUID, entry := range c.items {
		if now.Sub(entry.accessedAt) > c.config.TTL {
			toDelete = append(toDelete, orderUID)
		}
	}

	for _, orderUID := range toDelete {
		if entry, exists := c.items[orderUID]; exists {
			c.removeEntry(orderUID, entry)
			c.stats.Expirations++
		}
	}
}
