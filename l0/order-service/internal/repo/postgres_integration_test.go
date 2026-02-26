//go:build integration

package repo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"order-service/internal/config"
	"order-service/internal/models"
)

// Requires: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME env vars
// Run: go test ./internal/repo -tags=integration -v

func setupTestRepo(t *testing.T) *Repo {
	t.Helper()

	cfg := config.DBConfig{
		Host:     getTestEnv("DB_HOST", "localhost"),
		Port:     getTestEnvInt("DB_PORT", 5432),
		User:     getTestEnv("DB_USER", "orders_user"),
		Password: getTestEnv("DB_PASSWORD", "orders_password"),
		Name:     getTestEnv("DB_NAME", "orders_db"),
		MaxConns: 5,
		MinConns: 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r, err := New(ctx, cfg)
	if err != nil {
		t.Skipf("Skipping integration test - DB not available: %v", err)
	}

	return r
}

func getTestEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getTestEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

func createIntegrationOrder(uid string) *models.Order {
	now := time.Now().Add(-time.Hour)
	return &models.Order{
		OrderUID:        uid,
		TrackNumber:     "WBILTEST12345",
		Entry:           "WBIL",
		CustomerID:      "integration_test",
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

func TestIntegration_SaveAndGetOrder(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	ctx := context.Background()
	uid := "a1b2c3d4e5f6a7b8test"
	order := createIntegrationOrder(uid)

	// Clean up before and after
	_ = r.DeleteOrder(ctx, uid)
	defer r.DeleteOrder(ctx, uid) //nolint:errcheck

	// Save
	err := r.SaveOrder(ctx, order)
	if err != nil {
		t.Fatalf("SaveOrder failed: %v", err)
	}

	// Get
	got, err := r.GetOrder(ctx, uid)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}

	if got.OrderUID != uid {
		t.Errorf("Expected UID %s, got %s", uid, got.OrderUID)
	}
	if got.CustomerID != order.CustomerID {
		t.Errorf("Expected CustomerID %s, got %s", order.CustomerID, got.CustomerID)
	}
	if len(got.Items) != len(order.Items) {
		t.Errorf("Expected %d items, got %d", len(order.Items), len(got.Items))
	}
	if got.Delivery.Name != order.Delivery.Name {
		t.Errorf("Expected delivery name %s, got %s", order.Delivery.Name, got.Delivery.Name)
	}
	if got.Payment.Amount != order.Payment.Amount {
		t.Errorf("Expected amount %d, got %d", order.Payment.Amount, got.Payment.Amount)
	}
}

func TestIntegration_SaveOrder_Upsert(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	ctx := context.Background()
	uid := "b2c3d4e5f6a7b8c9test"
	order := createIntegrationOrder(uid)

	_ = r.DeleteOrder(ctx, uid)
	defer r.DeleteOrder(ctx, uid) //nolint:errcheck

	// First save
	if err := r.SaveOrder(ctx, order); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// Update
	order.CustomerID = "updated_customer"
	order.Delivery.City = "Saint Petersburg"
	order.Items = append(order.Items, models.Item{
		ChrtID:      1111111,
		TrackNumber: "WBILTEST12345",
		Price:       500,
		RID:         "newitemridtest1234",
		Name:        "New Item",
		Sale:        0,
		Size:        "L",
		TotalPrice:  500,
		NmID:        3333333,
		Brand:       "NewBrand",
		Status:      200,
	})

	if err := r.SaveOrder(ctx, order); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Verify
	got, err := r.GetOrder(ctx, uid)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}

	if got.CustomerID != "updated_customer" {
		t.Errorf("Expected updated customer, got %s", got.CustomerID)
	}
	if got.Delivery.City != "Saint Petersburg" {
		t.Errorf("Expected updated city, got %s", got.Delivery.City)
	}
	if len(got.Items) != 2 {
		t.Errorf("Expected 2 items after upsert, got %d", len(got.Items))
	}
}

func TestIntegration_GetOrder_NotFound(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	_, err := r.GetOrder(context.Background(), "nonexistent00000test")
	if err != ErrOrderNotFound {
		t.Errorf("Expected ErrOrderNotFound, got %v", err)
	}
}

func TestIntegration_DeleteOrder(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	ctx := context.Background()
	uid := "c3d4e5f6a7b8c9d0test"
	order := createIntegrationOrder(uid)

	_ = r.DeleteOrder(ctx, uid)

	if err := r.SaveOrder(ctx, order); err != nil {
		t.Fatalf("SaveOrder failed: %v", err)
	}

	if err := r.DeleteOrder(ctx, uid); err != nil {
		t.Fatalf("DeleteOrder failed: %v", err)
	}

	_, err := r.GetOrder(ctx, uid)
	if err != ErrOrderNotFound {
		t.Errorf("Expected ErrOrderNotFound after delete, got %v", err)
	}
}

func TestIntegration_DeleteOrder_NotFound(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	err := r.DeleteOrder(context.Background(), "nonexistent00000test")
	if err != ErrOrderNotFound {
		t.Errorf("Expected ErrOrderNotFound, got %v", err)
	}
}

func TestIntegration_GetAllOrders(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	ctx := context.Background()

	// Create 3 orders with different timestamps
	uids := []string{
		"d4e5f6a7b8c9d0e1test",
		"e5f6a7b8c9d0e1f2test",
		"f6a7b8c9d0e1f2a3test",
	}
	for _, uid := range uids {
		_ = r.DeleteOrder(ctx, uid)
	}
	defer func() {
		for _, uid := range uids {
			_ = r.DeleteOrder(ctx, uid)
		}
	}()

	for i, uid := range uids {
		order := createIntegrationOrder(uid)
		order.DateCreated = time.Now().Add(-time.Duration(len(uids)-i) * time.Minute)
		if err := r.SaveOrder(ctx, order); err != nil {
			t.Fatalf("SaveOrder %s failed: %v", uid, err)
		}
	}

	// Get all (limit 2)
	orders, err := r.GetAllOrders(ctx, 2)
	if err != nil {
		t.Fatalf("GetAllOrders failed: %v", err)
	}

	if len(orders) != 2 {
		t.Errorf("Expected 2 orders, got %d", len(orders))
	}

	// Should be newest first
	if len(orders) >= 2 && orders[0].DateCreated.Before(orders[1].DateCreated) {
		t.Error("Expected newest-first order")
	}
}

func TestIntegration_GetOrderUIDs(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	ctx := context.Background()
	uid := "a7b8c9d0e1f2a3b4test"

	_ = r.DeleteOrder(ctx, uid)
	defer r.DeleteOrder(ctx, uid) //nolint:errcheck

	order := createIntegrationOrder(uid)
	// Ensure this order is the newest so it appears on page 1
	order.DateCreated = time.Now()
	order.Payment.PaymentDT = time.Now().Unix()
	if err := r.SaveOrder(ctx, order); err != nil {
		t.Fatalf("SaveOrder failed: %v", err)
	}

	items, total, err := r.GetOrderUIDs(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetOrderUIDs failed: %v", err)
	}

	if total < 1 {
		t.Errorf("Expected total >= 1, got %d", total)
	}

	found := false
	for _, item := range items {
		if item.OrderUID == uid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find %s in UIDs list (total=%d, got %d items)",
			uid, total, len(items))
	}
}

func TestIntegration_GetOrderCount(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	count, err := r.GetOrderCount(context.Background())
	if err != nil {
		t.Fatalf("GetOrderCount failed: %v", err)
	}

	if count < 0 {
		t.Errorf("Expected non-negative count, got %d", count)
	}
}

func TestIntegration_CascadeDelete(t *testing.T) {
	r := setupTestRepo(t)
	defer r.Close()

	ctx := context.Background()
	uid := "b8c9d0e1f2a3b4c5test"
	order := createIntegrationOrder(uid)

	_ = r.DeleteOrder(ctx, uid)

	if err := r.SaveOrder(ctx, order); err != nil {
		t.Fatalf("SaveOrder failed: %v", err)
	}

	// Delete should cascade to deliveries, payments, items
	if err := r.DeleteOrder(ctx, uid); err != nil {
		t.Fatalf("DeleteOrder failed: %v", err)
	}

	// Verify all related data is gone (re-read returns not found)
	_, err := r.GetOrder(ctx, uid)
	if err != ErrOrderNotFound {
		t.Error("Order and related data should be fully deleted")
	}
}
