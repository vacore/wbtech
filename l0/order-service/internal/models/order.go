package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Order - structure of a single order
type Order struct {
	OrderUID          string    `json:"order_uid"`          // unique order id (16 hex, optionally + "test")
	TrackNumber       string    `json:"track_number"`       // Tracking number
	Entry             string    `json:"entry"`              // Selling entry point (WBIL, WBRU, etc.)
	Delivery          Delivery  `json:"delivery"`           // Delivery info
	Payment           Payment   `json:"payment"`            // Payment info
	Items             []Item    `json:"items"`              // List of items in order
	Locale            string    `json:"locale"`             // Locale (en,ru,etc.)
	InternalSignature string    `json:"internal_signature"` // Internal signature (may be empty)
	CustomerID        string    `json:"customer_id"`        // Client ID
	DeliveryService   string    `json:"delivery_service"`   // Delivery service
	ShardKey          string    `json:"shardkey"`           // Sharding key (0-9)
	SmID              int       `json:"sm_id"`              // Sales manager id
	DateCreated       time.Time `json:"date_created"`       // Order creation date
	OofShard          string    `json:"oof_shard"`          // OOF shard
}

// Delivery describes the delivery data of an order
type Delivery struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Zip     string `json:"zip"`
	City    string `json:"city"`
	Address string `json:"address"`
	Region  string `json:"region"`
	Email   string `json:"email"`
}

// Payment describes the payment data of an order
type Payment struct {
	Transaction  string `json:"transaction"`   // Transaction ID (generally = order_uid)
	RequestID    string `json:"request_id"`    // Request ID (may be empty)
	Currency     string `json:"currency"`      // Currency (ISO 4217: USD, RUB, EUR, etc.)
	Provider     string `json:"provider"`      // Payment provider
	Amount       int    `json:"amount"`        // Total sum (goods_total + delivery_cost + custom_fee)
	PaymentDT    int64  `json:"payment_dt"`    // Unix timestamp of this payment
	Bank         string `json:"bank"`          // Bank
	DeliveryCost int    `json:"delivery_cost"` // Delivery cost
	GoodsTotal   int    `json:"goods_total"`   // Total goods cost (total_price of all items)
	CustomFee    int    `json:"custom_fee"`    // Custom fee
}

// Item describes a single item in an order
type Item struct {
	ChrtID      int64  `json:"chrt_id"`      // Characteristic ID
	TrackNumber string `json:"track_number"` // Tracking number
	Price       int    `json:"price"`        // Price before discount
	RID         string `json:"rid"`          // Record ID
	Name        string `json:"name"`         // Item name
	Sale        int    `json:"sale"`         // Discount percent (0-100)
	Size        string `json:"size"`         // Item size (S, M, L, 42, etc.)
	TotalPrice  int    `json:"total_price"`  // Total price = price * (100 - sale) / 100
	NmID        int64  `json:"nm_id"`        // Nomenclature ID
	Brand       string `json:"brand"`        // Brand name
	Status      int    `json:"status"`       // Item status (HTTP-like codes)
}

// Regexps for validation
var (
	// order_uid: 16 hex chars, optionally with "test" postfix
	// Examples: "order_001" or "a1b2c3d4e5f6a7b8test"
	orderUIDRegex = regexp.MustCompile(`^[a-f0-9]{16}(test)?$`)

	// Email
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	// Phone in: +XXXXXXXXXXX (7-15 digits after +)
	phoneRegex = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

	// Currency: 3 capital letters (ISO 4217)
	currencyRegex = regexp.MustCompile(`^[A-Z]{3}$`)

	// Locale: 2 letters or 2 letters + underscore + 2 letters (en, ru, en_US)
	localeRegex = regexp.MustCompile(`^[a-z]{2}(_[A-Z]{2})?$`)
)

// Valid values for enum-like fields
var (
	validEntries          = []string{"WBIL", "WBRU", "WBKZ", "WBBY"}
	validDeliveryServices = []string{"meest", "dpd", "cdek", "boxberry", "pochta", "dhl", "fedex"}
	validProviders        = []string{"wbpay", "stripe", "paypal", "sberbank", "tinkoff", "yoomoney"}
	validBanks            = []string{"alpha", "sber", "tinkoff", "vtb", "raiffeisen", "chase", "hsbc"}
	validCurrencies       = []string{"USD", "RUB", "EUR", "KZT", "BYN", "UAH", "GBP"}
)

// ValidationError contains information about the validation error of a specific field
type ValidationError struct {
	Field   string // Name of the field that caused error
	Message string // Error description
}

// Error implements error interface
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a slice of ValidationError
type ValidationErrors []ValidationError

// Error implements error interface, building all errors in a single string
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate checks order is valid and returns the list of errors
func (o *Order) Validate() error {
	var errs ValidationErrors

	// === Main fields ===

	// order_uid: required, 16 hex chars + optional "test"
	if o.OrderUID == "" {
		errs = append(errs, ValidationError{"order_uid", "is required"})
	} else if !orderUIDRegex.MatchString(o.OrderUID) {
		errs = append(errs, ValidationError{"order_uid", "must be 16 hex characters, optionally followed by 'test'"})
	}

	// track_number: required, 5-30 chars
	if o.TrackNumber == "" {
		errs = append(errs, ValidationError{"track_number", "is required"})
	} else if len(o.TrackNumber) < 5 || len(o.TrackNumber) > 30 {
		errs = append(errs, ValidationError{"track_number", "must be 5-30 characters"})
	}

	// entry: required, from the valid list
	if o.Entry == "" {
		errs = append(errs, ValidationError{"entry", "is required"})
	} else if !contains(validEntries, o.Entry) {
		errs = append(errs, ValidationError{"entry", fmt.Sprintf("must be one of: %v", validEntries)})
	}

	// customer_id: required
	if o.CustomerID == "" {
		errs = append(errs, ValidationError{"customer_id", "is required"})
	}

	// locale: optional, but if present - must be valid
	if o.Locale != "" && !localeRegex.MatchString(o.Locale) {
		errs = append(errs, ValidationError{"locale", "invalid format, expected 'en' or 'en_US'"})
	}

	// shardkey: optional, but if present - must be a single digit 0-9
	if o.ShardKey != "" && (len(o.ShardKey) != 1 || o.ShardKey[0] < '0' || o.ShardKey[0] > '9') {
		errs = append(errs, ValidationError{"shardkey", "must be a single digit 0-9"})
	}

	// sm_id: 0-999
	if o.SmID < 0 || o.SmID > 999 {
		errs = append(errs, ValidationError{"sm_id", "must be between 0 and 999"})
	}

	// date_created: required, not in the future
	if o.DateCreated.IsZero() {
		errs = append(errs, ValidationError{"date_created", "is required"})
	} else if o.DateCreated.After(time.Now().Add(time.Hour)) {
		errs = append(errs, ValidationError{"date_created", "cannot be in the future"})
	}

	// delivery_service: optional, but if present - from the list
	if o.DeliveryService != "" && !contains(validDeliveryServices, o.DeliveryService) {
		errs = append(errs, ValidationError{"delivery_service", fmt.Sprintf("must be one of: %v", validDeliveryServices)})
	}

	// === Nested structures ===

	// Delivery
	if deliveryErrs := o.Delivery.Validate(); len(deliveryErrs) > 0 {
		errs = append(errs, deliveryErrs...)
	}

	// Payments
	if paymentErrs := o.Payment.Validate(); len(paymentErrs) > 0 {
		errs = append(errs, paymentErrs...)
	}

	// Items
	if len(o.Items) == 0 {
		errs = append(errs, ValidationError{"items", "at least one item is required"})
	} else {
		var calculatedTotal int
		for i, item := range o.Items {
			if itemErrs := item.Validate(i); len(itemErrs) > 0 {
				errs = append(errs, itemErrs...)
			}
			calculatedTotal += item.TotalPrice
		}

		// Check goods_total mathces total_price of all items
		if o.Payment.GoodsTotal != calculatedTotal {
			errs = append(errs, ValidationError{"payment.goods_total",
				fmt.Sprintf("mismatch: expected %d (sum of items), got %d", calculatedTotal, o.Payment.GoodsTotal)})
		}

		// Check amount = goods_total + delivery_cost + custom_fee
		expectedAmount := o.Payment.GoodsTotal + o.Payment.DeliveryCost + o.Payment.CustomFee
		if o.Payment.Amount != expectedAmount {
			errs = append(errs, ValidationError{"payment.amount",
				fmt.Sprintf("mismatch: expected %d (goods + delivery + custom), got %d",
					expectedAmount, o.Payment.Amount)})
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// Validate checks Delivery is valid and returns the list of errors
func (d *Delivery) Validate() ValidationErrors {
	var errs ValidationErrors

	// name: required, 2-100 chars
	if d.Name == "" {
		errs = append(errs, ValidationError{"delivery.name", "is required"})
	} else if len(d.Name) < 2 || len(d.Name) > 100 {
		errs = append(errs, ValidationError{"delivery.name", "must be 2-100 characters"})
	}

	// phone: optional, but if present - must be in international format
	if d.Phone != "" && !phoneRegex.MatchString(d.Phone) {
		errs = append(errs, ValidationError{"delivery.phone", "invalid format, expected +XXXXXXXXXXX"})
	}

	// email: optional, but if present - must be valid
	if d.Email != "" && !emailRegex.MatchString(d.Email) {
		errs = append(errs, ValidationError{"delivery.email", "invalid email format"})
	}

	// city: required
	if d.City == "" {
		errs = append(errs, ValidationError{"delivery.city", "is required"})
	}

	// address: required
	if d.Address == "" {
		errs = append(errs, ValidationError{"delivery.address", "is required"})
	}

	return errs
}

// Validate checks Payment is valid and returns the list of errors
func (p *Payment) Validate() ValidationErrors {
	var errs ValidationErrors

	// transaction: required
	if p.Transaction == "" {
		errs = append(errs, ValidationError{"payment.transaction", "is required"})
	}

	// currency: required, ISO 4217
	if p.Currency == "" {
		errs = append(errs, ValidationError{"payment.currency", "is required"})
	} else if !currencyRegex.MatchString(p.Currency) {
		errs = append(errs, ValidationError{"payment.currency", "must be 3 uppercase letters (ISO 4217)"})
	} else if !contains(validCurrencies, p.Currency) {
		errs = append(errs, ValidationError{"payment.currency", fmt.Sprintf("must be one of: %v", validCurrencies)})
	}

	// provider: optional, but if present - from the list
	if p.Provider != "" && !contains(validProviders, p.Provider) {
		errs = append(errs, ValidationError{"payment.provider", fmt.Sprintf("must be one of: %v", validProviders)})
	}

	// amount: non-negative
	if p.Amount < 0 {
		errs = append(errs, ValidationError{"payment.amount", "cannot be negative"})
	}

	// payment_dt: valid Unix timestamp, not in the future
	if p.PaymentDT <= 0 {
		errs = append(errs, ValidationError{"payment.payment_dt", "must be a valid unix timestamp"})
	} else if p.PaymentDT > time.Now().Add(time.Hour).Unix() {
		errs = append(errs, ValidationError{"payment.payment_dt", "cannot be in the future"})
	}

	// delivery_cost: non-negative
	if p.DeliveryCost < 0 {
		errs = append(errs, ValidationError{"payment.delivery_cost", "cannot be negative"})
	}

	// goods_total: non-negative
	if p.GoodsTotal < 0 {
		errs = append(errs, ValidationError{"payment.goods_total", "cannot be negative"})
	}

	// custom_fee: non-negative
	if p.CustomFee < 0 {
		errs = append(errs, ValidationError{"payment.custom_fee", "cannot be negative"})
	}

	// bank: optional, but if present - from the list
	if p.Bank != "" && !contains(validBanks, p.Bank) {
		errs = append(errs, ValidationError{"payment.bank", fmt.Sprintf("must be one of: %v", validBanks)})
	}

	return errs
}

// Validate checks Item is valid and returns the list of errors
func (item *Item) Validate(index int) ValidationErrors {
	var errs ValidationErrors
	prefix := fmt.Sprintf("items[%d]", index)

	// chrt_id: positive
	if item.ChrtID <= 0 {
		errs = append(errs, ValidationError{prefix + ".chrt_id", "must be positive"})
	}

	// name: required
	if item.Name == "" {
		errs = append(errs, ValidationError{prefix + ".name", "is required"})
	}

	// price: non-negative
	if item.Price < 0 {
		errs = append(errs, ValidationError{prefix + ".price", "cannot be negative"})
	}

	// sale: 0-100 percent
	if item.Sale < 0 || item.Sale > 100 {
		errs = append(errs, ValidationError{prefix + ".sale", "must be between 0 and 100"})
	}

	// total_price: non-negative
	if item.TotalPrice < 0 {
		errs = append(errs, ValidationError{prefix + ".total_price", "cannot be negative"})
	}

	// Check total_price is correct
	expectedTotal := item.Price - (item.Price * item.Sale / 100)
	if item.TotalPrice != expectedTotal {
		errs = append(errs, ValidationError{prefix + ".total_price",
			fmt.Sprintf("mismatch: expected %d (price - sale%%), got %d", expectedTotal, item.TotalPrice)})
	}

	// nm_id: positive
	if item.NmID <= 0 {
		errs = append(errs, ValidationError{prefix + ".nm_id", "must be positive"})
	}

	// brand: required
	if item.Brand == "" {
		errs = append(errs, ValidationError{prefix + ".brand", "is required"})
	}

	// status: HTTP-like codes 0-599
	if item.Status < 0 || item.Status > 599 {
		errs = append(errs, ValidationError{prefix + ".status", "must be between 0 and 599"})
	}

	return errs
}

// ParseOrder parses JSON-data into an Order struct and validates it
func ParseOrder(data []byte) (*Order, error) {
	var order Order
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}
	if err := order.Validate(); err != nil {
		return nil, fmt.Errorf("validating order: %w", err)
	}
	return &order, nil
}

// contains checks if a slice contains a string (case-insensitive)
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
