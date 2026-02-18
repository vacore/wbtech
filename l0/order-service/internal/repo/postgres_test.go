package repo

import (
	"order-service/internal/models"
	"testing"
	"time"
)

// localTestOrder creates a test order without importing testutil
func localTestOrder(uid string) *models.Order {
	now := time.Now().Add(-time.Hour)
	return &models.Order{
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
	}
}

// localMockRepo is a simple mock for repo tests only
type localMockRepo struct {
	orders    map[string]*models.Order
	orderUIDs []string
}

func newLocalMock() *localMockRepo {
	return &localMockRepo{
		orders:    make(map[string]*models.Order),
		orderUIDs: []string{},
	}
}

func (m *localMockRepo) add(order *models.Order) {
	m.orders[order.OrderUID] = order
	m.orderUIDs = append(m.orderUIDs, order.OrderUID)
}

func (m *localMockRepo) get(uid string) (*models.Order, error) {
	order, ok := m.orders[uid]
	if !ok {
		return nil, ErrOrderNotFound
	}
	return order, nil
}

func (m *localMockRepo) count() int {
	return len(m.orders)
}

// Tests (mocky-only)

func TestLocalMock_SaveAndGet(t *testing.T) {
	mock := newLocalMock()
	order := localTestOrder("order_001")

	mock.add(order)

	got, err := mock.get("order_001")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.OrderUID != order.OrderUID {
		t.Errorf("UID mismatch: %s vs %s", order.OrderUID, got.OrderUID)
	}
}

func TestLocalMock_NotFound(t *testing.T) {
	mock := newLocalMock()

	_, err := mock.get("nonexistent")
	if err != ErrOrderNotFound {
		t.Errorf("Expected ErrOrderNotFound, got %v", err)
	}
}

func TestLocalMock_Count(t *testing.T) {
	mock := newLocalMock()

	mock.add(localTestOrder("a"))
	mock.add(localTestOrder("b"))
	mock.add(localTestOrder("c"))

	if mock.count() != 3 {
		t.Errorf("Expected 3, got %d", mock.count())
	}
}

func TestLocalMock_Update(t *testing.T) {
	mock := newLocalMock()

	order := localTestOrder("order_001")
	order.CustomerID = "old"
	mock.add(order)

	order.CustomerID = "new"
	mock.add(order)

	got, _ := mock.get("order_001")
	if got.CustomerID != "new" {
		t.Errorf("Expected 'new', got %s", got.CustomerID)
	}
}

// Test that ErrOrderNotFound works correctly
func TestErrOrderNotFound(t *testing.T) {
	if ErrOrderNotFound.Error() != "order not found" {
		t.Errorf("Wrong error message: %s", ErrOrderNotFound.Error())
	}
}
