package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"order-service/internal/cache"
	"order-service/internal/models"
	"order-service/internal/testutil"
	"testing"
	"time"

	kafkalib "github.com/segmentio/kafka-go"
)

// PermanentError Tests

func TestPermanentError_Error(t *testing.T) {
	inner := errors.New("bad data")
	err := &PermanentError{Err: inner}

	if err.Error() != "bad data" {
		t.Errorf("Expected 'bad data', got %q", err.Error())
	}
}

func TestPermanentError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := &PermanentError{Err: inner}

	if !errors.Is(err, inner) {
		t.Error("Unwrap should expose inner error")
	}
}

func TestPermanentError_TypeAssertion(t *testing.T) {
	err := error(&PermanentError{Err: errors.New("test")})

	var permErr *PermanentError
	if !errors.As(err, &permErr) {
		t.Error("Should be detectable via errors.As")
	}
}

func TestNonPermanentError_TypeAssertion(t *testing.T) {
	err := errors.New("transient db error")

	var permErr *PermanentError
	if errors.As(err, &permErr) {
		t.Error("Regular error should NOT match PermanentError")
	}
}

// ConsumerStats tests

func TestConsumerStats_Initial(t *testing.T) {
	var stats ConsumerStats

	if stats.MessagesReceived.Load() != 0 {
		t.Errorf("Expected 0, got %d", stats.MessagesReceived.Load())
	}
	if stats.MessagesProcessed.Load() != 0 {
		t.Errorf("Expected 0, got %d", stats.MessagesProcessed.Load())
	}
	if stats.MessagesFailed.Load() != 0 {
		t.Errorf("Expected 0, got %d", stats.MessagesFailed.Load())
	}
	if stats.LastMessageTime.Load() != 0 {
		t.Error("Expected zero timestamp")
	}
}

// processMessage tests

// newTestConsumer creates a Consumer with a mock repo and a cache for unit testing.
// reader is nil because processMessage doesn't use it.
func newTestConsumer(repo *testutil.MockRepo) *Consumer {
	testCache := cache.New(cache.Config{MaxItems: 100, TTL: time.Hour})
	return &Consumer{
		reader: nil,
		repo:   repo,
		cache:  testCache,
		// stats is a value type with atomic fields — zero value is correct
	}
}

func TestProcessMessage_ValidOrder(t *testing.T) {
	mockRepo := testutil.NewMockRepo()
	c := newTestConsumer(mockRepo)

	data := testutil.CreateTestOrderJSON("a1b2c3d4e5f6a7b8test")
	msg := kafkalib.Message{
		Key:   []byte("a1b2c3d4e5f6a7b8test"),
		Value: data,
	}

	err := c.processMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify order was saved to repo
	if !mockRepo.HasOrder("a1b2c3d4e5f6a7b8test") {
		t.Error("Order should be saved to repo")
	}

	// Verify order was cached
	if !c.cache.Exists("a1b2c3d4e5f6a7b8test") {
		t.Error("Order should be cached after successful save")
	}
}

func TestProcessMessage_InvalidJSON(t *testing.T) {
	mockRepo := testutil.NewMockRepo()
	c := newTestConsumer(mockRepo)

	msg := kafkalib.Message{
		Key:   []byte("bad_json"),
		Value: []byte(`{this is not valid json}`),
	}

	err := c.processMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	// Should be a PermanentError
	var permErr *PermanentError
	if !errors.As(err, &permErr) {
		t.Error("Invalid JSON should return PermanentError")
	}

	// Nothing should be saved
	if mockRepo.OrderCount() != 0 {
		t.Error("No order should be saved for invalid JSON")
	}

	// Nothing should be cached
	if c.cache.Size() != 0 {
		t.Error("Nothing should be cached for invalid JSON")
	}
}

func TestProcessMessage_ValidationError(t *testing.T) {
	mockRepo := testutil.NewMockRepo()
	c := newTestConsumer(mockRepo)

	// Valid JSON but fails validation (empty order_uid)
	order := testutil.NewOrderBuilder("").Build()
	data, _ := json.Marshal(order)

	msg := kafkalib.Message{
		Key:   []byte("empty_uid"),
		Value: data,
	}

	err := c.processMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("Expected validation error")
	}

	var permErr *PermanentError
	if !errors.As(err, &permErr) {
		t.Error("Validation error should return PermanentError")
	}

	if mockRepo.OrderCount() != 0 {
		t.Error("No order should be saved for validation failure")
	}

	if c.cache.Size() != 0 {
		t.Error("Nothing should be cached for validation failure")
	}
}

func TestProcessMessage_DBSaveError(t *testing.T) {
	mockRepo := testutil.NewMockRepo()
	mockRepo.SetSaveError(errors.New("connection refused"))
	c := newTestConsumer(mockRepo)

	data := testutil.CreateTestOrderJSON("a1b2c3d4e5f6a7b8test")
	msg := kafkalib.Message{
		Key:   []byte("a1b2c3d4e5f6a7b8test"),
		Value: data,
	}

	err := c.processMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("Expected error for DB save failure")
	}

	// Should NOT be a PermanentError (transient — should be retried)
	var permErr *PermanentError
	if errors.As(err, &permErr) {
		t.Error("DB save error should be transient, not PermanentError")
	}

	// Should NOT be cached when DB save fails
	if c.cache.Exists("a1b2c3d4e5f6a7b8test") {
		t.Error("Order should not be cached when DB save fails")
	}
}

func TestProcessMessage_ValidationErrors_AreLogged(t *testing.T) {
	mockRepo := testutil.NewMockRepo()
	c := newTestConsumer(mockRepo)

	// Build an order with multiple validation errors
	order := testutil.NewOrderBuilder("a1b2c3d4e5f6a7b8test").
		WithItems([]models.Item{}).
		Build()
	data, _ := json.Marshal(order)

	msg := kafkalib.Message{
		Key:   []byte("multi_error"),
		Value: data,
	}

	err := c.processMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("Expected validation errors")
	}

	// Verify it's a PermanentError wrapping ValidationErrors
	var permErr *PermanentError
	if !errors.As(err, &permErr) {
		t.Fatal("Should be PermanentError")
	}

	_, isValidationErrs := permErr.Err.(models.ValidationErrors)
	if !isValidationErrs {
		t.Error("Inner error should be ValidationErrors")
	}
}

func TestProcessMessage_NilCache_DoesNotPanic(t *testing.T) {
	mockRepo := testutil.NewMockRepo()
	c := &Consumer{
		reader: nil,
		repo:   mockRepo,
		cache:  nil,
		// stats zero value is fine
	}

	data := testutil.CreateTestOrderJSON("a1b2c3d4e5f6a7b8test")
	msg := kafkalib.Message{
		Key:   []byte("a1b2c3d4e5f6a7b8test"),
		Value: data,
	}

	// Should not panic even with nil cache
	err := c.processMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !mockRepo.HasOrder("a1b2c3d4e5f6a7b8test") {
		t.Error("Order should still be saved to repo")
	}
}

func TestGetStats_ReflectsProcessing(t *testing.T) {
	mockRepo := testutil.NewMockRepo()
	c := newTestConsumer(mockRepo)

	// Process a valid message
	data := testutil.CreateTestOrderJSON("a1b2c3d4e5f6a7b8test")
	msg := kafkalib.Message{Key: []byte("test"), Value: data}
	c.processMessage(context.Background(), msg)

	// Stats aren't updated by processMessage itself — they're updated by Start().
	// But we can verify GetStats returns the struct correctly.
	stats := c.GetStats()
	if stats.MessagesReceived != 0 {
		t.Errorf("processMessage doesn't update stats directly, expected 0, got %d",
			stats.MessagesReceived)
	}
}

// Integration-style ParseOrder Tests

func TestParseValidOrder(t *testing.T) {
	data := testutil.CreateTestOrderJSON("a1b2c3d4e5f6a7b8test")

	order, err := models.ParseOrder(data)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if order.OrderUID != "a1b2c3d4e5f6a7b8test" {
		t.Errorf("Wrong UID: %s", order.OrderUID)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := models.ParseOrder([]byte(`{invalid}`))
	if err == nil {
		t.Error("Expected error")
	}
}

func TestParseEmptyOrderUID(t *testing.T) {
	order := testutil.NewOrderBuilder("").Build()
	data, _ := json.Marshal(order)
	_, err := models.ParseOrder(data)
	if err == nil {
		t.Error("Expected validation error")
	}
}

func TestParseEmptyItems(t *testing.T) {
	order := testutil.NewOrderBuilder("a1b2c3d4e5f6a7b8test").
		WithItems([]models.Item{}).
		Build()
	data, _ := json.Marshal(order)
	_, err := models.ParseOrder(data)
	if err == nil {
		t.Error("Expected error for empty items")
	}
}

// Benchmarks

func BenchmarkParseOrder(b *testing.B) {
	data := testutil.CreateTestOrderJSON("a1b2c3d4e5f6a7b8test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		models.ParseOrder(data)
	}
}

func BenchmarkProcessMessage(b *testing.B) {
	mockRepo := testutil.NewMockRepo()
	c := newTestConsumer(mockRepo)
	data := testutil.CreateTestOrderJSON("a1b2c3d4e5f6a7b8test")
	msg := kafkalib.Message{Key: []byte("bench"), Value: data}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.processMessage(ctx, msg)
	}
}
