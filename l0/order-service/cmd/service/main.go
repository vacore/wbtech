package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lmittmann/tint"

	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/handler"
	"order-service/internal/kafka"
	"order-service/internal/repo"
	"order-service/internal/tracing"
)

func main() {
	cfg := config.Load()

	logLevel := parseLogLevel(cfg.Log.Level)
	setupLogger(logLevel)

	slog.Info("=== ORDER SERVICE STARTING ===")

	// Init tracing
	shutdownTracer, err := tracing.Init("order-service", cfg.Tracing.Exporter)
	if err != nil {
		slog.Error("failed to init tracing", "error", err)
		os.Exit(1)
	}
	defer func() { // use separate context
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := shutdownTracer(shutdownCtx); err != nil {
			slog.Error("failed to shut down tracer", "error", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	slog.Info("=== DATABASE CONNECTION ===")
	slog.Info("connecting to PostgreSQL", "host", cfg.DB.Host, "port", cfg.DB.Port)

	orderRepo, err := repo.New(ctx, cfg.DB)
	if err != nil {
		slog.Error("connecting to PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer orderRepo.Close()
	slog.Info("connected to PostgreSQL", "min_conns", cfg.DB.MinConns, "max_conns", cfg.DB.MaxConns)

	slog.Info("=== CACHE INITIALIZATION ===")
	orderCache := cache.New(cfg.Cache)
	defer orderCache.Stop()
	slog.Info("cache created", "max_items", cfg.Cache.MaxItems, "ttl", cfg.Cache.TTL)

	slog.Info("filling from the database", "limit", cfg.Cache.MaxItems)
	orders, err := orderRepo.GetAllOrders(ctx, cfg.Cache.MaxItems)
	if err != nil {
		slog.Warn("filling cache from the DB", "error", err)
	} else {
		orderCache.LoadFromSlice(orders)
		slog.Info("cache filled", "orders_loaded", orderCache.Size())
	}

	slog.Info("=== KAFKA CONSUMER ===")
	slog.Info("connecting to Kafka", "at", cfg.Kafka.Brokers[0])
	consumer := kafka.NewConsumer(cfg.Kafka, orderRepo, orderCache)

	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer.Start(ctx)
	}()
	slog.Info("Kafka consumer started", "topic", cfg.Kafka.Topic, "group", cfg.Kafka.GroupID)

	slog.Info("=== HTTP Server ===")
	h := handler.New(orderCache, orderRepo)
	server := &http.Server{
		Addr:           ":" + cfg.HTTP.Port,
		Handler:        h.Router(),
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MiB,
	}

	ln, err := net.Listen("tcp", ":"+cfg.HTTP.Port)
	if err != nil {
		slog.Error("failed to bind port", "port", cfg.HTTP.Port, "error", err)
		os.Exit(1)
	}
	slog.Info("HTTP server listening", "on", "http://localhost:"+cfg.HTTP.Port)

	// Channel to catch start-up errors from HTTP server
	errChan := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server", "error", err)
			errChan <- err // Signal main to exit
		}
	}()

	slog.Info("=== SERVICE READY ===")
	slog.Info("Order service is running. Press Ctrl+C to stop.")

	// Channel to catch OS shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Now wait for Shutdown Signal or error
	select {
	case <-sigChan:
		slog.Info("=== SHUTTING DOWN ===")
		slog.Info("shutdown signal received...")
	case err := <-errChan:
		slog.Error("=== FATAL ERROR ===")
		slog.Error("fatal error", "error", err)
	}

	// Stop Kafka first (stop processing new messages)
	cancel()
	slog.Info("Kafka consumer stopping...")

	// Stop HTTP server (stop accepting new requests)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutting down HTTP server", "error", err)
	} else {
		slog.Info("HTTP server stopped")
	}

	// Close Kafka Connection
	if err := consumer.Close(); err != nil {
		slog.Error("closing Kafka consumer", "error", err)
	} else {
		slog.Info("Kafka consumer stopped")
	}

	// Wait for goroutines to finish
	wg.Wait()

	// Final Stats
	stats := consumer.GetStats()
	slog.Info("stats: messages", "received", stats.MessagesReceived, "processed", stats.MessagesProcessed, "failed", stats.MessagesFailed)

	cacheStats := orderCache.GetStats()
	slog.Info("stats: cache", "hits", cacheStats.Hits, "misses", cacheStats.Misses, "evictions", cacheStats.Evictions)
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func setupLogger(level slog.Level) {
	// Use colored output for terminals, plain text otherwise
	var handler slog.Handler

	if isTerminal() {
		handler = tint.NewHandler(os.Stderr, &tint.Options{
			Level:      level,
			TimeFormat: "15:04:05",
		})
	} else {
		// JSON for production / log aggregation
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	}

	slog.SetDefault(slog.New(handler))
}

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
