# 15 — Redis Infrastructure

## Overview

The `internal/redis` package is the single Redis infrastructure layer for the entire backend. It owns the client connection, lifecycle management, health checks, and generic key-value operations. No other package creates or manages a Redis connection — they receive a `*Redis` through constructor injection and interact with it via the public API or the raw `*goredis.Client` for advanced use cases.

---

## Package Structure

```
internal/redis/
├── redis.go      — Redis struct, New(), Client(), Close(), generic operations, retry logic
├── health.go     — Health(): connectivity probe
└── options.go    — Option functional type and WithClient() for test injection
```

---

## Design Principles

| Principle | How it is applied |
|---|---|
| **Single owner** | Only one `*Redis` exists in the application, created in `bootstrap.go` |
| **Constructor injection** | Every consumer receives `*redis.Redis` as an argument — no globals |
| **Private client** | `client *goredis.Client` is unexported; callers use `Client()` only when needed |
| **Thread safety** | go-redis clients are safe for concurrent use; `Close()` uses `sync.Once` |
| **Fail fast** | Startup retries up to 5 times with exponential backoff; if all fail, the process refuses to start |
| **Context propagation** | Every method accepts `context.Context` for cancellation and deadlines |
| **Thin wrappers** | Generic operation methods delegate directly to the client — no business logic |

---

## Public API

```go
// Construct
func New(ctx context.Context, cfg *config.Config, log logger.Logger, opts ...Option) (*Redis, error)

// Raw client access for advanced use cases
func (r *Redis) Client() *goredis.Client

// Liveness probe
func (r *Redis) Health(ctx context.Context) error

// Lifecycle
func (r *Redis) Close() error

// Generic operations
func (r *Redis) Get(ctx context.Context, key string) (string, error)
func (r *Redis) Set(ctx context.Context, key string, value any, ttl time.Duration) error
func (r *Redis) Delete(ctx context.Context, keys ...string) (int64, error)
func (r *Redis) Exists(ctx context.Context, keys ...string) (int64, error)
func (r *Redis) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)
func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error)
func (r *Redis) Increment(ctx context.Context, key string) (int64, error)
func (r *Redis) Decrement(ctx context.Context, key string) (int64, error)
func (r *Redis) FlushDB(ctx context.Context) error
```

---

## Connection Configuration

All configuration comes from `config.RedisConfig`. Nothing is hardcoded.

| Config field | go-redis option | Default (dev) | Default (prod) |
|---|---|---|---|
| `Host` + `Port` | `Addr` | `localhost:6379` | set at deploy time |
| `Password` | `Password` | _(empty)_ | set at deploy time |
| `DB` | `DB` | `0` | `0` |
| `PoolSize` | `PoolSize` | `10` | `20` |
| `MinIdleConns` | `MinIdleConns` | `2` | `5` |
| `DialTimeout` | `DialTimeout` | `5s` | `5s` |
| `ReadTimeout` | `ReadTimeout` | `3s` | `3s` |
| `WriteTimeout` | `WriteTimeout` | `3s` | `3s` |
| `PoolTimeout` | `PoolTimeout` | `4s` | `4s` |
| `MaxRetries` | `MaxRetries` | `3` | `3` |

`buildOptions` in `redis.go` is the single place that translates config values into `*goredis.Options`. Changing client behaviour (e.g. adding TLS or a custom dialer) happens only there.

### Environment variables

```env
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_POOL_SIZE=10
REDIS_MIN_IDLE_CONNS=2
REDIS_DIAL_TIMEOUT=5s
REDIS_READ_TIMEOUT=3s
REDIS_WRITE_TIMEOUT=3s
REDIS_POOL_TIMEOUT=4s
REDIS_MAX_RETRIES=3
```

---

## Startup and Retry Logic

`New()` calls `connectWithRetry()` which makes up to **5 attempts** with exponential backoff before giving up.

```
attempt 1  → immediate
attempt 2  → wait 500ms
attempt 3  → wait 1s
attempt 4  → wait 2s
attempt 5  → wait 4s  (capped at 15s)
```

Each attempt pings Redis within a **5s timeout**. On failure, a warning is logged with the attempt number and next retry delay.

Between attempts the retry loop listens on `ctx.Done()`. If the context is cancelled, the loop exits immediately:

```go
select {
case <-ctx.Done():
    return fmt.Errorf("failed to connect to redis: context cancelled: %w", ctx.Err())
case <-time.After(delay):
}
```

If all 5 attempts fail, `New()` returns:
```
failed to connect to redis after 5 attempts: <root cause>
```

> `MaxRetries` in config maps to go-redis's command-level retry count (for transient errors during normal operation). Startup retry attempts are always fixed at 5.

---

## Health Check

`Health(ctx)` issues a `PING` command and is intended for liveness probes. It is distinct from the startup ping — no retry is performed.

```go
if err := r.Health(ctx); err != nil {
    // redis unreachable — return 503
}
```

Returns `nil` on success, a wrapped error on failure:
```
failed to ping redis: <root cause>
```

---

## Generic Operations

These methods are intentionally thin. They wrap the go-redis command, wrap any error with context, and return. No caching logic, TTL strategies, or serialisation is implemented here.

| Method | Redis command | Notes |
|---|---|---|
| `Get(ctx, key)` | `GET` | Returns `goredis.Nil` if key absent — callers must check |
| `Set(ctx, key, value, ttl)` | `SET EX` | `ttl = 0` means no expiry |
| `Delete(ctx, keys...)` | `DEL` | Returns count of actually deleted keys |
| `Exists(ctx, keys...)` | `EXISTS` | Returns count of matched keys (can exceed 1 if a key is listed twice) |
| `Expire(ctx, key, ttl)` | `EXPIRE` | Returns `false` if key does not exist |
| `TTL(ctx, key)` | `TTL` | `-1` = no expiry; `-2` = key not found |
| `Increment(ctx, key)` | `INCR` | Creates key at 0 then increments if absent |
| `Decrement(ctx, key)` | `DECR` | Creates key at 0 then decrements if absent |
| `FlushDB(ctx)` | `FLUSHDB` | Deletes all keys in the selected DB — use with caution |

All errors are wrapped with context before being returned:

```
failed to execute GET "session:abc123": <root cause>
failed to execute SET "rate:user:42": <root cause>
```

---

## Raw Client Access

For operations not covered by the generic wrappers, call `Client()` to retrieve the underlying `*goredis.Client`:

```go
// Pipeline
pipe := r.Client().Pipeline()
pipe.Set(ctx, "k1", "v1", 0)
pipe.Set(ctx, "k2", "v2", 0)
_, err := pipe.Exec(ctx)

// Transaction (MULTI/EXEC)
_, err := r.Client().TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
    pipe.Set(ctx, "k", "v", 0)
    return nil
})

// Pub/Sub
sub := r.Client().Subscribe(ctx, "channel")
defer sub.Close()
```

---

## Graceful Shutdown

`Close()` is guarded by `sync.Once` — it is safe to call any number of times from any goroutine.

```go
// First call: logs "closing redis client", closes the connection, logs "redis client closed".
r.Close()

// Subsequent calls: no-op, returns nil.
r.Close()
r.Close()
```

In the app lifecycle (`lifecycle.go`), `Close()` is called during `Shutdown()` after the HTTP server is stopped but before the database pool is closed, following reverse initialization order.

---

## How Every Future Package Will Use This

Services and repositories that need Redis receive `*redis.Redis` through constructor injection. The struct itself is never imported from business logic — callers use only the generic operation methods or `Client()` for advanced commands.

### Pattern: rate limiting counter

```go
type RateLimiter struct {
    redis *redis.Redis
    log   logger.Logger
}

func NewRateLimiter(r *redis.Redis, log logger.Logger) *RateLimiter {
    return &RateLimiter{
        redis: r,
        log:   log.With(logger.String("component", "rate_limiter")),
    }
}

func (rl *RateLimiter) Allow(ctx context.Context, userID string) (bool, error) {
    key := fmt.Sprintf("rate:%s", userID)
    count, err := rl.redis.Increment(ctx, key)
    if err != nil {
        return false, err
    }
    if count == 1 {
        rl.redis.Expire(ctx, key, time.Minute)
    }
    return count <= 100, nil
}
```

### Pattern: session cache

```go
type SessionCache struct {
    redis *redis.Redis
}

func (c *SessionCache) Store(ctx context.Context, token string, data []byte, ttl time.Duration) error {
    return c.redis.Set(ctx, "session:"+token, data, ttl)
}

func (c *SessionCache) Load(ctx context.Context, token string) ([]byte, error) {
    val, err := c.redis.Get(ctx, "session:"+token)
    if errors.Is(err, goredis.Nil) {
        return nil, ErrSessionNotFound
    }
    return []byte(val), err
}
```

### Pattern: advanced use via Client()

```go
// Distributed lock using SET NX (future: use a proper redlock library)
ok, err := r.Client().SetNX(ctx, "lock:resource", "owner", 10*time.Second).Result()
```

---

## Integration Map

```
config.Config
      │
      ▼
redis.New(ctx, cfg, log)             ← constructed once in bootstrap.go
      │
      ├──▶ App.redis                 ← lifecycle owner (Close on shutdown)
      │
      ├──▶ r.Health()                ← called by health handler / monitoring
      │
      ├──▶ r.Get / Set / Delete ...  ← generic operations
      │         │
      │         ├──▶ RateLimiter
      │         ├──▶ SessionCache
      │         ├──▶ PresenceTracker
      │         └──▶ (future services)
      │
      └──▶ r.Client()               ← pipelines, pub/sub, streams, locks
                │
                ├──▶ PubSub broker
                ├──▶ Stream consumer/producer
                └──▶ Distributed lock manager
```

---

## Package Dependency Rule

```
✅  redis  ←  app
✅  redis  ←  services/*
✅  redis  ←  repositories/*
✅  redis  ←  handlers (health check only)

❌  redis  →  services/*
❌  redis  →  handlers
❌  redis  →  repositories/*
❌  goredis.Client  ←  any package except redis (use Client() accessor or wrappers)
```

The redis package depends only on `config`, `logger`, `go-redis/v9`, and the standard library.

---

## Testing

### Unit tests (no Redis required)

```bash
go test -short -v ./internal/redis/...
```

Inject a pre-configured client via `WithClient`:

```go
import "github.com/alicebob/miniredis/v2"

func TestRedis_SetGet(t *testing.T) {
    mr := miniredis.RunT(t)

    client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
    r := &redis.Redis{}  // use WithClient option via New or a test constructor

    // use r.Set / r.Get against the in-process miniredis server
}
```

### Integration tests (requires running Redis)

```bash
make up                   # start postgres + redis via docker compose
make test-integration     # run integration tests
```

Environment variables used by test helpers:

| Variable | Default | Purpose |
|---|---|---|
| `TEST_REDIS_ADDR` | `localhost:6379` | Redis address |
| `TEST_REDIS_DB` | `1` | Isolated database index for tests |

Covers:
- Successful connection (`New`)
- `Client()` returns a non-nil client
- `Close()` idempotency (three consecutive calls)
- `Health()` returns nil on live connection
- `Health()` returns descriptive error on cancelled context
- `Get` returns `goredis.Nil` for absent key
- `Set` / `Get` round-trip
- `Delete` returns correct count
- `Expire` / `TTL` interaction
- `Increment` / `Decrement` atomicity

---

## Future Extensions

The following can be added without changing the public API or any consumer:

| Extension | How |
|---|---|
| Pub/Sub broker | Add `Subscribe(ctx, channels...)` and `Publish(ctx, channel, msg)` methods using `r.client.Subscribe` / `r.client.Publish` |
| Redis Streams | Add `XAdd`, `XRead`, `XAck` wrappers or expose a dedicated `Streams()` accessor |
| Distributed locks (Redlock) | Add a `Lock(ctx, key, ttl)` method using `SET NX PX`; consider a proper redlock library for multi-node safety |
| Lua scripts | Cache compiled `goredis.Script` objects as package-level vars; expose `RunScript(ctx, script, keys, args)` |
| Cluster mode | Replace `goredis.NewClient` with `goredis.NewClusterClient` in `buildOptions`; no consumer changes required |
| Sentinel mode | Replace with `goredis.NewFailoverClient`; no consumer changes required |
| Connection jitter | Add `±10%` random jitter to retry delay in `connectWithRetry` to avoid thundering herd on mass restart |
| Metrics | Register a hook via `r.client.AddHook` to emit command latency and error counts to Prometheus |
