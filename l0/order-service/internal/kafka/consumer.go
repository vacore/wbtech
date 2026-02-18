package kafka

import (
	"context"
	"errors"
	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/logger"
	"order-service/internal/models"
	"order-service/internal/repo"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

// Consumer represents message processor from Kafka
type Consumer struct {
	reader *kafka.Reader
	repo   repo.OrderRepo
	cache  *cache.Cache
	stats  ConsumerStats
}

// ConsumerStats contains message processing statistics.
// All fields use atomic operations for lock-free concurrent access.
type ConsumerStats struct {
	MessagesReceived  atomic.Int64
	MessagesProcessed atomic.Int64
	MessagesFailed    atomic.Int64
	LastMessageTime   atomic.Int64 // unix nano
}

// Snapshot is a plain copy of stats safe to read after creation.
type StatsSnapshot struct {
	MessagesReceived  int64
	MessagesProcessed int64
	MessagesFailed    int64
	LastMessageTime   time.Time
}

// NewConsumer creates new Kafka consumer
func NewConsumer(cfg *config.Config, repo repo.OrderRepo, orderCache *cache.Cache) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.KafkaBrokers,
		Topic:          cfg.KafkaTopic,
		GroupID:        cfg.KafkaGroupId,
		MinBytes:       1,
		MaxBytes:       10_000_000,
		MaxWait:        500 * time.Millisecond,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})

	return &Consumer{
		reader: reader,
		repo:   repo,
		cache:  orderCache,
	}
}

// Start initiates messages consuming from Kafka
func (c *Consumer) Start(ctx context.Context) {
	logger.Info("Listening for messages...")

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				snap := c.GetStats()
				logger.Info("Consumer loop terminated (received: %d, processed: %d, failed: %d)",
					snap.MessagesReceived, snap.MessagesProcessed, snap.MessagesFailed)
				return
			}
			logger.Error("Error fetching message: %v", err)
			// Backoff to avoid tight spin-loop when Kafka is unreachable
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		c.stats.MessagesReceived.Add(1)
		c.stats.LastMessageTime.Store(time.Now().UnixNano())

		err = c.processMessage(ctx, msg)
		if err != nil {
			c.stats.MessagesFailed.Add(1)

			// Don't commit transient errors — message will be redelivered
			var permErr *PermanentError
			if !errors.As(err, &permErr) {
				// Retry the same message a few times before giving up
				for retry := 1; retry <= 3; retry++ {
					logger.Warn("Retrying message (attempt %d/3): %v", retry, err)
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Duration(retry) * time.Second):
					}
					if err = c.processMessage(ctx, msg); err == nil {
						c.stats.MessagesProcessed.Add(1)
						break
					}
				}
				if err != nil {
					c.stats.MessagesFailed.Add(1)
					logger.Error("Message failed after retries, committing to skip: %v", err)
					// Fall through to commit — avoid infinite stuck
				}
			}
			// Permanent error — fall through to commit
		} else {
			c.stats.MessagesProcessed.Add(1)
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			logger.Error("Error committing message: %v", err)
		}
	}
}

// processMessage gets a single message, extracts data from it and puts to DB
func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	order, err := models.ParseOrder(msg.Value)
	if err != nil {
		// Validation/JSON errors are permanent — no point in retrying
		if validationErrs, ok := err.(models.ValidationErrors); ok {
			var errStrings []string
			for _, e := range validationErrs {
				errStrings = append(errStrings, e.Error())
			}
			logger.ValidationErr(string(msg.Key), errStrings)
		} else {
			logger.Error("JSON Parse error [key=%s]: %v", string(msg.Key), err)
		}
		return &PermanentError{Err: err} // Mark as permanent
	}

	// Save in the DB
	if err := c.repo.SaveOrder(ctx, order); err != nil {
		logger.Error("Failed to save order %s to DB: %v", order.OrderUID, err)
		return err // Transient — will be retried
	}

	// Update cache immediately so reads see fresh data
	if c.cache != nil {
		c.cache.Set(order)
	}

	logger.Success("Order saved: %s | Customer: %s | Items: %d | Amount: %d %s",
		order.OrderUID, order.CustomerID, len(order.Items),
		order.Payment.Amount, order.Payment.Currency)
	return nil
}

// GetStats returns a point-in-time snapshot of consumer stats
func (c *Consumer) GetStats() StatsSnapshot {
	lastNano := c.stats.LastMessageTime.Load()
	var lastTime time.Time
	if lastNano > 0 {
		lastTime = time.Unix(0, lastNano)
	}
	return StatsSnapshot{
		MessagesReceived:  c.stats.MessagesReceived.Load(),
		MessagesProcessed: c.stats.MessagesProcessed.Load(),
		MessagesFailed:    c.stats.MessagesFailed.Load(),
		LastMessageTime:   lastTime,
	}
}

// Close closes Kafka reader
func (c *Consumer) Close() error {
	return c.reader.Close()
}
