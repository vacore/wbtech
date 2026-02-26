package mocks

import (
	"context"
	"sync"

	"order-service/internal/models"
	"order-service/internal/repo"
)

// MockRepo implements repo.OrderRepo for testing
type MockRepo struct {
	mu          sync.RWMutex
	orders      map[string]*models.Order
	orderUIDs   []string
	saveError   error
	getError    error
	countError  error
	uidsError   error
	deleteError error
}

func NewMockRepo() *MockRepo {
	return &MockRepo{
		orders:    make(map[string]*models.Order),
		orderUIDs: []string{},
	}
}

func (m *MockRepo) SetSaveError(err error) *MockRepo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveError = err
	return m
}

func (m *MockRepo) SetGetError(err error) *MockRepo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getError = err
	return m
}

func (m *MockRepo) SetCountError(err error) *MockRepo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.countError = err
	return m
}

func (m *MockRepo) SetUIDsError(err error) *MockRepo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uidsError = err
	return m
}

func (m *MockRepo) SetDeleteError(err error) *MockRepo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteError = err
	return m
}

func (m *MockRepo) AddOrder(order *models.Order) *MockRepo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[order.OrderUID] = order
	for _, uid := range m.orderUIDs {
		if uid == order.OrderUID {
			return m
		}
	}
	m.orderUIDs = append(m.orderUIDs, order.OrderUID)
	return m
}

func (m *MockRepo) Reset() *MockRepo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders = make(map[string]*models.Order)
	m.orderUIDs = []string{}
	m.saveError = nil
	m.getError = nil
	m.countError = nil
	m.uidsError = nil
	m.deleteError = nil
	return m
}

func (m *MockRepo) SaveOrder(_ context.Context, order *models.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveError != nil {
		return m.saveError
	}
	isNew := m.orders[order.OrderUID] == nil
	m.orders[order.OrderUID] = order
	if isNew {
		m.orderUIDs = append(m.orderUIDs, order.OrderUID)
	}
	return nil
}

func (m *MockRepo) GetOrder(_ context.Context, orderUID string) (*models.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getError != nil {
		return nil, m.getError
	}
	order, ok := m.orders[orderUID]
	if !ok {
		return nil, repo.ErrOrderNotFound
	}
	return order, nil
}

func (m *MockRepo) DeleteOrder(_ context.Context, orderUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteError != nil {
		return m.deleteError
	}
	if _, ok := m.orders[orderUID]; !ok {
		return repo.ErrOrderNotFound
	}
	delete(m.orders, orderUID)
	for i, uid := range m.orderUIDs {
		if uid == orderUID {
			m.orderUIDs = append(m.orderUIDs[:i], m.orderUIDs[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MockRepo) GetAllOrders(_ context.Context, limit int) ([]*models.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	orders := make([]*models.Order, 0, len(m.orders))
	for _, uid := range m.orderUIDs {
		if order, ok := m.orders[uid]; ok {
			orders = append(orders, order)
			if limit > 0 && len(orders) >= limit {
				break
			}
		}
	}
	return orders, nil
}

func (m *MockRepo) GetOrderUIDs(_ context.Context, limit, offset int, sortAsc bool) ([]repo.OrderListItem, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.uidsError != nil {
		return nil, 0, m.uidsError
	}

	uids := make([]string, len(m.orderUIDs))
	copy(uids, m.orderUIDs)
	if !sortAsc {
		for i, j := 0, len(uids)-1; i < j; i, j = i+1, j-1 {
			uids[i], uids[j] = uids[j], uids[i]
		}
	}

	total := int64(len(uids))
	start := offset
	if start > len(uids) {
		start = len(uids)
	}
	end := start + limit
	if end > len(uids) {
		end = len(uids)
	}

	result := make([]repo.OrderListItem, 0, end-start)
	for _, uid := range uids[start:end] {
		order := m.orders[uid]
		result = append(result, repo.OrderListItem{
			OrderUID:    uid,
			DateCreated: order.DateCreated,
		})
	}
	return result, total, nil
}

func (m *MockRepo) GetOrderCount(_ context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.countError != nil {
		return 0, m.countError
	}
	return int64(len(m.orders)), nil
}

// Test helpers (not part of the interface)

func (m *MockRepo) OrderCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.orders)
}

func (m *MockRepo) HasOrder(uid string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.orders[uid]
	return ok
}

// Ensure MockRepo implements repo.OrderRepo
var _ repo.OrderRepo = (*MockRepo)(nil)
