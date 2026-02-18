// Package testutil provides shared test utilities and helpers
package testutil

import (
	"context"
	"encoding/json"
	"order-service/internal/models"
	"order-service/internal/repo"
	"sync"
	"time"
)

// OrderBuilder provides a fluent interface for creating test orders
type OrderBuilder struct {
	order *models.Order
}

// NewOrderBuilder creates a new OrderBuilder with sensible defaults
func NewOrderBuilder(uid string) *OrderBuilder {
	now := time.Now().Add(-time.Hour)
	return &OrderBuilder{
		order: &models.Order{
			OrderUID:        uid,
			TrackNumber:     "WBILTEST12345",
			Entry:           "WBIL",
			CustomerID:      "test_customer",
			Locale:          "en",
			DeliveryService: "meest",
			ShardKey:        "9",
			SmID:            99,
			DateCreated:     now,
			OofShard:        "1",
			Delivery: models.Delivery{
				Name:    "Test User",
				Phone:   "+79001234567",
				Zip:     "123456",
				City:    "Moscow",
				Address: "Test Street 1",
				Region:  "Moscow Region",
				Email:   "test@example.com",
			},
			Payment: models.Payment{
				Transaction:  uid,
				RequestID:    "",
				Currency:     "RUB",
				Provider:     "wbpay",
				Amount:       1000,
				PaymentDT:    now.Unix(),
				Bank:         "alpha",
				DeliveryCost: 0,
				GoodsTotal:   1000,
				CustomFee:    0,
			},
			Items: []models.Item{
				{
					ChrtID:      9934930,
					TrackNumber: "WBILTEST12345",
					Price:       1000,
					RID:         "ab4219087a764ae0btest",
					Name:        "Test Item",
					Sale:        0,
					Size:        "M",
					TotalPrice:  1000,
					NmID:        2389212,
					Brand:       "TestBrand",
					Status:      200,
				},
			},
		},
	}
}

func (b *OrderBuilder) WithTrackNumber(track string) *OrderBuilder {
	b.order.TrackNumber = track
	return b
}

func (b *OrderBuilder) WithEntry(entry string) *OrderBuilder {
	b.order.Entry = entry
	return b
}

func (b *OrderBuilder) WithCustomerID(id string) *OrderBuilder {
	b.order.CustomerID = id
	return b
}

func (b *OrderBuilder) WithDelivery(d models.Delivery) *OrderBuilder {
	b.order.Delivery = d
	return b
}

func (b *OrderBuilder) WithDeliveryCost(cost int) *OrderBuilder {
	b.order.Payment.DeliveryCost = cost
	return b
}

func (b *OrderBuilder) WithCustomFee(fee int) *OrderBuilder {
	b.order.Payment.CustomFee = fee
	return b
}

func (b *OrderBuilder) WithLocale(locale string) *OrderBuilder {
	b.order.Locale = locale
	return b
}

func (b *OrderBuilder) WithDeliveryService(service string) *OrderBuilder {
	b.order.DeliveryService = service
	return b
}

func (b *OrderBuilder) WithShardKey(key string) *OrderBuilder {
	b.order.ShardKey = key
	return b
}

func (b *OrderBuilder) WithSmID(id int) *OrderBuilder {
	b.order.SmID = id
	return b
}

func (b *OrderBuilder) WithItems(items []models.Item) *OrderBuilder {
	b.order.Items = items
	return b
}

func (b *OrderBuilder) AddItem(item models.Item) *OrderBuilder {
	b.order.Items = append(b.order.Items, item)
	return b
}

func (b *OrderBuilder) WithDateCreated(t time.Time) *OrderBuilder {
	b.order.DateCreated = t
	b.order.Payment.PaymentDT = t.Unix()
	return b
}

// Build finalizes the order, recalculating totals
func (b *OrderBuilder) Build() *models.Order {
	goodsTotal := 0
	for i := range b.order.Items {
		item := &b.order.Items[i]
		item.TotalPrice = item.Price - (item.Price * item.Sale / 100)
		goodsTotal += item.TotalPrice
	}
	b.order.Payment.GoodsTotal = goodsTotal
	b.order.Payment.Amount = goodsTotal + b.order.Payment.DeliveryCost + b.order.Payment.CustomFee
	return b.order
}

// BuildJSON returns the order as JSON bytes
func (b *OrderBuilder) BuildJSON() []byte {
	order := b.Build()
	data, _ := json.Marshal(order)
	return data
}

// CreateTestOrder creates a simple valid test order
func CreateTestOrder(uid string) *models.Order {
	return NewOrderBuilder(uid).Build()
}

// CreateTestOrderJSON creates a test order and returns JSON bytes
func CreateTestOrderJSON(uid string) []byte {
	return NewOrderBuilder(uid).BuildJSON()
}

// CreateComplexTestOrder creates an order with sale discount and delivery cost
func CreateComplexTestOrder(uid string) *models.Order {
	return NewOrderBuilder(uid).
		WithDeliveryCost(300).
		WithItems([]models.Item{{
			ChrtID:      9934930,
			TrackNumber: "WBILTEST12345",
			Price:       1453,
			RID:         "ab4219087a764ae0btest",
			Name:        "Mascaras",
			Sale:        30,
			Size:        "0",
			NmID:        2389212,
			Brand:       "Vivienne Sabo",
			Status:      202,
		}}).
		Build()
}

// MockRepo is a thread-safe mock for repo.OrderRepo
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

func (m *MockRepo) SaveOrder(ctx context.Context, order *models.Order) error {
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

func (m *MockRepo) GetOrder(ctx context.Context, orderUID string) (*models.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getError != nil {
		return nil, m.getError
	}
	order, ok := m.orders[orderUID]
	if !ok {
		return nil, repo.ErrOrderNotFound // Use the real error!
	}
	return order, nil
}

func (m *MockRepo) DeleteOrder(ctx context.Context, orderUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteError != nil {
		return m.deleteError
	}
	if _, ok := m.orders[orderUID]; !ok {
		return repo.ErrOrderNotFound
	}
	delete(m.orders, orderUID)
	// Remove from UIDs slice
	for i, uid := range m.orderUIDs {
		if uid == orderUID {
			m.orderUIDs = append(m.orderUIDs[:i], m.orderUIDs[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MockRepo) GetAllOrders(ctx context.Context, limit int) ([]*models.Order, error) {
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

func (m *MockRepo) GetOrderUIDs(ctx context.Context, limit, offset int, sortAsc bool) ([]repo.OrderListItem, int64, error) {
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

	total := int64(len(m.orderUIDs))

	// Apply offset
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

func (m *MockRepo) GetOrderCount(ctx context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.countError != nil {
		return 0, m.countError
	}
	return int64(len(m.orders)), nil
}

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
