-- Down migration: drops all tables and indexes

-- Drop indexes first
DROP INDEX IF EXISTS idx_items_order_uid;
DROP INDEX IF EXISTS idx_orders_date_created;
DROP INDEX IF EXISTS idx_orders_customer_id;

-- Drop tables in dependency order (children first)
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS deliveries;
DROP TABLE IF EXISTS orders;
