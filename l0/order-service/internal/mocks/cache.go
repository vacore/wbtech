package mocks

import (
	"sync"
	"time"

	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/models"
)

// MockCache implements cache.OrderCache for testing
type MockCache struct {
	mu    sync.RWMutex
	items map[string]*models.Order
	stats cache.Stats
}

func NewMockCache() *MockCache {
	return &MockCache{items: make(map[string]*models.Order)}
}

func (m *MockCache) Set(order *models.Order) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[order.OrderUID] = order
}

func (m *MockCache) Get(orderUID string) (*models.Order, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.items[orderUID]
	if ok {
		m.stats.Hits++
	} else {
		m.stats.Misses++
	}
	return o, ok
}

func (m *MockCache) Delete(orderUID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, orderUID)
}

func (m *MockCache) Exists(orderUID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.items[orderUID]
	return ok
}

func (m *MockCache) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func (m *MockCache) LoadFromSlice(orders []*models.Order) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range orders {
		m.items[o.OrderUID] = o
	}
}

func (m *MockCache) GetStats() cache.Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

func (m *MockCache) GetConfig() config.CacheConfig {
	return config.CacheConfig{MaxItems: 100, TTL: time.Hour}
}

func (m *MockCache) Stop() {}

// Ensure MockCache implements cache.OrderCache
var _ cache.OrderCache = (*MockCache)(nil)
