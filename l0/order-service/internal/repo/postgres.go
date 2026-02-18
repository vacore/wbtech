package repo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"order-service/internal/config"
	"order-service/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrOrderNotFound is returned when order is not found in a DB
var ErrOrderNotFound = errors.New("order not found")

// SQL fragments shared between GetOrder and GetAllOrders.
// COALESCE ensures NULL-safe scanning into Go zero-value types,
// which is necessary when LEFT JOIN doesn't find a matching row.

const orderSelectColumns = `
	o.order_uid, o.track_number, o.entry,
	COALESCE(o.locale, ''), COALESCE(o.internal_signature, ''),
	o.customer_id, COALESCE(o.delivery_service, ''),
	COALESCE(o.shardkey, ''), COALESCE(o.sm_id, 0),
	o.date_created, COALESCE(o.oof_shard, ''),
	COALESCE(d.name, ''), COALESCE(d.phone, ''), COALESCE(d.zip, ''),
	COALESCE(d.city, ''), COALESCE(d.address, ''),
	COALESCE(d.region, ''), COALESCE(d.email, ''),
	COALESCE(p.transaction, ''), COALESCE(p.request_id, ''),
	COALESCE(p.currency, ''), COALESCE(p.provider, ''),
	COALESCE(p.amount, 0), COALESCE(p.payment_dt, 0),
	COALESCE(p.bank, ''), COALESCE(p.delivery_cost, 0),
	COALESCE(p.goods_total, 0), COALESCE(p.custom_fee, 0)`

const orderFromJoin = `
	FROM orders o
	LEFT JOIN deliveries d ON o.order_uid = d.order_uid
	LEFT JOIN payments p ON o.order_uid = p.order_uid`

const itemSelectColumns = `
	i.chrt_id, COALESCE(i.track_number, ''),
	COALESCE(i.price, 0), COALESCE(i.rid, ''),
	COALESCE(i.name, ''), COALESCE(i.sale, 0),
	COALESCE(i.size, ''), COALESCE(i.total_price, 0),
	COALESCE(i.nm_id, 0), COALESCE(i.brand, ''),
	COALESCE(i.status, 0)`

// scanOrderDests returns scan destinations matching orderSelectColumns order.
// 28 columns: 11 order + 7 delivery + 10 payment.
func scanOrderDests(o *models.Order) []any {
	return []any{
		&o.OrderUID, &o.TrackNumber, &o.Entry,
		&o.Locale, &o.InternalSignature,
		&o.CustomerID, &o.DeliveryService,
		&o.ShardKey, &o.SmID,
		&o.DateCreated, &o.OofShard,
		&o.Delivery.Name, &o.Delivery.Phone, &o.Delivery.Zip,
		&o.Delivery.City, &o.Delivery.Address,
		&o.Delivery.Region, &o.Delivery.Email,
		&o.Payment.Transaction, &o.Payment.RequestID,
		&o.Payment.Currency, &o.Payment.Provider,
		&o.Payment.Amount, &o.Payment.PaymentDT,
		&o.Payment.Bank, &o.Payment.DeliveryCost,
		&o.Payment.GoodsTotal, &o.Payment.CustomFee,
	}
}

// scanItemDests returns scan destinations matching itemSelectColumns order.
// 11 columns.
func scanItemDests(item *models.Item) []any {
	return []any{
		&item.ChrtID, &item.TrackNumber,
		&item.Price, &item.RID,
		&item.Name, &item.Sale,
		&item.Size, &item.TotalPrice,
		&item.NmID, &item.Brand,
		&item.Status,
	}
}

// Repo provides methods to work with the PostgreSQL DB
type Repo struct {
	pool *pgxpool.Pool
}

// New creates new PostgreSQL connection pool
func New(ctx context.Context, cfg *config.Config) (*Repo, error) {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.PathEscape(cfg.DBUser), url.PathEscape(cfg.DBPassword),
		cfg.DBHost, cfg.DBPort, cfg.DBName,
	)

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.DBMaxConns)
	poolConfig.MinConns = int32(cfg.DBMinConns)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Repo{pool: pool}, nil
}

// Close closes a connection pool
func (r *Repo) Close() {
	r.pool.Close()
}

// SaveOrder stores an order to a DB using transaction
// If exists it updates the current record (upsert)
func (r *Repo) SaveOrder(ctx context.Context, order *models.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck // no-op after Commit, error is irrelevant

	// Upsert order
	_, err = tx.Exec(ctx, `
		INSERT INTO orders (order_uid, track_number, entry, locale, internal_signature,
			customer_id, delivery_service, shardkey, sm_id, date_created, oof_shard)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (order_uid) DO UPDATE SET
			track_number = EXCLUDED.track_number,
			entry = EXCLUDED.entry,
			locale = EXCLUDED.locale,
			internal_signature = EXCLUDED.internal_signature,
			customer_id = EXCLUDED.customer_id,
			delivery_service = EXCLUDED.delivery_service,
			shardkey = EXCLUDED.shardkey,
			sm_id = EXCLUDED.sm_id,
			date_created = EXCLUDED.date_created,
			oof_shard = EXCLUDED.oof_shard
	`, order.OrderUID, order.TrackNumber, order.Entry, order.Locale,
		order.InternalSignature, order.CustomerID, order.DeliveryService,
		order.ShardKey, order.SmID, order.DateCreated, order.OofShard)
	if err != nil {
		return fmt.Errorf("failed to insert order: %w", err)
	}

	// Upsert delivery
	_, err = tx.Exec(ctx, `
		INSERT INTO deliveries (order_uid, name, phone, zip, city, address, region, email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (order_uid) DO UPDATE SET
			name = EXCLUDED.name,
			phone = EXCLUDED.phone,
			zip = EXCLUDED.zip,
			city = EXCLUDED.city,
			address = EXCLUDED.address,
			region = EXCLUDED.region,
			email = EXCLUDED.email
	`, order.OrderUID, order.Delivery.Name, order.Delivery.Phone,
		order.Delivery.Zip, order.Delivery.City, order.Delivery.Address,
		order.Delivery.Region, order.Delivery.Email)
	if err != nil {
		return fmt.Errorf("failed to insert delivery: %w", err)
	}

	// Upsert payment
	_, err = tx.Exec(ctx, `
		INSERT INTO payments (order_uid, transaction, request_id, currency, provider,
			amount, payment_dt, bank, delivery_cost, goods_total, custom_fee)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (order_uid) DO UPDATE SET
			transaction = EXCLUDED.transaction,
			request_id = EXCLUDED.request_id,
			currency = EXCLUDED.currency,
			provider = EXCLUDED.provider,
			amount = EXCLUDED.amount,
			payment_dt = EXCLUDED.payment_dt,
			bank = EXCLUDED.bank,
			delivery_cost = EXCLUDED.delivery_cost,
			goods_total = EXCLUDED.goods_total,
			custom_fee = EXCLUDED.custom_fee
	`, order.OrderUID, order.Payment.Transaction, order.Payment.RequestID,
		order.Payment.Currency, order.Payment.Provider, order.Payment.Amount,
		order.Payment.PaymentDT, order.Payment.Bank, order.Payment.DeliveryCost,
		order.Payment.GoodsTotal, order.Payment.CustomFee)
	if err != nil {
		return fmt.Errorf("failed to insert payment: %w", err)
	}

	// Replace items (delete old + insert new)
	_, err = tx.Exec(ctx, `DELETE FROM items WHERE order_uid = $1`, order.OrderUID)
	if err != nil {
		return fmt.Errorf("failed to delete old items: %w", err)
	}

	batch := &pgx.Batch{}
	for _, item := range order.Items {
		batch.Queue(`
			INSERT INTO items (order_uid, chrt_id, track_number, price, rid, name,
				sale, size, total_price, nm_id, brand, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, order.OrderUID, item.ChrtID, item.TrackNumber, item.Price,
			item.RID, item.Name, item.Sale, item.Size, item.TotalPrice,
			item.NmID, item.Brand, item.Status)
	}
	results := tx.SendBatch(ctx, batch)

	// Drain results — connection stays busy until all are read.
	for i := 0; i < len(order.Items); i++ {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return fmt.Errorf("failed to insert item %d: %w", i, err)
		}
	}
	// Releases the connection from batch mode
	if err := results.Close(); err != nil {
		return fmt.Errorf("failed to close batch results: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetOrder returns an order entry via orderUID
func (r *Repo) GetOrder(ctx context.Context, orderUID string) (*models.Order, error) {
	order := &models.Order{}

	// Query 1: Order + delivery + payment via JOIN
	err := r.pool.QueryRow(ctx,
		`SELECT `+orderSelectColumns+orderFromJoin+`
		WHERE o.order_uid = $1`,
		orderUID,
	).Scan(scanOrderDests(order)...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Query 2: items
	rows, err := r.pool.Query(ctx,
		`SELECT `+itemSelectColumns+` FROM items i WHERE i.order_uid = $1`,
		orderUID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item models.Item
		if err := rows.Scan(scanItemDests(&item)...); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating items: %w", err)
	}

	return order, nil
}

// GetAllOrders returns all order entries for cache restoring.
// limit = 0: no limit, limit > 0: max number of orders to fetch.
// Orders are returned in date_created DESC order (newest first).
func (r *Repo) GetAllOrders(ctx context.Context, limit int) ([]*models.Order, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck

	// Query 1: all orders + delivery + payment in one JOIN
	query := `SELECT ` + orderSelectColumns + orderFromJoin +
		` ORDER BY o.date_created DESC`

	var rows pgx.Rows
	if limit > 0 {
		query += ` LIMIT $1`
		rows, err = tx.Query(ctx, query, limit)
	} else {
		rows, err = tx.Query(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	defer rows.Close()

	// Collect orders, preserving insertion order via separate slice
	orderMap := make(map[string]*models.Order)
	orderUIDs := make([]string, 0)

	for rows.Next() {
		order := &models.Order{}
		if err := rows.Scan(scanOrderDests(order)...); err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		order.Items = make([]models.Item, 0) // initialize empty, items added below
		orderMap[order.OrderUID] = order
		orderUIDs = append(orderUIDs, order.OrderUID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating orders: %w", err)
	}

	if len(orderUIDs) == 0 {
		return []*models.Order{}, nil
	}

	// Query 2: all items for the fetched orders in a single query
	itemRows, err := tx.Query(ctx,
		`SELECT i.order_uid, `+itemSelectColumns+
			` FROM items i WHERE i.order_uid = ANY($1)`,
		orderUIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}
	defer itemRows.Close()

	for itemRows.Next() {
		var orderUID string
		var item models.Item
		if err := itemRows.Scan(
			&orderUID,
			&item.ChrtID, &item.TrackNumber,
			&item.Price, &item.RID,
			&item.Name, &item.Sale,
			&item.Size, &item.TotalPrice,
			&item.NmID, &item.Brand,
			&item.Status,
		); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		if order, ok := orderMap[orderUID]; ok {
			order.Items = append(order.Items, item)
		}
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating items: %w", err)
	}

	// Build result slice preserving date_created DESC order
	result := make([]*models.Order, 0, len(orderUIDs))
	for _, uid := range orderUIDs {
		result = append(result, orderMap[uid])
	}

	return result, nil
}

// GetOrderUIDs returns a paginated list of order UIDs and the total count.
// sortAsc=true: oldest first; sortAsc=false: newest first.
func (r *Repo) GetOrderUIDs(ctx context.Context, limit, offset int, sortAsc bool) ([]OrderListItem, int64, error) {
	// Get total count
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count orders: %w", err)
	}

	direction := "DESC"
	if sortAsc {
		direction = "ASC"
	}

	// Get paginated UIDs
	query := fmt.Sprintf(
		`SELECT order_uid, date_created FROM orders ORDER BY date_created %s LIMIT $1 OFFSET $2`,
		direction)

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get order UIDs: %w", err)
	}
	defer rows.Close()

	var items []OrderListItem
	for rows.Next() {
		var item OrderListItem
		if err := rows.Scan(&item.OrderUID, &item.DateCreated); err != nil {
			return nil, 0, fmt.Errorf("failed to scan order: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating order UIDs: %w", err)
	}

	return items, total, nil
}

// GetOrderCount returns the number of orders in a DB
func (r *Repo) GetOrderCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count orders: %w", err)
	}
	return count, nil
}

// DeleteOrder removes an order specified by the orderUID and all its related data
func (r *Repo) DeleteOrder(ctx context.Context, orderUID string) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM orders WHERE order_uid = $1`, orderUID)
	if err != nil {
		return fmt.Errorf("failed to delete order: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}
