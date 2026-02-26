// Package testutil provides shared test utilities and helpers
package testutil

import (
	"encoding/json"
	"time"

	"order-service/internal/models"
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
