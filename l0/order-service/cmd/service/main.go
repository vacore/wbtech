package main

import (
	"context"
	"net"
	"net/http"
	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/handler"
	"order-service/internal/kafka"
	"order-service/internal/logger"
	"order-service/internal/repo"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	logger.Title("Order Service Starting")

	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	logger.Title("Database Connection")
	logger.Info("Connecting to PostgreSQL at %s:%d...", cfg.DBHost, cfg.DBPort)

	orderRepo, err := repo.New(ctx, cfg)
	if err != nil {
		logger.Error("Failed to connect to PostgreSQL: %v", err)
		os.Exit(1)
	}
	defer orderRepo.Close()
	logger.Success("Connected to PostgreSQL (pool: min=%d, max=%d)", cfg.DBMinConns, cfg.DBMaxConns)

	logger.Title("Cache Initialization")
	cacheConfig := cache.Config{
		MaxItems: cfg.CacheMaxItems,
		TTL:      cfg.CacheTTL,
	}
	orderCache := cache.New(cacheConfig)
	defer orderCache.Stop()
	logger.Info("LRU Cache created: max_items=%d, ttl=%v", cfg.CacheMaxItems, cfg.CacheTTL)

	logger.Info("Restoring cache from database (limit: %d)...", cfg.CacheMaxItems)
	orders, err := orderRepo.GetAllOrders(ctx, cfg.CacheMaxItems)
	if err != nil {
		logger.Warn("Failed to restore cache from DB: %v", err)
	} else {
		orderCache.LoadFromSlice(orders)
		logger.Success("Cache restored: %d orders loaded", orderCache.Size())
	}

	logger.Title("Kafka Consumer")
	logger.Info("Connecting to Kafka at %s...", cfg.KafkaBrokers[0])
	consumer := kafka.NewConsumer(cfg, orderRepo, orderCache)

	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer.Start(ctx)
	}()
	logger.Success("Kafka consumer started (topic: %s, group: %s)", cfg.KafkaTopic, cfg.KafkaGroupId)

	logger.Title("HTTP Server")
	h := handler.New(orderCache, orderRepo)
	server := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      h.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ln, err := net.Listen("tcp", ":"+cfg.HTTPPort)
	if err != nil {
		logger.Error("Failed to bind port %s: %v", cfg.HTTPPort, err)
		os.Exit(1)
	}
	logger.Success("HTTP server listening on http://localhost:%s", cfg.HTTPPort)

	// Channel to catch start-up errors from HTTP server
	errChan := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error: %v", err)
			errChan <- err // Signal main to exit
		}
	}()

	logger.Title("Service Ready")
	logger.Success("Order Service is running. Press Ctrl+C to stop.")

	// Channel to catch OS shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Now wait for Shutdown Signal or error
	select {
	case <-sigChan:
		logger.Title("Shutting down")
		logger.Info("Shutdown signal received...")
	case err := <-errChan:
		logger.Title("Fatal Error")
		logger.Error("Service crashing due to: %v", err)
	}

	// Stop Kafka first (stop processing new messages)
	cancel()
	logger.Info("Kafka consumer stopping...")

	// Stop HTTP server (stop accepting new requests)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error: %v", err)
	} else {
		logger.Success("HTTP server stopped")
	}

	// Close Kafka Connection
	if err := consumer.Close(); err != nil {
		logger.Error("Kafka consumer close error: %v", err)
	} else {
		logger.Success("Kafka consumer stopped")
	}

	// Wait for goroutines to finish
	wg.Wait()

	// Final Stats
	stats := consumer.GetStats()
	logger.Title("Final Statistics")
	logger.Info("Messages Received:  %d", stats.MessagesReceived)
	logger.Info("Messages Processed: %d", stats.MessagesProcessed)
	logger.Info("Messages Failed:    %d", stats.MessagesFailed)

	cacheStats := orderCache.GetStats()
	logger.Info("Cache Hits:         %d", cacheStats.Hits)
	logger.Info("Cache Misses:       %d", cacheStats.Misses)
	logger.Info("Cache Evictions:    %d", cacheStats.Evictions)
}
