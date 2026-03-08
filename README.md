# Events

High-throughput event ingestion and analytics backend built with Go and ClickHouse. Designed for ~2K sustained and ~20K peak events/second with sub-millisecond ingestion latency.

## Quick Start (Docker Compose)

The fastest way to get everything running. Requires only Docker.

```bash
docker compose up --build
```

This starts:
- **ClickHouse** on ports 8123 (HTTP) and 9000 (native)
- **Event Service** on port 8080

The service auto-creates the database and table on startup. No manual migration needed.

Once running:
- **API**: http://localhost:8080
- **Swagger UI**: http://localhost:8080/swagger/
- **Health**: http://localhost:8080/health/ready

## Quick Start (Local, without Docker)

Prerequisites: Go 1.23+, a running ClickHouse instance.

```bash
# Build
go build -o event-service ./cmd/server

# Run (assumes ClickHouse on localhost:8123)
./event-service
```

For custom ClickHouse settings, set environment variables before running:

```bash
export CLICKHOUSE_ADDR=localhost:9000
export CLICKHOUSE_PROTOCOL=native
./event-service
```

## API Endpoints

| Method | Path              | Description                          |
|--------|-------------------|--------------------------------------|
| POST   | `/events`         | Ingest a single event                |
| POST   | `/events/bulk`    | Ingest up to 20,000 events at once   |
| GET    | `/metrics`        | Query aggregated event metrics       |
| GET    | `/health/live`    | Liveness probe                       |
| GET    | `/health/ready`   | Readiness probe with pipeline stats  |
| GET    | `/swagger/`       | Swagger UI (interactive API docs)    |

## Example Requests

### Ingest a single event

```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{
    "event_name": "product_view",
    "user_id": "user_123",
    "timestamp": 1723475612,
    "channel": "web",
    "campaign_id": "cmp_987",
    "tags": ["electronics", "homepage", "flash_sale"],
    "metadata": {
      "product_id": "prod-789",
      "price": 129.99,
      "currency": "TRY",
      "referrer": "google"
    }
  }'
```

Response (202 Accepted):
```json
{
  "event_id": "d358eb94c2cc218a6d518e942f44aab1",
  "status": "accepted"
}
```

### Ingest events in bulk

```bash
curl -X POST http://localhost:8080/events/bulk \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {"event_name": "page_view", "user_id": "u1", "timestamp": 1723475612, "channel": "web"},
      {"event_name": "click", "user_id": "u2", "timestamp": 1723475613, "channel": "mobile"},
      {"event_name": "purchase", "user_id": "u1", "timestamp": 1723475614}
    ]
  }'
```

Response (202 Accepted):
```json
{
  "accepted": 3,
  "duplicates": 0,
  "rejected": 0
}
```

### Query metrics with channel breakdown

```bash
curl "http://localhost:8080/metrics?event_name=product_view&from=1723400000&to=1723500000&group_by=channel"
```

Response:
```json
{
  "event_name": "product_view",
  "from": 1723400000,
  "to": 1723500000,
  "total_count": 15432,
  "unique_users": 8721,
  "breakdown": [
    {"key": "web", "total_count": 9200, "unique_users": 5100},
    {"key": "mobile", "total_count": 6232, "unique_users": 3621}
  ]
}
```

### Query metrics with time bucketing

```bash
# Hourly buckets
curl "http://localhost:8080/metrics?event_name=purchase&from=1723400000&to=1723500000&group_by=hourly"

# Daily buckets
curl "http://localhost:8080/metrics?event_name=purchase&from=1723000000&to=1723500000&group_by=daily"
```

### Duplicate detection (idempotency)

Sending the same event twice returns 200 instead of 202:

```bash
# First request → 202 Accepted
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{"event_name":"purchase","user_id":"u1","timestamp":1723475612}'

# Same payload again → 200 OK, status: "duplicate"
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{"event_name":"purchase","user_id":"u1","timestamp":1723475612}'
```

## Running Tests

```bash
go test ./internal/... -v
```

Covers: validation rules, dedup cache correctness and concurrency, pipeline batching/backpressure/graceful shutdown, HTTP handler responses.

## Environment Variables

| Variable                | Default          | Description                                    |
|-------------------------|------------------|------------------------------------------------|
| `SERVER_PORT`           | `8080`           | HTTP server port                               |
| `SERVER_READ_TIMEOUT`   | `5s`             | HTTP read timeout                              |
| `SERVER_WRITE_TIMEOUT`  | `10s`            | HTTP write timeout                             |
| `SERVER_SHUTDOWN_TIMEOUT`| `30s`           | Graceful shutdown deadline                     |
| `CLICKHOUSE_ADDR`       | `localhost:8123`  | ClickHouse address (host:port)                |
| `CLICKHOUSE_DATABASE`   | `events_db`      | Database name (auto-created)                   |
| `CLICKHOUSE_USERNAME`   | `default`        | ClickHouse username                            |
| `CLICKHOUSE_PASSWORD`   | *(empty)*        | ClickHouse password                            |
| `CLICKHOUSE_PROTOCOL`   | `http`           | `http` (port 8123) or `native` (port 9000)    |
| `PIPELINE_BUFFER_SIZE`  | `100000`         | In-memory event queue capacity                 |
| `PIPELINE_WORKERS`      | `4`              | Number of batch-insert worker goroutines       |
| `PIPELINE_BATCH_SIZE`   | `5000`           | Max events per ClickHouse insert               |
| `PIPELINE_FLUSH_INTERVAL`| `500ms`         | Max wait before flushing a partial batch       |
| `DEDUP_CACHE_SIZE`      | `1000000`        | In-memory dedup ring buffer capacity           |

## Project Structure

```
cmd/server/main.go              → Entrypoint, wiring, graceful shutdown
internal/
  config/                       → Environment-based configuration
  model/                        → Domain model (Event)
  dto/                          → HTTP request/response types
  validator/                    → Input validation rules
  dedup/                        → In-memory FIFO dedup cache
  pipeline/                     → Async bounded-buffer ingestion pipeline
  repository/                   → ClickHouse connection, migrations, queries
  service/                      → Business logic (ingestion, metrics)
  handler/                      → HTTP handlers
  middleware/                   → RequestID, logging, recovery, body limit
  server/                       → Router setup (Chi + Swagger)
docs/                           → Auto-generated OpenAPI spec
```

## Architecture

```
                    ┌─────────────────────────────────┐
                    │         HTTP Request             │
                    └──────────────┬──────────────────┘
                                   │
                    ┌──────────────▼──────────────────┐
                    │     Handler (validation only)    │
                    │     No I/O, no DB, < 1ms         │
                    └──────────────┬──────────────────┘
                                   │
                    ┌──────────────▼──────────────────┐
                    │     Dedup Cache (in-memory)      │
                    │     O(1) ring buffer, 1M cap     │
                    └──────────────┬──────────────────┘
                                   │
                    ┌──────────────▼──────────────────┐
                    │   Bounded Channel (100K buffer)  │
                    │   Non-blocking push or 503       │
                    └──────────────┬──────────────────┘
                                   │
                    ┌──────────────▼──────────────────┐
                    │   Worker Pool (4 goroutines)     │
                    │   Batch up to 5000 or 500ms      │
                    └──────────────┬──────────────────┘
                                   │
                    ┌──────────────▼──────────────────┐
                    │   ClickHouse (PrepareBatch)      │
                    │   ReplacingMergeTree, monthly    │
                    └─────────────────────────────────┘
```

## Design Decisions

### Why async pipeline instead of direct inserts?
ClickHouse performs best with fewer, larger inserts. Direct per-request inserts at 20K req/s would create 20K tiny parts/second, degrading performance. The pipeline batches up to 5000 events per insert, reducing insert operations by ~1000x while keeping HTTP latency under 1ms.

### Why ReplacingMergeTree?
It handles deduplication at the storage level during background merges. Combined with the in-memory dedup cache (which catches immediate retries), this provides two-layer idempotency without requiring a separate dedup store like Redis.

### Why deterministic event IDs?
`event_id = SHA256(event_name + user_id + timestamp)`. The same logical event always produces the same ID regardless of how many times it's sent. This is what makes idempotency work — no client-generated IDs needed.

### Why FIFO ring buffer instead of LRU cache?
The dedup cache uses a fixed-size ring buffer with a map overlay. Unlike LRU, there are zero heap allocations per operation — just a map lookup, an index increment, and at most one eviction. At 20K ops/sec this matters.

### Why `FINAL` in metrics queries?
ReplacingMergeTree may have un-merged duplicates between compaction cycles. The `FINAL` modifier ensures correct counts at query time. The trade-off is slightly slower reads, which is acceptable since the metrics endpoint doesn't need real-time latency.

## Trade-offs

| Decision | Benefit | Cost |
|----------|---------|------|
| Async pipeline | Sub-ms ingestion latency | Events not immediately queryable (~500ms delay) |
| In-memory dedup | Zero-allocation O(1) dedup | Per-instance only; doesn't dedup across multiple service instances |
| ReplacingMergeTree | Storage-level dedup without external deps | Temporary duplicates between merges; `FINAL` adds query overhead |
| Metadata as JSON string | Schema-flexible, no migration for new fields | Not directly queryable as columns in ClickHouse |
| Single binary | Simple deployment | Horizontal scaling requires shared-nothing awareness |

## Scaling Notes

- **Horizontal scaling**: Run multiple instances behind a load balancer. Each has its own dedup cache (first-pass filter); ClickHouse ReplacingMergeTree handles cross-instance dedup during merges.
- **Pipeline tuning**: All buffer/worker/batch parameters are configurable via env vars for different hardware profiles.
- **ClickHouse cluster**: Point `CLICKHOUSE_ADDR` at a distributed table or load balancer for clustered setups.
- **Monitoring**: `/health/ready` exposes live pipeline stats (submitted, processed, dropped, queue length). Structured JSON logs via `slog` are ready for any log aggregator.

## Future Improvements

- Kafka/NATS as durable ingestion buffer for at-least-once delivery guarantees
- Materialized views in ClickHouse for pre-aggregated metrics (faster reads at scale)
- Sharded dedup cache for reduced mutex contention at extreme throughput
- API authentication (API key or JWT) and per-tenant authorization
- Rate limiting middleware per API key / tenant
- Prometheus metrics endpoint for production monitoring
- Integration tests with testcontainers (ephemeral ClickHouse)
