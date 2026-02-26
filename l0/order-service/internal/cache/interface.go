package cache

import (
	"order-service/internal/config"
	"order-service/internal/models"
)

// OrderCache defines interface for order caching
type OrderCache interface {
	Set(order *models.Order)
	Get(orderUID string) (*models.Order, bool)
	Delete(orderUID string)
	Exists(orderUID string) bool
	Size() int
	LoadFromSlice(orders []*models.Order)
	GetStats() Stats
	GetConfig() config.CacheConfig
	Stop()
}
