# 13 — Logger Infrastructure

## Overview

The `internal/logger` package is the single logging infrastructure for the entire backend. It wraps [zerolog](https://github.com/rs/zerolog) behind a stable Go interface, meaning **no other package in the application ever imports zerolog directly**. If the underlying library needs to change in the future, only this package changes.

---

## Package Structure

```
internal/logger/
├── logger.go     — Logger interface + zerolog-backed implementation
├── options.go    — Functional Option pattern for configuration
├── fields.go     — Strongly typed Field constructors
└── context.go    — context.Context helpers (WithContext / FromContext)
```

---

## Design Principles

| Principle | How it is applied |
|---|---|
| **Dependency inversion** | All consumers depend on `logger.Logger` (interface), never on `*zerolog.Logger` |
| **No global state** | `New()` returns a value; no package-level `var log` |
| **Constructor injection** | Every component that needs logging receives a `logger.Logger` argument |
| **Thread safety** | zerolog is lock-free by design; the wrapper adds no shared mutable state |
| **Testability** | `WithWriter(w io.Writer)` allows any `io.Writer` (e.g. `*bytes.Buffer`) to be injected in tests; `Nop()` discards all output |
| **Extensibility** | New field types, output targets, and tracing integrations can be added without changing the interface |

---

## The Logger Interface

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    Fatal(msg string, fields ...Field)
    With(fields ...Field) Logger
}
```

`Fatal` logs the message then calls `os.Exit(1)`. Use it only in the bootstrap layer for unrecoverable startup errors.

`With` returns a **child logger** that permanently carries the given fields. The parent logger is not modified. This is the mechanism for contextual logging (e.g. attaching a `request_id` for the lifetime of an HTTP request).

---

## Configuration Options

`New()` accepts zero or more `Option` values. Defaults are sensible for production.

| Option | Default | Description |
|---|---|---|
| `WithLevel(string)` | `"info"` | Minimum level to emit. Accepted: `debug`, `info`, `warn`, `error`, `fatal` |
| `WithPretty(bool)` | `false` | Human-readable coloured console output. Enable in development only |
| `WithWriter(io.Writer)` | `os.Stdout` | Output destination. Override in tests with `*bytes.Buffer` |
| `WithCaller()` | off | Attaches source file and line number to every entry |
| `WithTimeFormat(string)` | `time.RFC3339` | Timestamp format string |

### How the App bootstrap uses it

```go
log := logger.New(
    logger.WithLevel(cfg.Logging.Level),
    logger.WithPretty(cfg.App.Environment == config.EnvDevelopment),
)
```

- In `development`: coloured, human-readable output to stdout.
- In `test` / `production`: structured JSON to stdout, consumed by log aggregators (Loki, CloudWatch, Datadog, etc.).

---

## Structured Fields

Never use untyped `map[string]interface{}` or raw variadic `any` pairs. Every log field is created through a typed constructor:

| Constructor | Go type | JSON output |
|---|---|---|
| `String(key, val)` | `string` | `"key":"val"` |
| `Int(key, val)` | `int` | `"key":42` |
| `Int64(key, val)` | `int64` | `"key":42` |
| `Bool(key, val)` | `bool` | `"key":true` |
| `Duration(key, val)` | `time.Duration` | `"key":1500` (ms) |
| `Time(key, val)` | `time.Time` | `"key":"2026-07-28T..."` |
| `Any(key, val)` | `interface{}` | serialized via `%v` |
| `Err(err)` | `error` | `"error":"message"` |

### Example

```go
log.Info("database connected",
    logger.String("host", "localhost"),
    logger.Int("port", 5432),
    logger.Duration("ping_latency", 4*time.Millisecond),
)
```

JSON output:

```json
{
  "level": "info",
  "time": "2026-07-28T10:00:00Z",
  "message": "database connected",
  "host": "localhost",
  "port": 5432,
  "ping_latency": 4
}
```

---

## Child Loggers and `With`

`With` creates a child logger that attaches fields to **every** subsequent entry without repeating them at every call site. Fields are applied to zerolog's internal context chain, so they are encoded once at construction time — not on every log call.

```go
// At the start of an HTTP request handler:
requestLog := log.With(
    logger.String("request_id", requestID),
    logger.String("method", r.Method),
    logger.String("path", r.URL.Path),
)

requestLog.Info("request received")
requestLog.Warn("rate limit approaching", logger.Int("remaining", 5))
requestLog.Error("handler failed", logger.Err(err))
// Every entry above automatically includes request_id, method, and path.
```

---

## Context Helpers

The `context.go` file provides two functions for passing a logger through `context.Context` — the standard Go mechanism for request-scoped values.

```go
// Store a child logger in a context (e.g. in HTTP middleware):
ctx = logger.WithContext(ctx, requestLog)

// Retrieve it anywhere downstream that has access to ctx:
log := logger.FromContext(ctx)
log.Info("processing item")
```

`FromContext` **never returns nil**. If no logger has been stored, it returns `Nop()` — a logger that discards all output — so downstream code never needs a nil guard.

---

## How Every Future Package Will Use This

The logger is passed as a constructor argument. No package reaches for a global variable.

### Pattern: constructor injection

```go
// Example — future PostgreSQL repository
type UserRepository struct {
    db  *pgxpool.Pool
    log logger.Logger
}

func NewUserRepository(db *pgxpool.Pool, log logger.Logger) *UserRepository {
    return &UserRepository{db: db, log: log}
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    r.log.Debug("querying user", logger.String("id", id))
    // ...
}
```

### Pattern: child logger per component

Each component creates a child logger that permanently tags its entries with the component name:

```go
func NewUserRepository(db *pgxpool.Pool, log logger.Logger) *UserRepository {
    return &UserRepository{
        db:  db,
        log: log.With(logger.String("component", "user_repository")),
    }
}
```

This produces entries like:

```json
{"level":"debug","component":"user_repository","id":"abc123","message":"querying user"}
```

---

## Integration Map

The diagram below shows how the logger flows through the application.

```
config.Load()
      │
      ▼
logger.New(WithLevel, WithPretty)          ← constructed once in bootstrap.go
      │
      ├──▶ App.log                         ← lifecycle events (start, shutdown)
      │
      ├──▶ database.NewPostgres(ctx, cfg)  ← receives logger (future)
      │
      ├──▶ database.NewRedis(ctx, cfg)     ← receives logger (future)
      │
      ├──▶ HTTP middleware                 ← creates per-request child via With()
      │         │
      │         └──▶ logger.WithContext()  ← injects child into request context
      │
      ├──▶ Handlers                        ← logger.FromContext(ctx)
      │
      ├──▶ Services (Auth, Party, Chat)    ← injected via constructor
      │
      └──▶ Repositories                   ← injected via constructor
```

---

## Package Dependency Rule

```
✅  logger  ←  app
✅  logger  ←  database
✅  logger  ←  server
✅  logger  ←  handlers
✅  logger  ←  services/*
✅  logger  ←  repositories/*

❌  logger  →  config     (logger does not import config)
❌  logger  →  database
❌  logger  →  services/*
❌  zerolog  ←  any package except logger
```

The logger package depends only on the Go standard library and zerolog. Nothing in the application layer is imported by it.

---

## Testing

### Capture log output

```go
func TestSomething(t *testing.T) {
    var buf bytes.Buffer
    log := logger.New(logger.WithWriter(&buf))

    svc := NewMyService(log)
    svc.DoWork()

    if !strings.Contains(buf.String(), "expected message") {
        t.Fatalf("expected log entry not found")
    }
}
```

### Discard all output

```go
func TestSomethingFast(t *testing.T) {
    log := logger.Nop()
    svc := NewMyService(log)
    // log output is silently discarded
}
```

---

## Future Extensions

The following can be added **without changing the `Logger` interface or any consumer**:

| Extension | How |
|---|---|
| File output with rotation | Swap `WithWriter` to an `io.Writer` backed by `lumberjack` or similar |
| Log sampling | Wrap the zerolog sampler inside `New()` behind a new `WithSampling` option |
| OpenTelemetry trace injection | Add a `WithTraceID` field injected by middleware; no interface change needed |
| Sentry error capture | Create a wrapping `Logger` implementation that forwards `Error`/`Fatal` calls to Sentry before delegating to the inner logger |
| Multiple outputs (file + stdout) | Use `zerolog.MultiLevelWriter` inside `New()` behind a `WithMultiWriter` option |
