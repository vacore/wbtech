package kafka

import "context"

// MessageConsumer defines interface for message consumption
type MessageConsumer interface {
	Start(ctx context.Context)
	Close() error
	GetStats() StatsSnapshot
}

// Ensure Consumer implements MessageConsumer
var _ MessageConsumer = (*Consumer)(nil)
