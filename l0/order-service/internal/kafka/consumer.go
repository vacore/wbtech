package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"

	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/metrics"
	"order-service/internal/models"
	"order-service/internal/repo"
)

// Consumer represents message processor from Kafka
type Consumer struct {
	reader    *kafka.Reader
	dlqWriter *kafka.Writer
	repo      repo.OrderRepo
	cache     cache.OrderCache
	stats     ConsumerStats
}

// dlqEnvelope is the message written to the DLQ topic
type dlqEnvelope struct {
	OriginalKey   string `json:"original_key"`
	OriginalValue string `json:"original_value"`
	Error         string `json:"error"`
	Topic         string `json:"topic"`
	Partition     int    `json:"partition"`
	Offset        int64  `json:"offset"`
	Timestamp     string `json:"timestamp"`
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
func NewConsumer(cfg config.KafkaConfig, repo repo.OrderRepo, orderCache cache.OrderCache) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		MinBytes:       1,
		MaxBytes:       10_000_000,
		MaxWait:        500 * time.Millisecond,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})

	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.DLQTopic,
		Balancer:     &kafka.LeastBytes{},
		WriteTimeout: 10 * time.Second,
		RequiredAcks: kafka.RequireAll,
	}

	return &Consumer{
		reader:    reader,
		dlqWriter: dlqWriter,
		repo:      repo,
		cache:     orderCache,
	}
}

const maxTransientRetries = 5

// Start initiates messages consuming from Kafka
func (c *Consumer) Start(ctx context.Context) {
	slog.Info("listening for messages")

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				snap := c.GetStats()
				slog.Info("consumer stopped",
					"received", snap.MessagesReceived,
					"processed", snap.MessagesProcessed,
					"failed", snap.MessagesFailed,
				)
				return
			}
			slog.Error("error fetching message", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		c.stats.MessagesReceived.Add(1)
		c.stats.LastMessageTime.Store(time.Now().UnixNano())

		c.processWithRetry(ctx, msg)
	}
}

func (c *Consumer) processWithRetry(ctx context.Context, msg kafka.Message) {
	var lastErr error

	for attempt := 1; attempt <= maxTransientRetries; attempt++ {
		lastErr = c.processMessage(ctx, msg)
		if lastErr == nil {
			c.stats.MessagesProcessed.Add(1)
			metrics.KafkaMessagesTotal.WithLabelValues("processed").Inc()
			break
		}

		// Permanent errors go straight to DLQ - no retry
		if errors.Is(lastErr, ErrPermanent) {
			c.stats.MessagesFailed.Add(1)
			metrics.KafkaMessagesTotal.WithLabelValues("failed").Inc()
			if dlqErr := c.sendToDLQ(ctx, msg, lastErr); dlqErr != nil {
				slog.Error("failed to send to DLQ", "error", dlqErr)
			}
			break
		}

		// Transient error - retry with backoff
		if attempt < maxTransientRetries {
			backoff := time.Duration(attempt) * time.Second
			slog.Warn("transient error, retrying",
				"attempt", attempt, "max", maxTransientRetries,
				"backoff", backoff, "error", lastErr)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		} else {
			// Exhausted retries - send to DLQ
			c.stats.MessagesFailed.Add(1)
			metrics.KafkaMessagesTotal.WithLabelValues("failed").Inc()
			slog.Error("transient error exhausted retries, sending to DLQ",
				"attempts", maxTransientRetries,
				"error", lastErr,
			)
			if dlqErr := c.sendToDLQ(ctx, msg, lastErr); dlqErr != nil {
				slog.Error("failed to send to DLQ", "error", dlqErr)
			}
		}
	}

	// Always commit after processing (success, permanent error, or exhausted retries)
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		slog.Error("error committing message", "error", err)
	}
}

// processMessage gets a single message, extracts data from it and puts to DB
func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	start := time.Now()
	defer func() { metrics.KafkaProcessDuration.Observe(time.Since(start).Seconds()) }()

	order, err := models.ParseOrder(msg.Value)
	if err != nil {
		// Validation/JSON errors are permanent - no point in retrying
		var validationErrs models.ValidationErrors
		if errors.As(err, &validationErrs) {
			var errStrings []string
			for _, e := range validationErrs {
				errStrings = append(errStrings, e.Error())
			}
			slog.Warn("validation failed", "key", string(msg.Key), "errors", errStrings)
		} else {
			slog.Error("parsing JSON", "key", string(msg.Key), "error", err)
		}
		return errors.Join(ErrPermanent, err) // permanent - both findable via errors.Is/As
	}

	// Save in the DB
	if err := c.repo.SaveOrder(ctx, order); err != nil {
		slog.Error("saving order to DB", "order_uid", order.OrderUID, "error", err)
		return fmt.Errorf("saving order %s: %w", order.OrderUID, err) // transient - will be retried
	}

	if c.cache != nil {
		c.cache.Set(order)
	}

	slog.Info("Order saved", "order_uid", order.OrderUID, "customer_id", order.CustomerID,
		"num_items", len(order.Items), "amount", order.Payment.Amount, "currency", order.Payment.Currency)
	return nil
}

// sendToDLQ sends the invalid message to the DLQ
func (c *Consumer) sendToDLQ(ctx context.Context, msg kafka.Message, originalErr error) error {
	envelope := dlqEnvelope{
		OriginalKey:   string(msg.Key),
		OriginalValue: string(msg.Value),
		Error:         originalErr.Error(),
		Topic:         msg.Topic,
		Partition:     msg.Partition,
		Offset:        msg.Offset,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshaling DLQ envelope: %w", err)
	}

	if err := c.dlqWriter.WriteMessages(ctx, kafka.Message{
		Key:   msg.Key,
		Value: data,
	}); err != nil {
		return fmt.Errorf("writing to DLQ: %w", err)
	}

	metrics.KafkaMessagesTotal.WithLabelValues("dlq").Inc()

	slog.Warn("message sent to DLQ", "key", string(msg.Key), "error", originalErr.Error())
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
	var errs []error
	if err := c.reader.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing reader: %w", err))
	}
	if err := c.dlqWriter.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing DLQ writer: %w", err))
	}
	return errors.Join(errs...)
}
