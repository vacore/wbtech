package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"order-service/internal/cache"
	"order-service/internal/metrics"
	"order-service/internal/repo"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 500
)

// Handler processes HTTP-requests via API
type Handler struct {
	cache     cache.OrderCache
	repo      repo.OrderRepo
	startTime time.Time
}

// New returns new Handler instance
func New(cache cache.OrderCache, repo repo.OrderRepo) *Handler {
	return &Handler{
		cache:     cache,
		repo:      repo,
		startTime: time.Now(),
	}
}

// skipPaths - paths that I don't want to log requests to
var skipPaths = map[string]bool{
	"/health": true,
	"/stats":  true,
	// "/orders": true, // order list request
	"/": true, // index.html load
}

// myLogger - middleware for logging with exceptions to certain paths
func myLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skipPaths[r.URL.Path] ||
			strings.HasPrefix(r.URL.Path, "/static/") ||
			(strings.HasPrefix(r.URL.Path, "/order/") && r.Method == http.MethodGet) {
			next.ServeHTTP(w, r)
			return
		}
		middleware.Logger(next).ServeHTTP(w, r)
	})
}

// Router creates and sets up the HTTP-router
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(myLogger)
	r.Use(metrics.Middleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/", h.serveIndex)
	r.Get("/order/{orderUID}", h.getOrder)
	r.Delete("/order/{orderUID}", h.deleteOrder)
	r.Get("/orders", h.getOrderList)
	r.Get("/health", h.healthCheck)
	r.Get("/stats", h.getStats)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("./web"))))

	r.Handle("/metrics", promhttp.Handler())
	return r
}

// serveIndex returns the main page of the web-interface
func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/index.html")
}

// getOrder returns an order specified by orderUID in the JSON-response
func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	orderUID := chi.URLParam(r, "orderUID")
	if orderUID == "" {
		h.errorResponse(w, http.StatusBadRequest, "order_uid is required")
		return
	}
	if len(orderUID) > 50 {
		h.errorResponse(w, http.StatusBadRequest, "order_uid too long")
		return
	}

	slog.Info("fetching order details", "order_uid", orderUID)

	start := time.Now()

	// Check if the order is in cache
	order, found := h.cache.Get(orderUID)
	source := "cache"

	if !found {
		// Cache MISS - Read from DB
		source = "database"
		var err error
		order, err = h.repo.GetOrder(r.Context(), orderUID)
		if err != nil {
			if errors.Is(err, repo.ErrOrderNotFound) {
				slog.Warn("order not found", "order_uid", orderUID)
				h.errorResponse(w, http.StatusNotFound, "order not found")
				return
			}
			slog.Error("Database", "error", err)
			h.errorResponse(w, http.StatusInternalServerError, "internal server error")
			return
		}

		h.cache.Set(order) // save to cache
	}

	duration := time.Since(start)

	slog.Info("order retrieved", "source", source, "duration", duration)

	h.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"order":  order,
		"source": source,
		"time":   duration.String(),
	})
}

// deleteOrder deletes order specified by orderUID from the database
func (h *Handler) deleteOrder(w http.ResponseWriter, r *http.Request) {
	orderUID := chi.URLParam(r, "orderUID")
	if orderUID == "" {
		h.errorResponse(w, http.StatusBadRequest, "order_uid is required")
		return
	}
	if len(orderUID) > 50 {
		h.errorResponse(w, http.StatusBadRequest, "order_uid too long")
		return
	}

	slog.Info("deleting order", "order_uid", orderUID)

	err := h.repo.DeleteOrder(r.Context(), orderUID)
	if err != nil {
		if errors.Is(err, repo.ErrOrderNotFound) {
			slog.Warn("delete order: order not found", "order_uid", orderUID)
			h.errorResponse(w, http.StatusNotFound, "order not found")
			return
		}
		slog.Error("delete order", "order_uid", orderUID, "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.cache.Delete(orderUID) // Remove from cache too

	slog.Info("order deleted", "order_uid", orderUID)

	h.jsonResponse(w, http.StatusOK, map[string]string{
		"status":    "deleted",
		"order_uid": orderUID,
	})
}

// getOrderList returns a paginated list of orders in the DB.
// Query params: ?limit=50&offset=0&sort=desc
func (h *Handler) getOrderList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit := defaultPageLimit
	offset := 0
	sortAsc := false

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > maxPageLimit {
				limit = maxPageLimit
			}
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if s := r.URL.Query().Get("sort"); s == "asc" {
		sortAsc = true
	}

	items, total, err := h.repo.GetOrderUIDs(ctx, limit, offset, sortAsc)
	if err != nil {
		slog.Error("getting order UIDs", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	type OrderInfo struct {
		OrderUID    string    `json:"order_uid"`
		DateCreated time.Time `json:"date_created"`
		Cached      bool      `json:"cached"`
	}

	orders := make([]OrderInfo, len(items))
	for i, item := range items {
		orders[i] = OrderInfo{
			OrderUID:    item.OrderUID,
			DateCreated: item.DateCreated,
			Cached:      h.cache.Exists(item.OrderUID),
		}
	}

	h.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"count":      len(orders),
		"total":      total,
		"limit":      limit,
		"offset":     offset,
		"orders":     orders,
		"cache_size": h.cache.Size(),
	})
}

// healthCheck checks service's health
func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	_, err := h.repo.GetOrderCount(r.Context())

	dbStatus := "ok"
	svcStatus := "ok"
	status := http.StatusOK

	if err != nil {
		dbStatus = "error"
		svcStatus = "degraded"
		status = http.StatusServiceUnavailable
	}

	h.jsonResponse(w, status, map[string]interface{}{
		"status":   svcStatus,
		"database": dbStatus,
		"uptime":   time.Since(h.startTime).String(),
	})
}

// getStats returns extended statistics of the service
func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	dbCount, err := h.repo.GetOrderCount(r.Context())
	if err != nil {
		slog.Error("getting order count", "error", err)
		dbCount = -1
	}

	cacheStats := h.cache.GetStats()
	cacheConfig := h.cache.GetConfig()

	// Calculate hit rate
	totalRequests := cacheStats.Hits + cacheStats.Misses
	var hitRate float64
	if totalRequests > 0 {
		hitRate = float64(cacheStats.Hits) / float64(totalRequests) * 100
	}

	h.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"cache_size":        h.cache.Size(),
		"cache_max_size":    cacheConfig.MaxItems,
		"db_count":          dbCount,
		"cache_hits":        cacheStats.Hits,
		"cache_misses":      cacheStats.Misses,
		"hit_rate":          fmt.Sprintf("%.2f%%", hitRate),
		"cache_evictions":   cacheStats.Evictions,
		"cache_expirations": cacheStats.Expirations,
		"cache_ttl":         cacheConfig.TTL.String(),
		"uptime":            time.Since(h.startTime).String(),
	})
}

func (h *Handler) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("encoding JSON response", "error", err)
	}
}

func (h *Handler) errorResponse(w http.ResponseWriter, status int, message string) {
	h.jsonResponse(w, status, map[string]string{"error": message})
}
