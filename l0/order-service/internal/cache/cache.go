package cache

import (
	"container/list"
	"context"
	"order-service/internal/models"
	"sync"
	"time"
)

// Config describes cache configuration
type Config struct {
	MaxItems int           // Maximum number of cache items (0 = unlimited)
	TTL      time.Duration // Item's time to live (0 = unlimited)
}

// cacheEntry describes a single cache item's data
type cacheEntry struct {
	order      *models.Order
	lastAccess time.Time     // Last access time (to track LRU)
	createdAt  time.Time     // Creation time (for TTL)
	element    *list.Element // Pointer to item in the LRU list
}

// Cache is a thread-safe LRU cache with TTL
type Cache struct {
	mu      sync.RWMutex           // protect the cache between multi-threads
	items   map[string]*cacheEntry // maps order_uid -> entry
	lruList *list.List             // LRU list
	config  Config                 // cache configuration used
	stats   Stats                  // current cache stats
	cancel  context.CancelFunc
}

// Stats contains cache statistics
type Stats struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	Expirations int64
}

// New creates a new LRU+TTL cache instance with specified configuration
func New(config Config) *Cache {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Cache{
		items:   make(map[string]*cacheEntry),
		lruList: list.New(),
		config:  config,
		cancel:  cancel,
	}

	// If TTL is set - start the background cache cleanup routine
	if config.TTL > 0 {
		go c.cleanupLoop(ctx)
	}

	return c
}

// Set adds or updates a cache item
func (c *Cache) Set(order *models.Order) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check if the order with this uid is present in the cache - update and return
	if entry, exists := c.items[order.OrderUID]; exists {
		// Replace present order with same order_uid with the updated one
		entry.order = order
		// Update access time
		entry.lastAccess = now
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
		lastAccess: now,
		createdAt:  now,
	}

	// Add to the front (this is the most recent now)
	entry.element = c.lruList.PushFront(order.OrderUID)
	c.items[order.OrderUID] = entry
}

// Get returns a cache item from the cache by the hash string
// Second parameter - if the valid item (non-expired) was found
func (c *Cache) Get(orderUID string) (*models.Order, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[orderUID]
	if !exists {
		c.stats.Misses++
		return nil, false
	}

	// Check TTL
	if c.config.TTL > 0 && time.Since(entry.createdAt) > c.config.TTL {
		// The entry has expired - remove it
		c.removeEntry(orderUID, entry)
		c.stats.Expirations++
		c.stats.Misses++
		return nil, false
	}

	// Update access time and make the most recent (move to front of the list)
	entry.lastAccess = time.Now()
	c.lruList.MoveToFront(entry.element)
	c.stats.Hits++

	return entry.order, true
}

// Size returns current cache number of elements
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Delete removes an item from the cache by the hash string
func (c *Cache) Delete(orderUID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.items[orderUID]; exists {
		c.removeEntry(orderUID, entry)
	}
}

// LoadFromSlice adds to the cache from the slice of orders.
// Duplicates within the input and against existing cache entries are skipped.
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
		// Skip duplicates (both within input and against existing cache entries)
		if _, exists := c.items[order.OrderUID]; exists {
			continue
		}

		entry := &cacheEntry{
			order:      order,
			lastAccess: now,
			createdAt:  now,
		}
		entry.element = c.lruList.PushBack(order.OrderUID)
		c.items[order.OrderUID] = entry
	}
}

// GetStats returns the cache statistics
func (c *Cache) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// GetConfig returns the cache configuration
func (c *Cache) GetConfig() Config {
	return c.config
}

// Exists checks if the order is cached (without touching the access time)
func (c *Cache) Exists(orderUID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.items[orderUID]
	if !exists {
		return false
	}

	// Check TTL
	if c.config.TTL > 0 && time.Since(entry.createdAt) > c.config.TTL {
		return false
	}

	return true
}

// evictOldest removes the oldest element in cache (at the end of the LRU)
func (c *Cache) evictOldest() {
	oldest := c.lruList.Back()
	if oldest == nil {
		return
	}

	orderUID := oldest.Value.(string)
	if entry, exists := c.items[orderUID]; exists {
		c.removeEntry(orderUID, entry)
		c.stats.Evictions++
	}
}

// removeEntry removes the element from the cache
func (c *Cache) removeEntry(orderUID string, entry *cacheEntry) {
	c.lruList.Remove(entry.element)
	delete(c.items, orderUID)
}

// Stop terminates the background cleanup goroutine
func (c *Cache) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// cleanupLoop checks for expired elements
func (c *Cache) cleanupLoop(ctx context.Context) {
	interval := c.config.TTL / 2 // Items live at most 1.5xTTL
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return // Clean exit
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
		if now.Sub(entry.createdAt) > c.config.TTL {
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
