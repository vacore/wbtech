package repo

import (
	"context"
	"time"

	"order-service/internal/models"
)

// OrderListItem is a lightweight summary returned by list queries
type OrderListItem struct {
	OrderUID    string    `json:"order_uid"`
	DateCreated time.Time `json:"date_created"`
}

// OrderRepo defines the interface for order data access
type OrderRepo interface {
	SaveOrder(ctx context.Context, order *models.Order) error
	GetOrder(ctx context.Context, orderUID string) (*models.Order, error)
	GetAllOrders(ctx context.Context, limit int) ([]*models.Order, error)
	GetOrderUIDs(ctx context.Context, limit, offset int, sortAsc bool) ([]OrderListItem, int64, error)
	GetOrderCount(ctx context.Context) (int64, error)
	DeleteOrder(ctx context.Context, orderUID string) error
}
