# 14 — Database Infrastructure

## Overview

The `internal/database` package is the single PostgreSQL infrastructure layer for the entire backend. It owns the connection pool, lifecycle management, health checks, and transaction helpers. No other package creates or manages a database connection — they receive a `*Database` through constructor injection and interact with it via the public API or the raw `*pgxpool.Pool` for repository use.

---

## Package Structure

```
internal/database/
├── postgres.go      — Database struct, New(), Pool(), Close(), retry logic
├── pool.go          — buildPoolConfig(): pool sizing from config
├── health.go        — Health(): connectivity probe
└── transaction.go   — BeginTx(): transaction helper
```

---

## Design Principles

| Principle | How it is applied |
|---|---|
| **Single owner** | Only one `*Database` exists in the application, created in `bootstrap.go` |
| **Constructor injection** | Every consumer receives `*database.Database` as an argument — no globals |
| **Private pool** | `pool *pgxpool.Pool` is unexported; callers use `Pool()` only when needed |
| **Thread safety** | `pgxpool.Pool` is safe for concurrent use; `Close()` uses `sync.Once` |
| **Fail fast** | Startup retries up to 5 times; if all fail, the process refuses to start |
| **Context propagation** | Every method accepts `context.Context` for cancellation and deadlines |

---

## Public API

```go
// Construct
func New(ctx context.Context, cfg config.DatabaseConfig, log logger.Logger) (*Database, error)

// Pool access for repositories
func (db *Database) Pool() *pgxpool.Pool

// Liveness probe
func (db *Database) Health(ctx context.Context) error

// Transaction helper
func (db *Database) BeginTx(ctx context.Context) (pgx.Tx, error)

// Lifecycle
func (db *Database) Close()
```

---

## Connection Pool Configuration

Pool sizing comes entirely from `config.DatabaseConfig`. Nothing is hardcoded.

| Config field | pgxpool field | Default |
|---|---|---|
| `MaxOpenConns` | `MaxConns` | 25 |
| `MaxIdleConns` | `MinConns` | 10 |
| `ConnMaxLifetime` | `MaxConnLifetime` | 30m |

`buildPoolConfig` in `pool.go` is the single place that translates config values into a `*pgxpool.Config`. Changing pool behaviour (e.g. adding `MaxConnIdleTime`) happens only there.

---

## Startup and Retry Logic

`New()` calls `connectWithRetry()` which makes up to **5 attempts** with exponential backoff before giving up.

```
attempt 1  → immediate
attempt 2  → wait 1s
attempt 3  → wait 2s
attempt 4  → wait 4s
attempt 5  → wait 8s  (capped at 30s)
```

Each attempt:
1. Creates the pool object (`pgxpool.NewWithConfig`) — no network call yet.
2. Pings the database within a **5s timeout**.
3. On failure, closes the pool and logs a warning with the attempt number and next retry delay.

Between attempts the retry loop listens on `ctx.Done()`. If the context is cancelled (e.g. shutdown signal during startup), the loop exits immediately and returns a wrapped `context.Canceled` error rather than waiting for the next retry slot.

```go
select {
case <-ctx.Done():
    return nil, fmt.Errorf("postgres connection cancelled: %w", ctx.Err())
case <-time.After(delay):
}
```

If all 5 attempts fail, `New()` returns:
```
postgres: all 5 connection attempts failed: failed to ping postgres: <root cause>
```

---

## Health Check

`Health(ctx)` pings the database within a **3s timeout** (shorter than the startup ping timeout because it runs on the hot path of liveness probes).

```go
if err := db.Health(ctx); err != nil {
    // database unreachable — return 503
}
```

Returns `nil` on success, a wrapped error on failure:
```
postgres health check failed: <root cause>
```

---

## Transactions

`BeginTx(ctx)` starts a new transaction and returns a `pgx.Tx`. The caller owns the transaction lifecycle.

```go
tx, err := db.BeginTx(ctx)
if err != nil { ... }
defer tx.Rollback(ctx) // safe no-op after Commit

// execute queries using tx

if err := tx.Commit(ctx); err != nil { ... }
```

`defer tx.Rollback(ctx)` is the standard guard pattern — if the function returns before reaching `Commit`, the transaction is automatically rolled back. After `Commit` succeeds, `Rollback` returns `ErrTxClosed` which is ignored.

---

## Graceful Shutdown

`Close()` is guarded by `sync.Once` — it is safe to call any number of times from any goroutine.

```go
// First call: logs "closing postgres pool", drains the pool, logs "postgres pool closed".
db.Close()

// Subsequent calls: no-op.
db.Close()
db.Close()
```

In the app lifecycle (`lifecycle.go`), `Close()` is called during `Shutdown()` after the HTTP server and Redis client are already stopped, following reverse initialization order.

---

## How Every Future Package Will Use This

Repositories receive `*database.Database` (or just `db.Pool()`) through constructor injection. The `Database` struct itself is never imported from a service or handler — only from repositories.

### Pattern: inject Pool into a repository

```go
type UserRepository struct {
    pool *pgxpool.Pool
    log  logger.Logger
}

func NewUserRepository(db *database.Database, log logger.Logger) *UserRepository {
    return &UserRepository{
        pool: db.Pool(),
        log:  log.With(logger.String("component", "user_repository")),
    }
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    row := r.pool.QueryRow(ctx, "SELECT id, email FROM users WHERE id = $1", id)
    // ...
}
```

### Pattern: use BeginTx for multi-step operations

```go
func (s *PartyService) CreateParty(ctx context.Context, req CreatePartyRequest) error {
    tx, err := s.db.BeginTx(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    // insert party
    // insert host membership
    // insert initial settings

    return tx.Commit(ctx)
}
```

---

## Integration Map

```
config.DatabaseConfig
        │
        ▼
database.New(ctx, cfg, log)         ← constructed once in bootstrap.go
        │
        ├──▶ App.db                  ← lifecycle owner (Close on shutdown)
        │
        ├──▶ db.Health()             ← called by health handler / monitoring
        │
        └──▶ db.Pool()
                │
                ├──▶ UserRepository
                ├──▶ PartyRepository
                ├──▶ ChatRepository
                ├──▶ SessionRepository
                └──▶ (future repositories)
```

---

## Package Dependency Rule

```
✅  database  ←  app
✅  database  ←  repositories/*
✅  database  ←  handlers (health check only)

❌  database  →  services/*
❌  database  →  handlers
❌  database  →  repositories/*
❌  pgxpool   ←  any package except database (use Pool() accessor)
```

The database package depends only on `config`, `logger`, `pgx/v5`, and the standard library.

---

## Testing

### Unit tests (no database required)

```bash
go test -short -v ./internal/database/...
```

Covers:
- `buildPoolConfig` field mapping (MaxConns, MinConns, ConnMaxLifetime)
- DSN format validation
- `connectWithRetry` fast exit on pre-cancelled context

### Integration tests (requires running PostgreSQL)

```bash
make up                   # start postgres + redis via docker compose
make test-integration     # run integration tests
```

Environment variables used by the test helpers:

| Variable | Default | Purpose |
|---|---|---|
| `TEST_POSTGRES_DSN` | — | Must be non-empty to enable integration tests |
| `TEST_POSTGRES_HOST` | `localhost` | Database host |
| `TEST_POSTGRES_USER` | `streamtogether` | Database user |
| `TEST_POSTGRES_PASSWORD` | `streamtogether` | Database password |
| `TEST_POSTGRES_DB` | `streamtogether` | Database name |

Covers:
- Successful connection (`New`)
- `Pool()` returns a non-nil pool
- `Close()` idempotency (three consecutive calls)
- Pool unusable after `Close()`
- `Health()` returns nil on live connection
- `Health()` returns descriptive error on cancelled context and closed pool
- `BeginTx()` commit, rollback, double-rollback, cancelled context

---

## Future Extensions

The following can be added without changing the public API or any consumer:

| Extension | How |
|---|---|
| Read replica pool | Add a `readPool *pgxpool.Pool` field; expose `ReadPool()` for read-heavy repositories |
| Query tracing | Inject a `pgx.QueryTracer` into `pgxpool.Config.ConnConfig.Tracer` inside `buildPoolConfig` |
| Statement timeout | Set `default_transaction_isolation` or `statement_timeout` in `ConnConfig.RuntimeParams` |
| Connection retry jitter | Add `±10%` random jitter to the backoff delay in `connectWithRetry` to avoid thundering herd |
| Metrics | Register pool stat collection (`db.pool.Stat()`) on a ticker and emit to Prometheus |
