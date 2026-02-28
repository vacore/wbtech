What is it?
==========

A microservice that receives orders data from Kafka, saves it to a PostgreSQL DB instance,
and serves requests via HTTP API with a web interface, caching reads in a LRU+TTL cache.

Architecture:
============
```
order-service:
┌──────────────────────────────────────────────────────────┐
│ ┌─────────────┐    ┌──────────────┐    ┌────────────┐    │
│ │ Kafka       │───>│ Consumer     │───>│ Repository │────┼──> PostgreSQL
│ │ (orders)    │    │              │    │ (repo)     │    │
│ └─────────────┘    │ - validate   │    └────────────┘    │
│                    │ - save       │           │          │
│ ┌─────────────┐    │ - cache      │     ┌─────v──────┐   │
│ │ Kafka       │<───│ - DLQ        │     │ Cache      │   │
│ │ (DLQ)       │    └──────────────┘     │ (LRU+TTL)  │   │
│ └─────────────┘                         └────────────┘   │
│ ┌─────────────┐    ┌──────────────┐           ^          │
│ │ Browser     │───>│ HTTP Handler │───────────┘          │
│ │ (Web UI)    │<───│ (chi)        │                      │
│ └─────────────┘    └──────────────┘                      │
│ ┌─────────────┐    ┌──────────────┐                      │
│ │ Prometheus  │<───│ /metrics     │                      │
│ └─────────────┘    └──────────────┘                      │
└──────────────────────────────────────────────────────────┘

test-producer:
┌─────────────────────────────────────────────────────────────────────────────┐
│ ┌──────────┐    ┌───────────────────────────────────┐                       │
│ │ Kafka    │<───│ CRUD Simulator                    │                       │
│ │ (orders) │    │ - CREATE: generate + send Kafka   │                       │
│ └──────────┘    │ - UPDATE: generate + send Kafka   │ HTTP ┌──────────────┐ │
│                 │ - READ:   HTTP GET /order/{id}    │─────>│ order-service│ │
│                 │ - DELETE: HTTP DELETE /order/{id} │─────>│ (orders)     │ │
│                 │ - INVALID: corrupted messages     │      └──────────────┘ │
│                 └───────────────────────────────────┘                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

Operations:
==========
CREATE/UPDATE (via Kafka incoming message):
 - Take Kafka message with data of a new/existing order
 - Parse order: unmarshal JSON, validate fields:
    - OK            : save to DB, and Cache
    - PermanentError: write to the DLQ
 - If !noPermanentErrors -> Commit Kafka offset, else -> retry

READ (via HTTP API):
 - Fire HTTP GET /order/{id} handler
 - Check cache:
    - if Miss -> query from DB, save to cache
    - marshal JSON -> response

DELETE (via HTTP API):
 - Fire HTTP DELETE /order/{id}
 - Check cache:
    - if Hit -> remove from cache
    - response OK

Cache:
=====
At any given time the cache stores at most <CacheMaxItems> items.

Strategy: LRU + TTL.
- LRU (Least Recently Used): each access (except delete) makes the item in cache the most recent.
  If it's a CREATE, and the cache is full -> evict the least recent element.
- TTL (Time To Live): all items live for a specified time.
  A background process is running to evict expired ones.

On startup:
    The service queries <CacheMaxItems> from the DB sorted in descending order by date_created,
    and stores them into cache.

Instructions:
============

Setup:
-----

Init the environment:
`$ docker compose up -d kafka postgres zookeeper`  # run in detached mode

Check it's working:
`$ docker compose ps`

See respective logs (if needed):
`$ docker compose logs postgres`  # add -f to logs to follow

Run order-service:
`$ docker compose up order-service`  # add --build if there are code changes

To speed-up cache filling (for demo):
`$ CACHE_MAX_ITEMS=10 docker compose up order-service`

Run CRUD simulator:
`$ docker compose up test-producer`

To speed-up order submission (for demo):
`$ SIM_WORKERS=32 SIM_MIN_DELAY=10ms SIM_MAX_DELAY=100ms SIM_SEED_COUNT=50 docker compose up test-producer`

Shut down the environment:
`$ docker compose down --remove-orphans`
`$ docker volume prune -af`

Override log level:
`$ LOG_LEVEL=warn docker compose up order-service`


Working check:
-------------

Web-interface: http://localhost:8081
Metrics:       http://localhost:8081/metrics

Get specific order:
`$ curl -s http://localhost:8081/order/a8715b15e9826314test | jq '.'`

Get all orders:
`$ curl -s http://localhost:8081/orders | jq '.'`

Cache stats:
`$ curl -s http://localhost:8081/stats | jq '.'`

Health check:
`$ curl -s http://localhost:8081/health | jq '.'`


Testing:
-------

Run unit tests:
`$ go test ./... | grep -v '\[no test files\]'`

Run benchmarks:
`$ go test ./... -bench`

Get coverage report:
`$ go test ./... -coverprofile=coverage.out`
`$ go tool cover -html=coverage.out -o coverage.html`

Integration tests with PostgreSQL:
`$ go test ./internal/repo -tags=integration -v -count=1`


Manual build and run:
--------------------

Build only service:
`$ go build -o build/service cmd/service/main.go`

Build only test_producer:
`$ go build -o build/test_producer cmd/test_producer/main.go`

Run service/test_producer:
`$ go run cmd/service/main.go`
`$ go run cmd/service/test_producer.go`

Run the linter to check for issues:
`$ golangci-lint run`


PostgreSQL:
----------

Apply (up):
`$ docker exec -i orders_postgres psql -U orders_user -d orders_db < sql/up.sql`

Rollback (down):
`$ docker exec -i orders_postgres psql -U orders_user -d orders_db < sql/down.sql`

Enter console:
`$ docker exec -it orders_postgres psql -U orders_user -d orders_db`

List tables:
`orders_db=# \dt`
`orders_db=# \d orders`
`orders_db=# \d deliveries`
`orders_db=# \d payments`
`orders_db=# \d items`

Count orders:
`orders_db=# select count(*) from orders;`

List all orders:
`orders_db=# select order_uid, customer_id, delivery_service, date_created
             from orders order by date_created desc;`

View specific order:
`orders_db=# select o.order_uid, o.customer_id, o.track_number,
                    d.name, d.city, d.phone,
                    p.amount, p.currency, p.bank
             from orders o
             join deliveries d on o.order_uid = d.order_uid
             join payments p on o.order_uid = p.order_uid
             where o.order_uid = '29ffe374f2fbe60dtest';`

View items for an order:
`orders_db=# select name, brand, price, sale, total_price, size, status
             from items where order_uid = '29ffe374f2fbe60dtest';`

Delete all orders:
`orders_db=# delete from orders;`
