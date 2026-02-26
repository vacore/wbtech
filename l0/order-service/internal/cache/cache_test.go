package cache

import (
	"sync"
	"testing"
	"time"

	"order-service/internal/config"
	"order-service/internal/models"
	"order-service/internal/testutil"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		config config.CacheConfig
	}{
		{"custom config", config.CacheConfig{MaxItems: 100, TTL: 5 * time.Minute}},
		{"no limits", config.CacheConfig{MaxItems: 0, TTL: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.config)
			if c == nil {
				t.Fatal("New() returned nil")
			}
			if c.Size() != 0 {
				t.Errorf("New cache should be empty, got size %d", c.Size())
			}
		})
	}
}

func TestCache_SetAndGet(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: time.Hour})
	defer c.Stop()

	order := testutil.CreateTestOrder("order_001")
	c.Set(order)

	got, found := c.Get("order_001")
	if !found {
		t.Fatal("Expected to find order")
	}
	if got.OrderUID != order.OrderUID {
		t.Errorf("Expected %s, got %s", order.OrderUID, got.OrderUID)
	}

	_, found = c.Get("nonexistent")
	if found {
		t.Error("Should not find nonexistent key")
	}
}

func TestCache_SetUpdatesExisting(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: time.Hour})
	defer c.Stop()

	order1 := testutil.NewOrderBuilder("order_001").WithCustomerID("old").Build()
	c.Set(order1)

	order2 := testutil.NewOrderBuilder("order_001").WithCustomerID("new").Build()
	c.Set(order2)

	if c.Size() != 1 {
		t.Errorf("Expected size=1, got %d", c.Size())
	}
	got, _ := c.Get("order_001")
	if got.CustomerID != "new" {
		t.Errorf("Expected CustomerID=new, got %s", got.CustomerID)
	}
}

func TestCache_LRUEviction(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 3, TTL: time.Hour})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))
	c.Set(testutil.CreateTestOrder("order_002"))
	c.Set(testutil.CreateTestOrder("order_003"))

	c.Get("order_001") // access to make recent

	c.Set(testutil.CreateTestOrder("order_004"))

	if c.Exists("order_002") {
		t.Error("order_002 should be evicted")
	}
	if !c.Exists("order_001") {
		t.Error("order_001 should exist")
	}
}

func TestCache_TTLExpiration(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: 50 * time.Millisecond})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	time.Sleep(100 * time.Millisecond)

	_, found := c.Get("order_001")
	if found {
		t.Error("Order should have expired")
	}
}

func TestCache_Delete(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: time.Hour})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))
	c.Set(testutil.CreateTestOrder("order_002"))

	c.Delete("order_001")

	if c.Exists("order_001") {
		t.Error("order_001 should be deleted")
	}
	if !c.Exists("order_002") {
		t.Error("order_002 should exist")
	}
}

func TestCache_LoadFromSlice(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 5, TTL: time.Hour})
	defer c.Stop()

	orders := []*models.Order{
		testutil.CreateTestOrder("a"),
		testutil.CreateTestOrder("b"),
	}
	c.LoadFromSlice(orders)

	if c.Size() != 2 {
		t.Errorf("Expected size=2, got %d", c.Size())
	}
}

func TestCache_LoadFromSliceRespectsLimit(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 2, TTL: time.Hour})
	defer c.Stop()

	orders := []*models.Order{
		testutil.CreateTestOrder("a"),
		testutil.CreateTestOrder("b"),
		testutil.CreateTestOrder("c"),
	}
	c.LoadFromSlice(orders)

	if c.Size() != 2 {
		t.Errorf("Expected size=2, got %d", c.Size())
	}
}

func TestCache_LoadFromSlice_RespectsLimitWithExistingItems(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 3, TTL: time.Hour})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("existing_1"))
	c.Set(testutil.CreateTestOrder("existing_2"))

	orders := []*models.Order{
		testutil.CreateTestOrder("new_1"),
		testutil.CreateTestOrder("new_2"),
		testutil.CreateTestOrder("new_3"),
	}
	c.LoadFromSlice(orders)

	if c.Size() > 3 {
		t.Errorf("Cache should not exceed MaxItems=3, got %d", c.Size())
	}
}

func TestCache_LoadFromSlice_SkipsDuplicatesInInput(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: time.Hour})
	defer c.Stop()

	orders := []*models.Order{
		testutil.CreateTestOrder("a"),
		testutil.CreateTestOrder("a"), // duplicate within input
		testutil.CreateTestOrder("b"),
	}
	c.LoadFromSlice(orders)

	if c.Size() != 2 {
		t.Errorf("Expected size=2 (duplicate skipped), got %d", c.Size())
	}
}

func TestCache_LoadFromSlice_SkipsExistingEntries(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: time.Hour})
	defer c.Stop()

	// Pre-populate cache
	existing := testutil.NewOrderBuilder("a").WithCustomerID("original").Build()
	c.Set(existing)

	// Load slice containing a duplicate UID
	orders := []*models.Order{
		testutil.NewOrderBuilder("a").WithCustomerID("from_slice").Build(),
		testutil.CreateTestOrder("b"),
	}
	c.LoadFromSlice(orders)

	if c.Size() != 2 {
		t.Errorf("Expected size=2, got %d", c.Size())
	}

	// Existing entry should be preserved, not overwritten
	got, _ := c.Get("a")
	if got.CustomerID != "original" {
		t.Errorf("Expected original entry preserved, got CustomerID=%s", got.CustomerID)
	}
}

func TestCache_Stats(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: time.Hour})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	c.Get("order_001") // hit
	c.Get("order_001") // hit
	c.Get("missing")   // miss

	stats := c.GetStats()
	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
}

func TestCache_ExistsDoesNotUpdateLRU(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 2, TTL: time.Hour})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))
	c.Set(testutil.CreateTestOrder("order_002"))

	c.Exists("order_001") // should NOT update LRU

	c.Set(testutil.CreateTestOrder("order_003"))

	if c.Exists("order_001") {
		t.Error("order_001 should be evicted")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 100, TTL: time.Hour})
	defer c.Stop()

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Set(testutil.CreateTestOrder("concurrent"))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Get("concurrent")
				c.Size()
			}
		}()
	}
	wg.Wait()
}

func TestCache_Get_ExpirationIncrementsStats(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: 50 * time.Millisecond})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	time.Sleep(60 * time.Millisecond)

	_, found := c.Get("order_001")
	if found {
		t.Error("Should not find expired order")
	}

	stats := c.GetStats()
	if stats.Expirations != 1 {
		t.Errorf("Expected 1 expiration, got %d", stats.Expirations)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
}

func TestCache_Exists_ReturnsFalseForExpired(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: 50 * time.Millisecond})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	time.Sleep(60 * time.Millisecond)

	if c.Exists("order_001") {
		t.Error("Expired item should not exist")
	}
}

func TestCache_EvictOldest_EmptyCache(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 1, TTL: time.Hour})
	defer c.Stop()

	// Should not panic on empty cache
	c.mu.Lock()
	c.evictOldest()
	c.mu.Unlock()

	if c.Size() != 0 {
		t.Errorf("Expected size=0, got %d", c.Size())
	}
}

func TestCache_Stop(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: time.Minute})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	// Should not panic, even called twice
	c.Stop()
	c.Stop()

	// Cache still functional after stop (just no background cleanup)
	got, found := c.Get("order_001")
	if !found {
		t.Error("Cache should still work after Stop")
	}
	if got.OrderUID != "order_001" {
		t.Errorf("Expected order_001, got %s", got.OrderUID)
	}
}

func TestCache_Stop_NilCancel(t *testing.T) {
	// Cache with TTL=0 doesn't start cleanup goroutine,
	// but cancel is still set. Test that Stop is safe regardless.
	c := New(config.CacheConfig{MaxItems: 10, TTL: 0})

	c.Stop() // should not panic
}

func TestCache_CleanupExpired_NoTTL(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: 0})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	c.cleanupExpired()

	if c.Size() != 1 {
		t.Error("Should not expire anything when TTL=0")
	}
}

func TestCache_CleanupExpired_RemovesExpired(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: 50 * time.Millisecond})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("old_1"))
	c.Set(testutil.CreateTestOrder("old_2"))

	time.Sleep(60 * time.Millisecond)

	c.Set(testutil.CreateTestOrder("fresh"))

	c.cleanupExpired()

	if c.Size() != 1 {
		t.Errorf("Expected 1 after cleanup, got %d", c.Size())
	}
	if !c.Exists("fresh") {
		t.Error("Fresh order should still exist")
	}

	stats := c.GetStats()
	if stats.Expirations != 2 {
		t.Errorf("Expected 2 expirations, got %d", stats.Expirations)
	}
}

func TestCache_EvictionIncrementsStats(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 2, TTL: time.Hour})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("a"))
	c.Set(testutil.CreateTestOrder("b"))
	c.Set(testutil.CreateTestOrder("c")) // evicts "a"

	stats := c.GetStats()
	if stats.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestCache_TTLSlidingWindow(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: 80 * time.Millisecond})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	// Access at 50ms - resets TTL window
	time.Sleep(50 * time.Millisecond)
	_, found := c.Get("order_001")
	if !found {
		t.Fatal("Should still be alive at 50ms")
	}

	// At 100ms total (50ms since last access) - still alive
	time.Sleep(50 * time.Millisecond)
	_, found = c.Get("order_001")
	if !found {
		t.Fatal("Should still be alive - only 50ms since last access")
	}

	// Wait full TTL without any access
	time.Sleep(90 * time.Millisecond)
	_, found = c.Get("order_001")
	if found {
		t.Error("Should have expired after 90ms of inactivity")
	}
}

func TestCache_Set_ResetsTTL(t *testing.T) {
	c := New(config.CacheConfig{MaxItems: 10, TTL: 80 * time.Millisecond})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("order_001"))

	// Wait 50ms, then update via Set - should reset TTL
	time.Sleep(50 * time.Millisecond)
	c.Set(testutil.CreateTestOrder("order_001"))

	// Wait another 50ms (100ms total, but only 50ms since Set)
	time.Sleep(50 * time.Millisecond)
	_, found := c.Get("order_001")
	if !found {
		t.Error("Set should reset TTL - order should still be alive")
	}
}

// Benchmarks

func BenchmarkCache_Set(b *testing.B) {
	c := New(config.CacheConfig{MaxItems: 10000, TTL: time.Hour})
	defer c.Stop()

	order := testutil.CreateTestOrder("bench")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(order)
	}
}

func BenchmarkCache_Get(b *testing.B) {
	c := New(config.CacheConfig{MaxItems: 10000, TTL: time.Hour})
	defer c.Stop()

	c.Set(testutil.CreateTestOrder("bench"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("bench")
	}
}
