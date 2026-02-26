package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"

	"order-service/internal/models"
)

const (
	Reset    = "\033[0m"
	Red      = "\033[31m"
	Green    = "\033[32m"
	Yellow   = "\033[33m"
	Blue     = "\033[34m"
	Purple   = "\033[35m"
	Cyan     = "\033[36m"
	Gray     = "\033[90m"
	opCreate = "CREATE"
	opRead   = "READ"
	opUpdate = "UPDATE"
	opDelete = "DELETE"
)

type SimConfig struct {
	KafkaBroker string
	KafkaTopic  string
	ServiceURL  string
	Workers     int
	MinDelay    time.Duration
	MaxDelay    time.Duration
	InvalidPct  float64
	SeedCount   int
}

type Stats struct {
	Creates    atomic.Int64
	Reads      atomic.Int64
	Updates    atomic.Int64
	Deletes    atomic.Int64
	InvalidOps atomic.Int64
	Errors     atomic.Int64
}

func (st *Stats) Total() int64 {
	return st.Creates.Load() + st.Reads.Load() + st.Updates.Load() +
		st.Deletes.Load() + st.InvalidOps.Load()
}

type trackedOrder struct {
	UID         string
	TrackNumber string
}

type Simulator struct {
	writer     *kafka.Writer
	httpClient *http.Client
	httpBase   string

	mu     sync.RWMutex
	orders []trackedOrder

	printMu sync.Mutex // serializes log output

	config SimConfig
	stats  Stats
}

// Dummy data

var (
	names = []string{
		"Иван Петров", "Мария Сидорова", "Алексей Козлов",
		"Елена Новикова", "John Smith", "Jane Doe",
		"Robert Johnson", "Emily Brown", "Michael Davis",
	}

	cities = []struct {
		City, Region, Zip, Country string
	}{
		{"Москва", "Московская область", "101000", "RU"},
		{"Санкт-Петербург", "Ленинградская область", "190000", "RU"},
		{"Новосибирск", "Новосибирская область", "630000", "RU"},
		{"New York", "New York", "10001", "US"},
		{"London", "Greater London", "SW1A1AA", "GB"},
		{"Tel Aviv", "Tel Aviv District", "6100000", "IL"},
	}

	streets = []string{
		"ул. Ленина", "ул. Пушкина", "пр. Мира",
		"Main Street", "Oak Avenue", "Broadway",
	}

	brands = []struct {
		Name     string
		Products []string
	}{
		{"Nike", []string{"Air Max 90", "Air Force 1", "Dunk Low"}},
		{"Adidas", []string{"Ultraboost", "Stan Smith", "Superstar"}},
		{"Puma", []string{"RS-X", "Suede Classic", "Future Rider"}},
		{"Samsung", []string{"Galaxy S24", "Galaxy Buds", "Galaxy Watch"}},
		{"Apple", []string{"iPhone 15", "AirPods Pro", "iPad Pro"}},
		{"Vivienne Sabo", []string{"Mascara Cabaret", "Lip Gloss", "Eye Shadow Palette"}},
	}

	sizes            = []string{"XS", "S", "M", "L", "XL", "36", "38", "40", "42", "44"}
	entries          = []string{"WBIL", "WBRU", "WBKZ", "WBBY"}
	deliveryServices = []string{"meest", "dpd", "cdek", "boxberry", "pochta", "dhl", "fedex"}
	providers        = []string{"wbpay", "stripe", "paypal", "sberbank", "tinkoff", "yoomoney"}
	banks            = []string{"alpha", "sber", "tinkoff", "vtb", "raiffeisen", "chase", "hsbc"}
	locales          = []string{"ru", "en", "kz"}
	itemStatuses     = []int{100, 200, 201, 202, 300}
)

func main() {
	cfg := loadConfig()

	fmt.Printf("\n%s=== ORDER SERVICE SIMULATOR ===%s\n", Purple, Reset)
	fmt.Printf("  Kafka:      %s (topic: %s)\n", cfg.KafkaBroker, cfg.KafkaTopic)
	fmt.Printf("  Service:    %s\n", cfg.ServiceURL)
	fmt.Printf("  Workers:    %d\n", cfg.Workers)
	fmt.Printf("  Delay:      %s – %s\n", cfg.MinDelay, cfg.MaxDelay)
	fmt.Printf("  Invalid:    %.0f%%\n", cfg.InvalidPct)
	fmt.Printf("  Seed:       %d orders\n", cfg.SeedCount)
	fmt.Println()

	sim := NewSimulator(cfg)
	defer sim.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for order-service to be ready
	sim.waitForService(ctx)

	// Seed phase - sequential, creates initial orders for update/read/delete
	sim.seed(ctx)

	// Start workers
	var wg sync.WaitGroup
	sim.log(-1, Green, "+OK", "Starting %d workers (delay: %s–%s, invalid: %.0f%%)",
		cfg.Workers, cfg.MinDelay, cfg.MaxDelay, cfg.InvalidPct)
	fmt.Println()

	startTime := time.Now()

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go sim.worker(ctx, i, &wg)
	}

	go sim.statsReporter(ctx, startTime)

	// Wait for shutdown signal
	<-sigChan
	fmt.Printf("\n%s>>> Shutting down... waiting for workers to finish%s\n", Yellow, Reset)
	cancel()
	wg.Wait()

	// Final report
	time.Sleep(100 * time.Millisecond) // let output flush
	sim.printFinalStats(time.Since(startTime))
}

// Setup

func loadConfig() SimConfig {
	return SimConfig{
		KafkaBroker: envOrDefault("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:  envOrDefault("KAFKA_TOPIC", "orders"),
		ServiceURL:  envOrDefault("ORDER_SERVICE_URL", "http://localhost:8081"),
		Workers:     envOrDefaultInt("SIM_WORKERS", 16),
		MinDelay:    envOrDefaultDuration("SIM_MIN_DELAY", 500*time.Millisecond),
		MaxDelay:    envOrDefaultDuration("SIM_MAX_DELAY", 3*time.Second),
		InvalidPct:  envOrDefaultFloat("SIM_INVALID_PCT", 15.0),
		SeedCount:   envOrDefaultInt("SIM_SEED_COUNT", 5),
	}
}

func NewSimulator(cfg SimConfig) *Simulator {
	return &Simulator{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(cfg.KafkaBroker),
			Topic:        cfg.KafkaTopic,
			Balancer:     &kafka.LeastBytes{},
			WriteTimeout: 10 * time.Second,
			RequiredAcks: kafka.RequireOne,
		},
		httpClient: &http.Client{Timeout: 10 * time.Second},
		httpBase:   cfg.ServiceURL,
		orders:     make([]trackedOrder, 0, 256),
		config:     cfg,
	}
}

func (s *Simulator) Close() {
	if err := s.writer.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "kafka writer close error: %v\n", err)
	}
}

func (s *Simulator) waitForService(ctx context.Context) {
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.httpBase+"/health", nil)
		if err != nil {
			return
		}

		resp, err := s.httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				s.log(-1, Green, "+OK", "Service is ready at %s", s.httpBase)
				return
			}
		}
		if i == 0 {
			s.log(-1, Yellow, "...", "Waiting for service at %s...", s.httpBase)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	s.log(-1, Yellow, "WRN", "Service may not be ready, proceeding anyway")
}

func (s *Simulator) seed(ctx context.Context) {
	fmt.Printf("\n%s=== SEED PHASE ===%s\n\n", Purple, Reset)

	rng := mrand.New(mrand.NewSource(time.Now().UnixNano())) //nolint:gosec

	msgs := make([]kafka.Message, 0, s.config.SeedCount)
	for i := 0; i < s.config.SeedCount; i++ {
		order := s.generateOrder(rng)
		// Offset each order by 1ms so date_created is unique and sort is deterministic
		order.DateCreated = order.DateCreated.Add(time.Duration(i) * time.Millisecond)
		order.Payment.PaymentDT = order.DateCreated.Unix()

		data, _ := json.Marshal(order)

		msgs = append(msgs, kafka.Message{
			Key:   []byte(order.OrderUID),
			Value: data,
		})

		s.mu.Lock()
		s.orders = append(s.orders, trackedOrder{
			UID:         order.OrderUID,
			TrackNumber: order.TrackNumber,
		})
		s.mu.Unlock()

		s.stats.Creates.Add(1)
		s.log(-1, Green, "+OK", "CREATE  %s (items:%d amt:%d %s)",
			order.OrderUID, len(order.Items), order.Payment.Amount, order.Payment.Currency)
	}

	if err := s.writer.WriteMessages(ctx, msgs...); err != nil {
		s.log(-1, Red, "ERR", "SEED batch write failed: %v", err)
		return
	}

	fmt.Printf("\n%s=== SEED COMPLETE: %d orders ===%s\n\n", Green, len(s.orders), Reset)
}

func (s *Simulator) worker(ctx context.Context, id int, wg *sync.WaitGroup) {
	defer wg.Done()

	// See note in seed
	rng := mrand.New(mrand.NewSource(time.Now().UnixNano() + int64(id)*1337)) //nolint:gosec

	for {
		delay := s.randomDelay(rng)
		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		s.doOperation(ctx, rng, id)
	}
}

func (s *Simulator) doOperation(ctx context.Context, rng *mrand.Rand, wid int) {
	if ctx.Err() != nil {
		return
	}

	op := s.pickOperation(rng)

	// With configured probability, make it invalid
	if rng.Float64()*100 < s.config.InvalidPct {
		s.doInvalidOp(ctx, rng, op, wid)
		return
	}

	switch op {
	case opCreate:
		s.doCreate(ctx, rng, wid)
	case opRead:
		s.doRead(ctx, rng, wid)
	case opUpdate:
		s.doUpdate(ctx, rng, wid)
	case opDelete:
		s.doDelete(ctx, rng, wid)
	}
}

func (s *Simulator) pickOperation(rng *mrand.Rand) string {
	s.mu.RLock()
	count := len(s.orders)
	s.mu.RUnlock()

	if count == 0 {
		return opCreate
	}

	r := rng.Float64()

	if count < 3 {
		// Build up the pool first
		switch {
		case r < 0.60:
			return opCreate
		case r < 0.80:
			return opRead
		case r < 0.95:
			return opUpdate
		default:
			return opDelete
		}
	}

	// Normal distribution
	switch {
	case r < 0.30:
		return opCreate // 30%
	case r < 0.55:
		return opRead // 25%
	case r < 0.80:
		return opUpdate // 25%
	default:
		return opDelete // 20%
	}
}

// Normal CRUD operations

func (s *Simulator) doCreate(ctx context.Context, rng *mrand.Rand, wid int) {
	order := s.generateOrder(rng)
	data, _ := json.Marshal(order)

	err := s.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(order.OrderUID),
		Value: data,
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.stats.Errors.Add(1)
		s.log(wid, Red, "ERR", "CREATE FAIL: %v", err)
		return
	}

	s.mu.Lock()
	s.orders = append(s.orders, trackedOrder{
		UID:         order.OrderUID,
		TrackNumber: order.TrackNumber,
	})
	s.mu.Unlock()

	s.stats.Creates.Add(1)
	s.log(wid, Green, "+OK", "CREATE  %s (items:%d amt:%d %s)",
		order.OrderUID, len(order.Items), order.Payment.Amount, order.Payment.Currency)
}

func (s *Simulator) doRead(ctx context.Context, rng *mrand.Rand, wid int) {
	uid := s.pickRandomOrderUID(rng)
	if uid == "" {
		s.doCreate(ctx, rng, wid) // nothing to read yet
		return
	}

	reqURL := fmt.Sprintf("%s/order/%s", s.httpBase, uid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		s.stats.Errors.Add(1)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.stats.Errors.Add(1)
		s.log(wid, Red, "ERR", "READ FAIL: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.ReadAll(resp.Body)
		// Order might have been deleted by another worker - not really an error
		s.stats.Reads.Add(1)
		s.log(wid, Yellow, "DEL", "READ    %s (HTTP %d - likely deleted)", uid, resp.StatusCode)
		return
	}

	var result struct {
		Source string `json:"source"`
		Time   string `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.log(wid, Yellow, "DEL", "READ    %s (response decode error: %v)", uid, err)
		s.stats.Reads.Add(1)
		return
	}

	s.stats.Reads.Add(1)
	s.log(wid, Blue, "GET", "READ    %s (%s %s)", uid, result.Source, result.Time)
}

func (s *Simulator) doUpdate(ctx context.Context, rng *mrand.Rand, wid int) {
	target := s.pickRandomTrackedOrder(rng)
	if target == nil {
		s.doCreate(ctx, rng, wid)
		return
	}

	// Generate a new order body, keep the same UID and track number
	order := s.generateOrder(rng)
	order.OrderUID = target.UID
	order.Payment.Transaction = target.UID
	order.TrackNumber = target.TrackNumber
	for i := range order.Items {
		order.Items[i].TrackNumber = target.TrackNumber
	}

	modDesc := s.applyRandomMod(rng, &order)
	recalcTotals(&order)

	data, _ := json.Marshal(order)
	err := s.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(order.OrderUID),
		Value: data,
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.stats.Errors.Add(1)
		s.log(wid, Red, "ERR", "UPDATE FAIL: %v", err)
		return
	}

	s.stats.Updates.Add(1)
	s.log(wid, Cyan, "UPD", "UPDATE  %s (%s)", order.OrderUID, modDesc)
}

func (s *Simulator) doDelete(ctx context.Context, rng *mrand.Rand, wid int) {
	target := s.removeRandomTrackedOrder(rng)
	if target == nil {
		return // nothing to delete
	}

	reqURL := fmt.Sprintf("%s/order/%s", s.httpBase, target.UID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		s.stats.Errors.Add(1)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.stats.Errors.Add(1)
		s.log(wid, Red, "ERR", "DELETE FAIL: %v", err)
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		s.stats.Deletes.Add(1)
		s.log(wid, Yellow, "DEL", "DELETE  %s", target.UID)
	} else {
		s.stats.Errors.Add(1)
		s.log(wid, Red, "ERR", "DELETE HTTP %d: %s", resp.StatusCode, target.UID)
	}
}

// Invlid operations

func (s *Simulator) doInvalidOp(ctx context.Context, rng *mrand.Rand, op string, wid int) {
	switch op {
	case opCreate:
		s.doInvalidCreate(ctx, rng, wid)
	case opRead:
		s.doInvalidRead(ctx, rng, wid)
	case opUpdate:
		s.doInvalidUpdate(ctx, rng, wid)
	case opDelete:
		s.doInvalidDelete(ctx, rng, wid)
	}
}

// buildCorruptedOrder generates a valid order, applies corruption, and marshals to JSON.
func (s *Simulator) buildCorruptedOrder(rng *mrand.Rand, corrupt func(*models.Order)) []byte {
	order := s.generateOrder(rng)
	corrupt(&order)
	data, _ := json.Marshal(order)
	return data
}

// sendInvalidMessage sends an invalid message to Kafka and records stats.
func (s *Simulator) sendInvalidMessage(ctx context.Context, wid int, op, key string, value []byte, desc string) {
	err := s.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: value,
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.stats.Errors.Add(1)
		return
	}

	s.stats.InvalidOps.Add(1)
	s.log(wid, Purple, "INV", "INV.%-6s %s", op, desc)
}

func (s *Simulator) doInvalidCreate(ctx context.Context, rng *mrand.Rand, wid int) {
	var key, desc string
	var value []byte

	switch rng.Intn(6) {
	case 0:
		// Raw broken bytes
		key = "bad_json"
		value = []byte(`{this is not valid json!!!}`)
		desc = "malformed JSON"

	case 1:
		key = "empty_uid"
		value = s.buildCorruptedOrder(rng, func(o *models.Order) {
			o.OrderUID = ""
			o.Payment.Transaction = ""
		})
		desc = "empty order_uid"

	case 2:
		key = "no_items"
		value = s.buildCorruptedOrder(rng, func(o *models.Order) {
			o.Items = []models.Item{}
			o.Payment.GoodsTotal = 0
			o.Payment.Amount = o.Payment.DeliveryCost + o.Payment.CustomFee
		})
		desc = "empty items"

	case 3:
		key = "neg_price"
		value = s.buildCorruptedOrder(rng, func(o *models.Order) {
			for i := range o.Items {
				o.Items[i].Price = -100
				o.Items[i].TotalPrice = -100
			}
			o.Payment.GoodsTotal = -100 * len(o.Items)
			o.Payment.Amount = o.Payment.GoodsTotal + o.Payment.DeliveryCost + o.Payment.CustomFee
		})
		desc = "negative price"

	case 4:
		key = "bad_currency"
		value = s.buildCorruptedOrder(rng, func(o *models.Order) {
			o.Payment.Currency = "FAKE"
		})
		desc = "invalid currency"

	case 5:
		key = "amount_mismatch"
		value = s.buildCorruptedOrder(rng, func(o *models.Order) {
			o.Payment.Amount = o.Payment.Amount + 99999
		})
		desc = "amount mismatch"
	}

	s.sendInvalidMessage(ctx, wid, "CREATE", key, value, desc)
}

func (s *Simulator) doInvalidUpdate(ctx context.Context, rng *mrand.Rand, wid int) {
	target := s.pickRandomTrackedOrder(rng)

	var key, desc string
	var value []byte

	if target != nil {
		switch rng.Intn(3) {
		case 0:
			// Raw broken bytes - intentionally not generated from a struct
			key = target.UID
			value = []byte(`{completely broken json for update`)
			desc = fmt.Sprintf("malformed JSON for %s", truncate(target.UID, 20))

		case 1:
			key = target.UID
			value = s.buildCorruptedOrder(rng, func(o *models.Order) {
				o.OrderUID = target.UID
				o.Payment.Transaction = target.UID
				o.TrackNumber = target.TrackNumber
				o.Items = []models.Item{}
				o.Payment.GoodsTotal = 0
				o.Payment.Amount = o.Payment.DeliveryCost + o.Payment.CustomFee
			})
			desc = fmt.Sprintf("empty items for %s", truncate(target.UID, 20))

		case 2:
			key = target.UID
			value = s.buildCorruptedOrder(rng, func(o *models.Order) {
				o.OrderUID = target.UID
				o.Entry = "INVALID"
				o.CustomerID = ""
				o.Delivery.Name = ""
				o.Delivery.City = ""
				o.Delivery.Address = ""
				o.Payment.Transaction = ""
				o.Payment.Currency = "XXX"
				o.Payment.PaymentDT = 0
				o.Items = []models.Item{{
					ChrtID:     -1,
					Name:       "",
					Price:      -500,
					Sale:       200,
					TotalPrice: -500,
					NmID:       0,
					Brand:      "",
					Status:     999,
				}}
			})
			desc = fmt.Sprintf("all-invalid fields for %s", truncate(target.UID, 20))
		}
	} else {
		// No tracked orders to target - send garbage
		key = "orphan_update"
		value = []byte(`{not even json`)
		desc = "orphan malformed update"
	}

	s.sendInvalidMessage(ctx, wid, "UPDATE", key, value, desc)
}

func (s *Simulator) doInvalidRead(ctx context.Context, rng *mrand.Rand, wid int) {
	var uid, desc string

	switch rng.Intn(3) {
	case 0:
		uid = randomHex(40)
		desc = "nonexistent UID"
	case 1:
		uid = "x"
		desc = "too-short UID"
	case 2:
		uid = strings.Repeat("z", 200)
		desc = "extremely long UID"
	}

	reqURL := fmt.Sprintf("%s/order/%s", s.httpBase, uid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		s.stats.Errors.Add(1)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.stats.Errors.Add(1)
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	s.stats.InvalidOps.Add(1)
	s.log(wid, Purple, "INV", "INV.READ:   %s (HTTP %d)", desc, resp.StatusCode)
}

func (s *Simulator) doInvalidDelete(ctx context.Context, rng *mrand.Rand, wid int) {
	var uid, desc string

	switch rng.Intn(3) {
	case 0:
		uid = randomHex(40)
		desc = "nonexistent UID"
	case 1:
		uid = "already-deleted-fake-uid"
		desc = "fake UID"
	case 2:
		uid = "0000000000000000"
		desc = "zeroed UID"
	}

	reqURL := fmt.Sprintf("%s/order/%s", s.httpBase, uid)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		s.stats.Errors.Add(1)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.stats.Errors.Add(1)
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	s.stats.InvalidOps.Add(1)
	s.log(wid, Purple, "INV", "INV.DELETE: %s (HTTP %d)", desc, resp.StatusCode)
}

// Order generation

func (s *Simulator) generateOrder(rng *mrand.Rand) models.Order {
	uid := generateOrderUID()
	trackNumber := fmt.Sprintf("WBIL%s%05d", randomHex(4), rng.Intn(100000))
	cityInfo := cities[rng.Intn(len(cities))]

	itemCount := 1 + rng.Intn(4)
	items := make([]models.Item, itemCount)
	for i := range items {
		items[i] = s.generateItem(rng, trackNumber)
	}

	goodsTotal := 0
	for _, item := range items {
		goodsTotal += item.TotalPrice
	}

	deliveryCost := (rng.Intn(30) + 10) * 100
	if goodsTotal > 500000 {
		deliveryCost = 0
	}

	customFee := 0
	if cityInfo.Country != "RU" && rng.Float32() < 0.3 {
		customFee = rng.Intn(50) * 100
	}

	now := time.Now()

	return models.Order{
		OrderUID:    uid,
		TrackNumber: trackNumber,
		Entry:       entries[rng.Intn(len(entries))],
		Delivery: models.Delivery{
			Name:    names[rng.Intn(len(names))],
			Phone:   fmt.Sprintf("+79%09d", rng.Intn(1000000000)),
			Zip:     cityInfo.Zip,
			City:    cityInfo.City,
			Address: fmt.Sprintf("%s, д. %d, кв. %d", streets[rng.Intn(len(streets))], rng.Intn(150)+1, rng.Intn(500)+1),
			Region:  cityInfo.Region,
			Email:   fmt.Sprintf("customer%d@example.com", rng.Intn(10000)),
		},
		Payment: models.Payment{
			Transaction:  uid,
			Currency:     "RUB",
			Provider:     providers[rng.Intn(len(providers))],
			Amount:       goodsTotal + deliveryCost + customFee,
			PaymentDT:    now.Unix(),
			Bank:         banks[rng.Intn(len(banks))],
			DeliveryCost: deliveryCost,
			GoodsTotal:   goodsTotal,
			CustomFee:    customFee,
		},
		Items:             items,
		Locale:            locales[rng.Intn(len(locales))],
		InternalSignature: "",
		CustomerID:        fmt.Sprintf("cust_%s", randomHex(8)),
		DeliveryService:   deliveryServices[rng.Intn(len(deliveryServices))],
		ShardKey:          fmt.Sprintf("%d", rng.Intn(10)),
		SmID:              rng.Intn(100),
		DateCreated:       now,
		OofShard:          fmt.Sprintf("%d", rng.Intn(10)),
	}
}

func (s *Simulator) generateItem(rng *mrand.Rand, trackNumber string) models.Item {
	brand := brands[rng.Intn(len(brands))]
	product := brand.Products[rng.Intn(len(brand.Products))]
	price := (rng.Intn(495) + 5) * 100
	sale := rng.Intn(71)
	totalPrice := price - (price * sale / 100)

	return models.Item{
		ChrtID:      int64(rng.Intn(90000000) + 10000000),
		TrackNumber: trackNumber,
		Price:       price,
		RID:         fmt.Sprintf("%stest", randomHex(16)),
		Name:        product,
		Sale:        sale,
		Size:        sizes[rng.Intn(len(sizes))],
		TotalPrice:  totalPrice,
		NmID:        int64(rng.Intn(90000000) + 10000000),
		Brand:       brand.Name,
		Status:      itemStatuses[rng.Intn(len(itemStatuses))],
	}
}

func (s *Simulator) applyRandomMod(rng *mrand.Rand, order *models.Order) string {
	switch rng.Intn(5) {
	case 0:
		extra := s.generateItem(rng, order.TrackNumber)
		order.Items = append(order.Items, extra)
		return fmt.Sprintf("+item → %d total", len(order.Items))
	case 1:
		city := cities[rng.Intn(len(cities))]
		order.Delivery.City = city.City
		order.Delivery.Region = city.Region
		return fmt.Sprintf("city → %s", city.City)
	case 2:
		order.CustomerID = fmt.Sprintf("upd_%s", randomHex(6))
		return fmt.Sprintf("customer → %s", order.CustomerID)
	case 3:
		if len(order.Items) > 0 {
			sale := 10 + rng.Intn(60)
			order.Items[0].Sale = sale
			return fmt.Sprintf("sale → %d%%", sale)
		}
		return "no-op"
	case 4:
		order.Delivery.Name = names[rng.Intn(len(names))]
		return fmt.Sprintf("name → %s", order.Delivery.Name)
	}
	return "modified"
}

func recalcTotals(order *models.Order) {
	goodsTotal := 0
	for i := range order.Items {
		item := &order.Items[i]
		item.TotalPrice = item.Price - (item.Price * item.Sale / 100)
		goodsTotal += item.TotalPrice
	}
	order.Payment.GoodsTotal = goodsTotal
	order.Payment.Amount = goodsTotal + order.Payment.DeliveryCost + order.Payment.CustomFee
}

// Tracked order helpers (thread-safe)

func (s *Simulator) pickRandomOrderUID(rng *mrand.Rand) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.orders) == 0 {
		return ""
	}
	return s.orders[rng.Intn(len(s.orders))].UID
}

func (s *Simulator) pickRandomTrackedOrder(rng *mrand.Rand) *trackedOrder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.orders) == 0 {
		return nil
	}
	o := s.orders[rng.Intn(len(s.orders))]
	return &o // return copy
}

func (s *Simulator) removeRandomTrackedOrder(rng *mrand.Rand) *trackedOrder {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.orders) == 0 {
		return nil
	}
	idx := rng.Intn(len(s.orders))
	o := s.orders[idx]
	s.orders[idx] = s.orders[len(s.orders)-1] // swap with last (O(1) removal)
	s.orders = s.orders[:len(s.orders)-1]
	return &o
}

// Stats & logging

func (s *Simulator) statsReporter(ctx context.Context, startTime time.Time) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.printStats(time.Since(startTime))
		}
	}
}

func (s *Simulator) printStats(elapsed time.Duration) {
	c := s.stats.Creates.Load()
	r := s.stats.Reads.Load()
	u := s.stats.Updates.Load()
	d := s.stats.Deletes.Load()
	inv := s.stats.InvalidOps.Load()
	errs := s.stats.Errors.Load()
	total := s.stats.Total()

	s.mu.RLock()
	pool := len(s.orders)
	s.mu.RUnlock()

	opsPerSec := float64(total) / elapsed.Seconds()

	line := fmt.Sprintf(
		"\n%s── STATS ── C:%d R:%d U:%d D:%d │ Inv:%d Err:%d │ Total:%d │ Pool:%d │ %.1f ops/s │ %s ──%s\n",
		Gray, c, r, u, d, inv, errs, total, pool, opsPerSec,
		elapsed.Round(time.Second), Reset)

	s.printMu.Lock()
	fmt.Print(line)
	s.printMu.Unlock()
}

func (s *Simulator) printFinalStats(elapsed time.Duration) {
	c := s.stats.Creates.Load()
	r := s.stats.Reads.Load()
	u := s.stats.Updates.Load()
	d := s.stats.Deletes.Load()
	inv := s.stats.InvalidOps.Load()
	errs := s.stats.Errors.Load()
	total := s.stats.Total()

	s.mu.RLock()
	pool := len(s.orders)
	s.mu.RUnlock()

	opsPerSec := float64(total) / elapsed.Seconds()

	fmt.Printf("%s=== SIMULATION SUMMARY ===%s\n", Purple, Reset)
	fmt.Printf("  Duration:    %s\n", elapsed.Round(time.Second))
	fmt.Printf("  Throughput:  %.1f ops/sec\n\n", opsPerSec)
	fmt.Printf("  %s Creates:  %5d  (%s)%s\n", Green, c, pct(c, total), Reset)
	fmt.Printf("  %s Reads:    %5d  (%s)%s\n", Blue, r, pct(r, total), Reset)
	fmt.Printf("  %s Updates:  %5d  (%s)%s\n", Cyan, u, pct(u, total), Reset)
	fmt.Printf("  %s Deletes:  %5d  (%s)%s\n", Yellow, d, pct(d, total), Reset)
	fmt.Printf("  %s Invalid:  %5d  (%s)%s\n", Purple, inv, pct(inv, total), Reset)
	fmt.Printf("  %s Errors:   %5d%s\n", Red, errs, Reset)
	fmt.Printf("  %s── Total:   %5d%s\n", Gray, total, Reset)
	fmt.Printf("  %s── Pool:    %5d orders%s\n", Gray, pool, Reset)
	fmt.Println()
}

func (s *Simulator) log(workerID int, color, tag, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("15:04:05")

	var prefix string
	if workerID < 0 {
		prefix = "SEED"
	} else {
		prefix = fmt.Sprintf("W%02d", workerID)
	}

	line := fmt.Sprintf("%s %s[%s] [%-3s] %s%s\n", ts, color, prefix, tag, msg, Reset)

	s.printMu.Lock()
	fmt.Print(line)
	s.printMu.Unlock()
}

// Utilities

func (s *Simulator) randomDelay(rng *mrand.Rand) time.Duration {
	min := s.config.MinDelay
	max := s.config.MaxDelay
	if max <= min {
		return min
	}
	delta := int64(max - min)
	return min + time.Duration(rng.Int63n(delta))
}

func generateOrderUID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + "test"
}

func randomHex(length int) string {
	b := make([]byte, (length+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:length]
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func pct(n, total int64) string {
	if total == 0 {
		return "  0.0%"
	}
	return fmt.Sprintf("%5.1f%%", float64(n)/float64(total)*100)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrDefaultInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envOrDefaultFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envOrDefaultDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
