package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Helper to create a valid order for testing
func createValidOrder() *Order {
	return &Order{
		OrderUID:        "a1b2c3d4e5f6a7b8test",
		TrackNumber:     "WBILMTESTTRACK",
		Entry:           "WBIL",
		CustomerID:      "test_customer",
		Locale:          "en",
		DeliveryService: "meest",
		ShardKey:        "9",
		SmID:            99,
		DateCreated:     time.Now().Add(-time.Hour),
		OofShard:        "1",
		Delivery: Delivery{
			Name:    "Test User",
			Phone:   "+79001234567",
			Zip:     "123456",
			City:    "Moscow",
			Address: "Test Street 1",
			Region:  "Moscow Region",
			Email:   "test@example.com",
		},
		Payment: Payment{
			Transaction:  "a1b2c3d4e5f6a7b8test",
			RequestID:    "",
			Currency:     "RUB",
			Provider:     "wbpay",
			Amount:       1318,
			PaymentDT:    time.Now().Add(-time.Hour).Unix(),
			Bank:         "alpha",
			DeliveryCost: 300,
			GoodsTotal:   1018,
			CustomFee:    0,
		},
		Items: []Item{
			{
				ChrtID:      9934930,
				TrackNumber: "WBILMTESTTRACK",
				Price:       1453,
				RID:         "ab4219087a764ae0btest",
				Name:        "Mascaras",
				Sale:        30,
				Size:        "0",
				TotalPrice:  1018,
				NmID:        2389212,
				Brand:       "Vivienne Sabo",
				Status:      202,
			},
		},
	}
}

func TestOrder_Validate_ValidOrder(t *testing.T) {
	order := createValidOrder()
	err := order.Validate()
	if err != nil {
		t.Errorf("Valid order should not have errors: %v", err)
	}
}

func TestOrder_Validate_OrderUID(t *testing.T) {
	tests := []struct {
		name      string
		orderUID  string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid 16 hex",
			orderUID:  "a1b2c3d4e5f6a7b8",
			wantError: false,
		},
		{
			name:      "valid 16 hex with test suffix",
			orderUID:  "a1b2c3d4e5f6a7b8test",
			wantError: false,
		},
		{
			name:      "empty",
			orderUID:  "",
			wantError: true,
			errorMsg:  "order_uid",
		},
		{
			name:      "too short",
			orderUID:  "a1b2c3",
			wantError: true,
			errorMsg:  "order_uid",
		},
		{
			name:      "invalid characters",
			orderUID:  "a1b2c3d4e5f6a7bX",
			wantError: true,
			errorMsg:  "order_uid",
		},
		{
			name:      "uppercase hex",
			orderUID:  "A1B2C3D4E5F6A7B8",
			wantError: true,
			errorMsg:  "order_uid",
		},
		{
			name:      "with wrong suffix",
			orderUID:  "a1b2c3d4e5f6a7b8prod",
			wantError: true,
			errorMsg:  "order_uid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.OrderUID = tt.orderUID
			// Update payment transaction to match
			order.Payment.Transaction = tt.orderUID

			err := order.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Expected validation error")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_TrackNumber(t *testing.T) {
	tests := []struct {
		name        string
		trackNumber string
		wantError   bool
	}{
		{"valid", "WBILMTESTTRACK", false},
		{"min length (5)", "TRACK", false},
		{"max length (30)", "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234", false},
		{"empty", "", true},
		{"too short", "TRCK", true},
		{"too long", "ABCDEFGHIJKLMNOPQRSTUVWXYZ12345", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.TrackNumber = tt.trackNumber
			order.Items[0].TrackNumber = tt.trackNumber

			err := order.Validate()
			if tt.wantError && err == nil {
				t.Error("Expected validation error")
			} else if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_Entry(t *testing.T) {
	tests := []struct {
		name      string
		entry     string
		wantError bool
	}{
		{"WBIL", "WBIL", false},
		{"WBRU", "WBRU", false},
		{"WBKZ", "WBKZ", false},
		{"WBBY", "WBBY", false},
		{"empty", "", true},
		{"invalid", "INVALID", true},
		{"lowercase", "wbil", false}, // contains is case-insensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.Entry = tt.entry

			err := order.Validate()
			if tt.wantError && err == nil {
				t.Error("Expected validation error")
			} else if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_CustomerID(t *testing.T) {
	order := createValidOrder()
	order.CustomerID = ""

	err := order.Validate()
	if err == nil {
		t.Error("Expected error for empty customer_id")
	}
	if !strings.Contains(err.Error(), "customer_id") {
		t.Errorf("Expected error about customer_id, got: %v", err)
	}
}

func TestOrder_Validate_Locale(t *testing.T) {
	tests := []struct {
		name      string
		locale    string
		wantError bool
	}{
		{"valid en", "en", false},
		{"valid ru", "ru", false},
		{"valid en_US", "en_US", false},
		{"empty (optional)", "", false},
		{"invalid format", "english", true},
		{"invalid with number", "e1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.Locale = tt.locale

			err := order.Validate()
			if tt.wantError && err == nil {
				t.Error("Expected validation error")
			} else if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_ShardKey(t *testing.T) {
	tests := []struct {
		name      string
		shardKey  string
		wantError bool
	}{
		{"valid 0", "0", false},
		{"valid 9", "9", false},
		{"valid 5", "5", false},
		{"empty (optional)", "", false},
		{"two digits", "12", true},
		{"letter", "a", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.ShardKey = tt.shardKey

			err := order.Validate()
			if tt.wantError && err == nil {
				t.Error("Expected validation error")
			} else if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_SmID(t *testing.T) {
	tests := []struct {
		name      string
		smID      int
		wantError bool
	}{
		{"valid 0", 0, false},
		{"valid 99", 99, false},
		{"valid 999", 999, false},
		{"negative", -1, true},
		{"too large", 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.SmID = tt.smID

			err := order.Validate()
			if tt.wantError && err == nil {
				t.Error("Expected validation error")
			} else if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_DateCreated(t *testing.T) {
	tests := []struct {
		name        string
		dateCreated time.Time
		wantError   bool
	}{
		{"valid past", time.Now().Add(-time.Hour), false},
		{"valid now", time.Now(), false},
		{"zero", time.Time{}, true},
		{"future", time.Now().Add(2 * time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.DateCreated = tt.dateCreated

			err := order.Validate()
			if tt.wantError && err == nil {
				t.Error("Expected validation error")
			} else if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_DeliveryService(t *testing.T) {
	tests := []struct {
		name            string
		deliveryService string
		wantError       bool
	}{
		{"valid meest", "meest", false},
		{"valid dpd", "dpd", false},
		{"valid uppercase", "DPD", false},
		{"empty (optional)", "", false},
		{"invalid", "unknown_service", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := createValidOrder()
			order.DeliveryService = tt.deliveryService

			err := order.Validate()
			if tt.wantError && err == nil {
				t.Error("Expected validation error")
			} else if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestOrder_Validate_NoItems(t *testing.T) {
	order := createValidOrder()
	order.Items = []Item{}
	order.Payment.GoodsTotal = 0
	order.Payment.Amount = order.Payment.DeliveryCost

	err := order.Validate()
	if err == nil {
		t.Error("Expected error for empty items")
	}
	if !strings.Contains(err.Error(), "items") {
		t.Errorf("Expected error about items, got: %v", err)
	}
}

func TestOrder_Validate_GoodsTotalMismatch(t *testing.T) {
	order := createValidOrder()
	order.Payment.GoodsTotal = 9999 // Wrong total

	err := order.Validate()
	if err == nil {
		t.Error("Expected error for goods_total mismatch")
	}
	if !strings.Contains(err.Error(), "goods_total") {
		t.Errorf("Expected error about goods_total, got: %v", err)
	}
}

func TestOrder_Validate_AmountMismatch(t *testing.T) {
	order := createValidOrder()
	order.Payment.Amount = 9999 // Wrong amount

	err := order.Validate()
	if err == nil {
		t.Error("Expected error for amount mismatch")
	}
	if !strings.Contains(err.Error(), "amount") {
		t.Errorf("Expected error about amount, got: %v", err)
	}
}

// Delivery validation tests

func TestDelivery_Validate_Name(t *testing.T) {
	tests := []struct {
		name      string
		dName     string
		wantError bool
	}{
		{"valid", "John Doe", false},
		{"min length (2)", "JD", false},
		{"unicode", "Иван Петров", false},
		{"empty", "", true},
		{"too short", "J", true},
		{"too long", strings.Repeat("a", 101), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Delivery{
				Name:    tt.dName,
				City:    "Moscow",
				Address: "Test Street",
			}
			errs := d.Validate()
			if tt.wantError && len(errs) == 0 {
				t.Error("Expected validation error")
			} else if !tt.wantError && len(errs) > 0 {
				t.Errorf("Unexpected errors: %v", errs)
			}
		})
	}
}

func TestDelivery_Validate_Phone(t *testing.T) {
	tests := []struct {
		name      string
		phone     string
		wantError bool
	}{
		{"valid RU", "+79001234567", false},
		{"valid US", "+12025551234", false},
		{"valid short", "+1234567", false},
		{"empty (optional)", "", false},
		{"no plus", "79001234567", true},
		{"too short", "+12345", true},
		{"with letters", "+7900123456a", true},
		{"with spaces", "+7 900 123 4567", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Delivery{
				Name:    "Test User",
				Phone:   tt.phone,
				City:    "Moscow",
				Address: "Test Street",
			}
			errs := d.Validate()
			hasPhoneError := false
			for _, e := range errs {
				if strings.Contains(e.Field, "phone") {
					hasPhoneError = true
					break
				}
			}
			if tt.wantError && !hasPhoneError {
				t.Error("Expected phone validation error")
			} else if !tt.wantError && hasPhoneError {
				t.Error("Unexpected phone validation error")
			}
		})
	}
}

func TestDelivery_Validate_Email(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		wantError bool
	}{
		{"valid", "test@example.com", false},
		{"valid with subdomain", "test@mail.example.com", false},
		{"valid with plus", "test+tag@example.com", false},
		{"empty (optional)", "", false},
		{"no @", "testexample.com", true},
		{"no domain", "test@", true},
		{"no local part", "@example.com", true},
		{"spaces", "test @example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Delivery{
				Name:    "Test User",
				Email:   tt.email,
				City:    "Moscow",
				Address: "Test Street",
			}
			errs := d.Validate()
			hasEmailError := false
			for _, e := range errs {
				if strings.Contains(e.Field, "email") {
					hasEmailError = true
					break
				}
			}
			if tt.wantError && !hasEmailError {
				t.Error("Expected email validation error")
			} else if !tt.wantError && hasEmailError {
				t.Error("Unexpected email validation error")
			}
		})
	}
}

func TestDelivery_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		delivery  Delivery
		errorMsgs []string
	}{
		{
			name:      "missing city",
			delivery:  Delivery{Name: "Test", Address: "Street"},
			errorMsgs: []string{"city"},
		},
		{
			name:      "missing address",
			delivery:  Delivery{Name: "Test", City: "Moscow"},
			errorMsgs: []string{"address"},
		},
		{
			name:      "missing both",
			delivery:  Delivery{Name: "Test"},
			errorMsgs: []string{"city", "address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.delivery.Validate()
			for _, msg := range tt.errorMsgs {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Field, msg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing %q", msg)
				}
			}
		})
	}
}

// Payment validation tests

func TestPayment_Validate_Currency(t *testing.T) {
	tests := []struct {
		name      string
		currency  string
		wantError bool
	}{
		{"valid RUB", "RUB", false},
		{"valid USD", "USD", false},
		{"valid EUR", "EUR", false},
		{"empty", "", true},
		{"lowercase", "rub", true},
		{"invalid code", "XXX", true},
		{"too long", "USDD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Payment{
				Transaction: "test",
				Currency:    tt.currency,
				PaymentDT:   time.Now().Unix(),
			}
			errs := p.Validate()
			hasCurrencyError := false
			for _, e := range errs {
				if strings.Contains(e.Field, "currency") {
					hasCurrencyError = true
					break
				}
			}
			if tt.wantError && !hasCurrencyError {
				t.Error("Expected currency validation error")
			} else if !tt.wantError && hasCurrencyError {
				t.Error("Unexpected currency validation error")
			}
		})
	}
}

func TestPayment_Validate_Provider(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		wantError bool
	}{
		{"valid wbpay", "wbpay", false},
		{"valid stripe", "stripe", false},
		{"valid uppercase", "WBPAY", false},
		{"empty (optional)", "", false},
		{"invalid", "unknown_provider", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Payment{
				Transaction: "test",
				Currency:    "RUB",
				Provider:    tt.provider,
				PaymentDT:   time.Now().Unix(),
			}
			errs := p.Validate()
			hasProviderError := false
			for _, e := range errs {
				if strings.Contains(e.Field, "provider") {
					hasProviderError = true
					break
				}
			}
			if tt.wantError && !hasProviderError {
				t.Error("Expected provider validation error")
			} else if !tt.wantError && hasProviderError {
				t.Error("Unexpected provider validation error")
			}
		})
	}
}

func TestPayment_Validate_NegativeAmounts(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		amount    int
		delivery  int
		goods     int
		custom    int
		wantError bool
	}{
		{"negative amount", "amount", -100, 0, 0, 0, true},
		{"negative delivery_cost", "delivery_cost", 0, -100, 0, 0, true},
		{"negative goods_total", "goods_total", 0, 0, -100, 0, true},
		{"negative custom_fee", "custom_fee", 0, 0, 0, -100, true},
		{"all zero", "", 0, 0, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Payment{
				Transaction:  "test",
				Currency:     "RUB",
				Amount:       tt.amount,
				DeliveryCost: tt.delivery,
				GoodsTotal:   tt.goods,
				CustomFee:    tt.custom,
				PaymentDT:    time.Now().Unix(),
			}
			errs := p.Validate()
			if tt.wantError && len(errs) == 0 {
				t.Error("Expected validation error")
			}
		})
	}
}

func TestPayment_Validate_PaymentDT(t *testing.T) {
	tests := []struct {
		name      string
		paymentDT int64
		wantError bool
	}{
		{"valid past", time.Now().Add(-time.Hour).Unix(), false},
		{"valid now", time.Now().Unix(), false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"future", time.Now().Add(2 * time.Hour).Unix(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Payment{
				Transaction: "test",
				Currency:    "RUB",
				PaymentDT:   tt.paymentDT,
			}
			errs := p.Validate()
			hasPaymentDTError := false
			for _, e := range errs {
				if strings.Contains(e.Field, "payment_dt") {
					hasPaymentDTError = true
					break
				}
			}
			if tt.wantError && !hasPaymentDTError {
				t.Error("Expected payment_dt validation error")
			} else if !tt.wantError && hasPaymentDTError {
				t.Error("Unexpected payment_dt validation error")
			}
		})
	}
}

// Item validation tests

func TestItem_Validate(t *testing.T) {
	tests := []struct {
		name      string
		item      Item
		wantError bool
		errorMsgs []string
	}{
		{
			name: "valid item",
			item: Item{
				ChrtID:     12345,
				Name:       "Test Item",
				Price:      1000,
				Sale:       10,
				TotalPrice: 900,
				NmID:       67890,
				Brand:      "TestBrand",
				Status:     200,
			},
			wantError: false,
		},
		{
			name: "negative chrt_id",
			item: Item{
				ChrtID:     -1,
				Name:       "Test",
				Price:      100,
				TotalPrice: 100,
				NmID:       1,
				Brand:      "Test",
				Status:     200,
			},
			wantError: true,
			errorMsgs: []string{"chrt_id"},
		},
		{
			name: "empty name",
			item: Item{
				ChrtID:     1,
				Name:       "",
				Price:      100,
				TotalPrice: 100,
				NmID:       1,
				Brand:      "Test",
				Status:     200,
			},
			wantError: true,
			errorMsgs: []string{"name"},
		},
		{
			name: "negative price",
			item: Item{
				ChrtID:     1,
				Name:       "Test",
				Price:      -100,
				TotalPrice: 100,
				NmID:       1,
				Brand:      "Test",
				Status:     200,
			},
			wantError: true,
			errorMsgs: []string{"price"},
		},
		{
			name: "sale over 100",
			item: Item{
				ChrtID:     1,
				Name:       "Test",
				Price:      100,
				Sale:       101,
				TotalPrice: 100,
				NmID:       1,
				Brand:      "Test",
				Status:     200,
			},
			wantError: true,
			errorMsgs: []string{"sale"},
		},
		{
			name: "negative sale",
			item: Item{
				ChrtID:     1,
				Name:       "Test",
				Price:      100,
				Sale:       -10,
				TotalPrice: 100,
				NmID:       1,
				Brand:      "Test",
				Status:     200,
			},
			wantError: true,
			errorMsgs: []string{"sale"},
		},
		{
			name: "total_price mismatch",
			item: Item{
				ChrtID:     1,
				Name:       "Test",
				Price:      1000,
				Sale:       10,
				TotalPrice: 500, // Should be 900
				NmID:       1,
				Brand:      "Test",
				Status:     200,
			},
			wantError: true,
			errorMsgs: []string{"total_price"},
		},
		{
			name: "empty brand",
			item: Item{
				ChrtID:     1,
				Name:       "Test",
				Price:      100,
				TotalPrice: 100,
				NmID:       1,
				Brand:      "",
				Status:     200,
			},
			wantError: true,
			errorMsgs: []string{"brand"},
		},
		{
			name: "invalid status",
			item: Item{
				ChrtID:     1,
				Name:       "Test",
				Price:      100,
				TotalPrice: 100,
				NmID:       1,
				Brand:      "Test",
				Status:     600,
			},
			wantError: true,
			errorMsgs: []string{"status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.item.Validate(0)
			if tt.wantError && len(errs) == 0 {
				t.Error("Expected validation error")
			} else if !tt.wantError && len(errs) > 0 {
				t.Errorf("Unexpected errors: %v", errs)
			}

			for _, msg := range tt.errorMsgs {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Field, msg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing %q, got %v", msg, errs)
				}
			}
		})
	}
}

// ParseOrder tests

func TestParseOrder_ValidJSON(t *testing.T) {
	order := createValidOrder()
	data, _ := json.Marshal(order)

	parsed, err := ParseOrder(data)
	if err != nil {
		t.Fatalf("ParseOrder failed: %v", err)
	}

	if parsed.OrderUID != order.OrderUID {
		t.Errorf("OrderUID mismatch: expected %s, got %s", order.OrderUID, parsed.OrderUID)
	}
	if len(parsed.Items) != len(order.Items) {
		t.Errorf("Items count mismatch: expected %d, got %d", len(order.Items), len(parsed.Items))
	}
}

func TestParseOrder_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid json}`)

	_, err := ParseOrder(data)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("Expected 'invalid JSON' error, got: %v", err)
	}
}

func TestParseOrder_ValidationError(t *testing.T) {
	// Valid JSON but invalid order (empty order_uid)
	data := []byte(`{
		"order_uid": "",
		"track_number": "TEST",
		"entry": "WBIL",
		"customer_id": "test",
		"delivery": {"name": "Test", "city": "Moscow", "address": "Street"},
		"payment": {"transaction": "test", "currency": "RUB", "payment_dt": 1637907727},
		"items": [],
		"date_created": "2024-01-01T00:00:00Z"
	}`)

	_, err := ParseOrder(data)
	if err == nil {
		t.Error("Expected validation error")
	}
}

// ValidationError tests

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Field:   "order_uid",
		Message: "is required",
	}

	expected := "order_uid: is required"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestValidationErrors_Error(t *testing.T) {
	errs := ValidationErrors{
		{Field: "order_uid", Message: "is required"},
		{Field: "track_number", Message: "is too short"},
	}

	result := errs.Error()
	if !strings.Contains(result, "order_uid: is required") {
		t.Error("Expected error to contain order_uid error")
	}
	if !strings.Contains(result, "track_number: is too short") {
		t.Error("Expected error to contain track_number error")
	}
}

func TestValidationErrors_EmptyError(t *testing.T) {
	var errs ValidationErrors
	if errs.Error() != "" {
		t.Errorf("Expected empty string, got %q", errs.Error())
	}
}

// Helper function tests

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	tests := []struct {
		item     string
		expected bool
	}{
		{"apple", true},
		{"APPLE", true}, // case insensitive
		{"Apple", true}, // case insensitive
		{"banana", true},
		{"grape", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.item, func(t *testing.T) {
			result := contains(slice, tt.item)
			if result != tt.expected {
				t.Errorf("contains(%v, %q) = %v, want %v", slice, tt.item, result, tt.expected)
			}
		})
	}
}

// Propagation through Order.Validate()

func TestOrder_Validate_PropagatesDeliveryErrors(t *testing.T) {
	order := createValidOrder()
	order.Delivery.Name = "" // Required field
	order.Delivery.City = "" // Required field

	err := order.Validate()
	if err == nil {
		t.Fatal("Expected validation error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "delivery.name") {
		t.Error("Expected delivery.name error to propagate")
	}
	if !strings.Contains(errStr, "delivery.city") {
		t.Error("Expected delivery.city error to propagate")
	}
}

func TestOrder_Validate_PropagatesItemErrors(t *testing.T) {
	order := createValidOrder()
	order.Items[0].Name = ""  // Required
	order.Items[0].Brand = "" // Required

	err := order.Validate()
	if err == nil {
		t.Fatal("Expected validation error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "items[0].name") {
		t.Error("Expected items[0].name error to propagate")
	}
	if !strings.Contains(errStr, "items[0].brand") {
		t.Error("Expected items[0].brand error to propagate")
	}
}

func TestOrder_Validate_MultipleItemErrors(t *testing.T) {
	order := createValidOrder()
	// Add a second invalid item
	order.Items = append(order.Items, Item{
		ChrtID:     -1,
		Name:       "",
		Price:      -100,
		Sale:       0,
		TotalPrice: -100,
		NmID:       0,
		Brand:      "",
		Status:     200,
	})

	err := order.Validate()
	if err == nil {
		t.Fatal("Expected validation errors")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "items[1]") {
		t.Error("Expected errors referencing items[1]")
	}
}

// Payment Bank Validation

func TestPayment_Validate_InvalidBank(t *testing.T) {
	p := &Payment{
		Transaction: "test",
		Currency:    "RUB",
		PaymentDT:   time.Now().Unix(),
		Bank:        "unknown_bank",
	}

	errs := p.Validate()
	hasBankError := false
	for _, e := range errs {
		if strings.Contains(e.Field, "bank") {
			hasBankError = true
			break
		}
	}
	if !hasBankError {
		t.Error("Expected bank validation error for unknown bank")
	}
}

func TestPayment_Validate_ValidBanks(t *testing.T) {
	validBanksList := []string{"alpha", "sber", "tinkoff", "vtb", "raiffeisen", "chase", "hsbc"}

	for _, bank := range validBanksList {
		t.Run(bank, func(t *testing.T) {
			p := &Payment{
				Transaction: "test",
				Currency:    "RUB",
				PaymentDT:   time.Now().Unix(),
				Bank:        bank,
			}
			errs := p.Validate()
			for _, e := range errs {
				if strings.Contains(e.Field, "bank") {
					t.Errorf("Bank %q should be valid, got error: %s", bank, e.Message)
				}
			}
		})
	}
}

// Item Edge Cases

func TestItem_Validate_NegativeTotalPrice(t *testing.T) {
	item := Item{
		ChrtID:     12345,
		Name:       "Test",
		Price:      100,
		Sale:       0,
		TotalPrice: -50, // Negative, also mismatches expected (100)
		NmID:       12345,
		Brand:      "Test",
		Status:     200,
	}

	errs := item.Validate(0)
	hasTotalPriceError := false
	for _, e := range errs {
		if strings.Contains(e.Field, "total_price") {
			hasTotalPriceError = true
			break
		}
	}
	if !hasTotalPriceError {
		t.Error("Expected total_price validation error")
	}
}

func TestItem_Validate_ZeroNmID(t *testing.T) {
	item := Item{
		ChrtID:     12345,
		Name:       "Test",
		Price:      100,
		Sale:       0,
		TotalPrice: 100,
		NmID:       0, // Invalid: must be positive
		Brand:      "Test",
		Status:     200,
	}

	errs := item.Validate(0)
	hasNmIDError := false
	for _, e := range errs {
		if strings.Contains(e.Field, "nm_id") {
			hasNmIDError = true
			break
		}
	}
	if !hasNmIDError {
		t.Error("Expected nm_id validation error for zero value")
	}
}

func TestItem_Validate_NegativeNmID(t *testing.T) {
	item := Item{
		ChrtID:     12345,
		Name:       "Test",
		Price:      100,
		Sale:       0,
		TotalPrice: 100,
		NmID:       -1,
		Brand:      "Test",
		Status:     200,
	}

	errs := item.Validate(0)
	hasNmIDError := false
	for _, e := range errs {
		if strings.Contains(e.Field, "nm_id") {
			hasNmIDError = true
			break
		}
	}
	if !hasNmIDError {
		t.Error("Expected nm_id validation error for negative value")
	}
}

// Benchmarks

func BenchmarkOrder_Validate(b *testing.B) {
	order := createValidOrder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order.Validate()
	}
}

func BenchmarkParseOrder(b *testing.B) {
	order := createValidOrder()
	data, _ := json.Marshal(order)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseOrder(data)
	}
}
