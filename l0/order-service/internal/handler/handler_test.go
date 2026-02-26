package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/mocks"
	"order-service/internal/testutil"
)

func createTestHandler() (*Handler, *mocks.MockRepo) {
	orderCache := cache.New(config.CacheConfig{MaxItems: 100, TTL: time.Hour})
	mockRepo := mocks.NewMockRepo()
	h := &Handler{
		cache:     orderCache,
		repo:      mockRepo,
		startTime: time.Now(),
	}
	return h, mockRepo
}

func TestHandler_Router(t *testing.T) {
	h, _ := createTestHandler()
	if h.Router() == nil {
		t.Fatal("Router() returned nil")
	}
}

func TestHandler_HealthCheck(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.AddOrder(testutil.CreateTestOrder("order_001"))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != "ok" {
		t.Errorf("Expected status=ok, got %v", resp["status"])
	}
	if resp["database"] != "ok" {
		t.Errorf("Expected database=ok, got %v", resp["database"])
	}
}

func TestHandler_HealthCheck_DBError(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.SetCountError(errors.New("db error"))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != "degraded" {
		t.Errorf("Expected status=degraded, got %v", resp["status"])
	}
	if resp["database"] != "error" {
		t.Errorf("Expected database=error, got %v", resp["database"])
	}
}

func TestHandler_GetStats(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.AddOrder(testutil.CreateTestOrder("order_001"))

	h.cache.Set(testutil.CreateTestOrder("order_001"))
	h.cache.Get("order_001") // hit
	h.cache.Get("missing")   // miss

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	fields := []string{"cache_size", "cache_hits", "cache_misses", "db_count", "uptime"}
	for _, f := range fields {
		if _, ok := resp[f]; !ok {
			t.Errorf("Missing field: %s", f)
		}
	}
}

func TestHandler_GetOrder_FromCache(t *testing.T) {
	h, _ := createTestHandler()
	h.cache.Set(testutil.CreateTestOrder("order_001"))

	req := httptest.NewRequest(http.MethodGet, "/order/order_001", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["source"] != "cache" {
		t.Errorf("Expected source=cache, got %v", resp["source"])
	}
}

func TestHandler_GetOrder_FromDB(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.AddOrder(testutil.CreateTestOrder("order_002"))

	req := httptest.NewRequest(http.MethodGet, "/order/order_002", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["source"] != "database" {
		t.Errorf("Expected source=database, got %v", resp["source"])
	}

	// Should be cached now
	if !h.cache.Exists("order_002") {
		t.Error("Order should be cached")
	}
}

func TestHandler_GetOrder_NotFound(t *testing.T) {
	h, _ := createTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/order/nonexistent", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rec.Code)
	}
}

func TestHandler_GetOrderList(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.AddOrder(testutil.CreateTestOrder("order_001"))
	mockRepo.AddOrder(testutil.CreateTestOrder("order_002"))

	h.cache.Set(testutil.CreateTestOrder("order_001"))

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["count"].(float64) != 2 {
		t.Errorf("Expected count=2, got %v", resp["count"])
	}
	if resp["total"].(float64) != 2 {
		t.Errorf("Expected total=2, got %v", resp["total"])
	}
	if resp["limit"].(float64) != float64(defaultPageLimit) {
		t.Errorf("Expected limit=%d, got %v", defaultPageLimit, resp["limit"])
	}
	if resp["offset"].(float64) != 0 {
		t.Errorf("Expected offset=0, got %v", resp["offset"])
	}
}

func TestHandler_GetOrderList_WithPagination(t *testing.T) {
	h, mockRepo := createTestHandler()
	for i := 0; i < 5; i++ {
		mockRepo.AddOrder(testutil.CreateTestOrder(fmt.Sprintf("order_%03d", i)))
	}

	req := httptest.NewRequest(http.MethodGet, "/orders?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["count"].(float64) != 2 {
		t.Errorf("Expected count=2 (page size), got %v", resp["count"])
	}
	if resp["total"].(float64) != 5 {
		t.Errorf("Expected total=5, got %v", resp["total"])
	}
	if resp["limit"].(float64) != 2 {
		t.Errorf("Expected limit=2, got %v", resp["limit"])
	}
	if resp["offset"].(float64) != 1 {
		t.Errorf("Expected offset=1, got %v", resp["offset"])
	}
}

func TestHandler_GetOrderList_LimitCapped(t *testing.T) {
	h, _ := createTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/orders?limit=9999", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["limit"].(float64) != float64(maxPageLimit) {
		t.Errorf("Expected limit capped to %d, got %v", maxPageLimit, resp["limit"])
	}
}

func TestHandler_CORS(t *testing.T) {
	h, _ := createTestHandler()

	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Missing CORS header")
	}
}

func TestHandler_ContentType(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.AddOrder(testutil.CreateTestOrder("test_order"))
	h.cache.Set(testutil.CreateTestOrder("test_order"))

	endpoints := []string{"/health", "/stats", "/order/test_order"}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)

			ct := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Expected application/json, got %s", ct)
			}
		})
	}
}

func TestHandler_JSONResponse(t *testing.T) {
	h, _ := createTestHandler()
	rec := httptest.NewRecorder()
	h.jsonResponse(rec, http.StatusOK, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("Wrong content type")
	}
}

func TestHandler_ErrorResponse(t *testing.T) {
	h, _ := createTestHandler()
	rec := httptest.NewRecorder()
	h.errorResponse(rec, http.StatusBadRequest, "test error")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "test error" {
		t.Errorf("Expected 'test error', got %s", resp["error"])
	}
}

func TestHandler_DeleteOrder_Success(t *testing.T) {
	h, mockRepo := createTestHandler()
	order := testutil.CreateTestOrder("order_001")
	mockRepo.AddOrder(order)
	h.cache.Set(order)

	req := httptest.NewRequest(http.MethodDelete, "/order/order_001", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("Expected status=deleted, got %s", resp["status"])
	}
	if resp["order_uid"] != "order_001" {
		t.Errorf("Expected order_uid=order_001, got %s", resp["order_uid"])
	}

	// Verify removed from cache
	if h.cache.Exists("order_001") {
		t.Error("Order should be removed from cache after delete")
	}

	// Verify removed from repo
	if mockRepo.HasOrder("order_001") {
		t.Error("Order should be removed from repo after delete")
	}
}

func TestHandler_DeleteOrder_TooLongUID(t *testing.T) {
	h, _ := createTestHandler()

	longUID := strings.Repeat("a", 51)
	req := httptest.NewRequest(http.MethodDelete, "/order/"+longUID, nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", rec.Code)
	}
}

func TestHandler_DeleteOrder_NotFound(t *testing.T) {
	h, _ := createTestHandler()

	req := httptest.NewRequest(http.MethodDelete, "/order/nonexistent", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "order not found" {
		t.Errorf("Expected 'order not found', got %s", resp["error"])
	}
}

func TestHandler_DeleteOrder_DBError(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.SetDeleteError(errors.New("connection refused"))

	req := httptest.NewRequest(http.MethodDelete, "/order/some_order", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", rec.Code)
	}
}

func TestHandler_DeleteOrder_AlsoRemovesFromCache(t *testing.T) {
	h, mockRepo := createTestHandler()
	order := testutil.CreateTestOrder("cached_order")
	mockRepo.AddOrder(order)
	h.cache.Set(order)

	// Verify it's cached
	if !h.cache.Exists("cached_order") {
		t.Fatal("Setup failed: order should be in cache")
	}

	req := httptest.NewRequest(http.MethodDelete, "/order/cached_order", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}
	if h.cache.Exists("cached_order") {
		t.Error("Order should be evicted from cache after delete")
	}
}

func TestHandler_GetOrder_DBError(t *testing.T) {
	h, mockRepo := createTestHandler()
	// Not in cache, and DB returns a generic error (not ErrOrderNotFound)
	mockRepo.SetGetError(errors.New("connection refused"))

	req := httptest.NewRequest(http.MethodGet, "/order/some_order", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "internal server error" {
		t.Errorf("Expected 'internal server error', got %s", resp["error"])
	}
}

func TestHandler_GetOrderList_DBError(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.SetUIDsError(errors.New("db connection lost"))

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", rec.Code)
	}
}

func TestHandler_GetStats_CountError(t *testing.T) {
	h, mockRepo := createTestHandler()
	mockRepo.SetCountError(errors.New("db error"))

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	// Should still return 200 - count falls back to -1
	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	dbCount := resp["db_count"].(float64)
	if dbCount != -1 {
		t.Errorf("Expected db_count=-1 on error, got %v", dbCount)
	}
}

func TestHandler_GetOrder_CacheMissThenDB(t *testing.T) {
	h, mockRepo := createTestHandler()
	// Order in DB but NOT in cache
	mockRepo.AddOrder(testutil.CreateTestOrder("db_only"))

	req := httptest.NewRequest(http.MethodGet, "/order/db_only", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["source"] != "database" {
		t.Errorf("Expected source=database, got %v", resp["source"])
	}

	// After a DB fetch, the order should now be in cache
	if !h.cache.Exists("db_only") {
		t.Error("Order should be cached after DB fetch")
	}

	// Second request should come from cache
	rec2 := httptest.NewRecorder()
	h.Router().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/order/db_only", nil))

	var resp2 map[string]interface{}
	json.NewDecoder(rec2.Body).Decode(&resp2)
	if resp2["source"] != "cache" {
		t.Errorf("Second request should come from cache, got %v", resp2["source"])
	}
}

func TestHandler_GetOrder_EmptyUID(t *testing.T) {
	h, _ := createTestHandler()

	// Chi won't route to this (no match), but test the param validation
	req := httptest.NewRequest(http.MethodGet, "/order/", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	// Chi returns 405 or redirects for trailing slash - just verify no panic
	if rec.Code == http.StatusOK {
		t.Error("Empty UID should not return 200")
	}
}

func TestHandler_GetOrder_TooLongUID(t *testing.T) {
	h, _ := createTestHandler()

	longUID := strings.Repeat("a", 51)
	req := httptest.NewRequest(http.MethodGet, "/order/"+longUID, nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for too-long UID, got %d", rec.Code)
	}
}

func TestHandler_GetOrderList_InvalidParams(t *testing.T) {
	h, _ := createTestHandler()

	// Negative limit should use default
	req := httptest.NewRequest(http.MethodGet, "/orders?limit=-5&offset=-1", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["limit"].(float64) != float64(defaultPageLimit) {
		t.Errorf("Negative limit should fall back to default, got %v", resp["limit"])
	}
}

func TestHandler_GetOrderList_SortAsc(t *testing.T) {
	h, mockRepo := createTestHandler()
	for i := 0; i < 3; i++ {
		mockRepo.AddOrder(testutil.CreateTestOrder(fmt.Sprintf("order_%03d", i)))
	}

	req := httptest.NewRequest(http.MethodGet, "/orders?sort=asc", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
}

func TestHandler_GetOrder_CachePopulatedAfterDBFetch(t *testing.T) {
	h, mockRepo := createTestHandler()
	order := testutil.CreateTestOrder("fetch_and_cache")
	mockRepo.AddOrder(order)

	// First request - cache miss, DB hit
	req1 := httptest.NewRequest(http.MethodGet, "/order/fetch_and_cache", nil)
	rec1 := httptest.NewRecorder()
	h.Router().ServeHTTP(rec1, req1)

	var resp1 map[string]interface{}
	json.NewDecoder(rec1.Body).Decode(&resp1)
	if resp1["source"] != "database" {
		t.Errorf("First request should be from database, got %v", resp1["source"])
	}

	// Second request - cache hit
	req2 := httptest.NewRequest(http.MethodGet, "/order/fetch_and_cache", nil)
	rec2 := httptest.NewRecorder()
	h.Router().ServeHTTP(rec2, req2)

	var resp2 map[string]interface{}
	json.NewDecoder(rec2.Body).Decode(&resp2)
	if resp2["source"] != "cache" {
		t.Errorf("Second request should be from cache, got %v", resp2["source"])
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	h, _ := createTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/order/some_uid", nil)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("POST to /order/{uid} should not return 200")
	}
}

// Benchmarks

func BenchmarkHandler_GetOrder_CacheHit(b *testing.B) {
	h, _ := createTestHandler()
	h.cache.Set(testutil.CreateTestOrder("bench"))
	router := h.Router()
	req := httptest.NewRequest(http.MethodGet, "/order/bench", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}
}

func BenchmarkHandler_HealthCheck(b *testing.B) {
	h, _ := createTestHandler()
	router := h.Router()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}
}
