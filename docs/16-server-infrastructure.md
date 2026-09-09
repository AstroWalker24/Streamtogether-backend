# 16 — HTTP Server Infrastructure

## Overview

The `internal/server` package is the HTTP server infrastructure layer for the entire backend. It wraps [Fiber v2](https://github.com/gofiber/fiber) behind a clean Go struct, providing a production-ready server that handles startup, graceful shutdown, route registration, and centralised error handling. **No other package constructs or owns a Fiber application** — they receive a `*Server` through constructor injection and attach routes via `RegisterRoutes`.

The server package has no knowledge of authentication, business logic, databases, or any domain concept. It is a pure infrastructure concern.

---

## Package Structure

```
internal/server/
├── server.go      — Server struct, New(), App()
├── options.go     — buildFiberConfig(): config → fiber.Config
├── errors.go      — newErrorHandler(): centralised JSON error responses
├── lifecycle.go   — Start(), Shutdown()
├── routes.go      — RegisterRoutes(): injection point for all API modules
└── context.go     — Locals key constants and RequestID() accessor
```

---

## Design Principles

| Principle | How it is applied |
|---|---|
| **Single owner** | Only one `*Server` exists in the application, created in `bootstrap.go` |
| **Constructor injection** | Every consumer receives `*server.Server` as an argument — no globals |
| **Private Fiber app** | `app *fiber.App` is unexported; callers use `App()` only when direct access is justified |
| **No hardcoded values** | Every Fiber config field maps 1-to-1 to a `config.ServerConfig` or `config.AppConfig` field |
| **Thread safety** | No mutable shared state after construction; Fiber is safe for concurrent use |
| **Separation of concerns** | The server never imports handlers, services, repositories, or auth packages |
| **Testability** | `New()` accepts `*config.Config` and `logger.Logger`; isolated instances can be created in tests without a running process |

---

## Public API

```go
// Construct
func New(cfg *config.Config, log logger.Logger) (*Server, error)

// Fiber app access — use sparingly (prefer RegisterRoutes)
func (s *Server) App() *fiber.App

// Route injection point for API modules
func (s *Server) RegisterRoutes(fn func(router fiber.Router))

// Lifecycle
func (s *Server) Start(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
```

---

## Fiber Configuration

`buildFiberConfig` in `options.go` is the single place that translates config values into `fiber.Config`. No value is hardcoded there.

| `config.ServerConfig` field | `fiber.Config` field | Default |
|---|---|---|
| `cfg.App.Name` | `AppName` | `"Streamtogether Backend"` |
| `Server.BodyLimit` | `BodyLimit` | `4194304` (4 MB) |
| `Server.ReadBufferSize` | `ReadBufferSize` | `4096` |
| `Server.WriteBufferSize` | `WriteBufferSize` | `4096` |
| `Server.ReadTimeout` | `ReadTimeout` | `15s` |
| `Server.WriteTimeout` | `WriteTimeout` | `15s` |
| `Server.IdleTimeout` | `IdleTimeout` | `60s` |
| `Server.CaseSensitive` | `CaseSensitive` | `false` |
| `Server.StrictRouting` | `StrictRouting` | `false` |
| `Server.Immutable` | `Immutable` | `false` |
| _(always)_ | `DisableStartupMessage` | `true` |
| _(always)_ | `ErrorHandler` | `newErrorHandler(log)` |

### Environment variables

```env
SERVER_BODY_LIMIT=4194304
SERVER_READ_BUFFER_SIZE=4096
SERVER_WRITE_BUFFER_SIZE=4096
SERVER_CASE_SENSITIVE=false
SERVER_STRICT_ROUTING=false
SERVER_IMMUTABLE=false
```

The existing timeout variables (`READ_TIMEOUT`, `WRITE_TIMEOUT`, `IDLE_TIMEOUT`, `SHUTDOWN_TIMEOUT`) are reused — no new timeout env vars are needed.

---

## Lifecycle

### Start

```go
func (s *Server) Start(_ context.Context) error
```

`Start` calls `app.Listen(addr)`, which **blocks** until the server stops. It is designed to be called in a goroutine — exactly as `app/lifecycle.go` does:

```go
go func() {
    serverErr <- a.server.Start(ctx)
}()
```

When `Shutdown` is called from another goroutine, `Listen` returns `nil` and the goroutine exits cleanly. If the port is already in use or the network is unavailable, `Listen` returns immediately with an error.

Logs emitted:
```
{"level":"info","message":"server starting","address":"0.0.0.0:8080"}
```

### Shutdown

```go
func (s *Server) Shutdown(ctx context.Context) error
```

`Shutdown` calls Fiber's `ShutdownWithContext`, which waits for all in-flight requests to complete before closing the listener. The `ShutdownTimeout` from `config.ServerConfig` is applied by the caller (`app/lifecycle.go`) via `context.WithTimeout`.

```go
ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
defer cancel()

if err := srv.Shutdown(ctx); err != nil { ... }
```

If the context deadline is exceeded before all requests drain, `ShutdownWithContext` returns a timeout error, which is wrapped and returned to the caller.

Logs emitted:
```
{"level":"info","message":"server shutdown initiated"}
{"level":"info","message":"server shutdown complete"}
```

---

## Centralised Error Handler

`newErrorHandler` in `errors.go` is registered as Fiber's global `ErrorHandler`. Every unhandled error in any route — whether returned explicitly or panicked (with recovery middleware) — flows through this handler.

### Behaviour

| Error type | Action |
|---|---|
| `*fiber.Error` | Maps `Code` → string code, returns the Fiber message |
| Any other error | Logs at `Error` level, returns a generic 500 response |

Stack traces are **never** included in the response body.

### Response shape

```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "Cannot GET /unknown"
  }
}
```

### Status code → string code mapping

| HTTP status | `code` string |
|---|---|
| 400 | `BAD_REQUEST` |
| 401 | `UNAUTHORIZED` |
| 403 | `FORBIDDEN` |
| 404 | `NOT_FOUND` |
| 405 | `METHOD_NOT_ALLOWED` |
| 409 | `CONFLICT` |
| 422 | `UNPROCESSABLE_ENTITY` |
| 429 | `TOO_MANY_REQUESTS` |
| 500+ | `INTERNAL_ERROR` |

### Returning an error from a handler

```go
// Return a specific HTTP error:
return fiber.NewError(fiber.StatusNotFound, "party not found")

// Return a generic 500 (logged, not exposed):
return fmt.Errorf("unexpected db failure: %w", err)
```

---

## Route Registration

`RegisterRoutes` provides the injection point for every API module. The server package never knows which routes exist — modules self-register by calling this method during bootstrap.

```go
// In bootstrap.go, after srv is created:
srv.RegisterRoutes(func(r fiber.Router) {
    api := r.Group("/api/v1")
    health.Register(api)
    parties.Register(api)
    // ...
})
```

Each module receives a `fiber.Router` and mounts its own sub-group and handlers. The server is only the mounting surface.

---

## Context Helpers

`context.go` defines unexported Locals key constants and accessor functions for values stored by middleware.

```go
// KeyRequestID is the Fiber Locals key written by request-ID middleware.
const KeyRequestID contextKey = "request_id"

// RequestID returns the request ID set by middleware, or "" if not present.
func RequestID(c *fiber.Ctx) string
```

Middleware stores values via:
```go
c.Locals(server.KeyRequestID, generatedID)
```

Handlers and downstream middleware read values via:
```go
id := server.RequestID(c)
```

This avoids string-key collisions across packages. All Locals keys for server-managed values are defined here.

---

## How Every Future Package Will Use This

API modules register their routes through `RegisterRoutes`. They never import the Fiber app directly.

### Pattern: module self-registration

```go
// internal/handlers/health/health.go
package health

import "github.com/gofiber/fiber/v2"

type Handler struct{ ... }

func NewHandler(...) *Handler { ... }

// Register mounts the health routes on the given router.
func (h *Handler) Register(r fiber.Router) {
    r.Get("/health/live",  h.Live)
    r.Get("/health/ready", h.Ready)
}
```

```go
// bootstrap.go
healthHandler := health.NewHandler(db, redis, log)

srv.RegisterRoutes(func(r fiber.Router) {
    healthHandler.Register(r)
})
```

### Pattern: group with shared prefix

```go
srv.RegisterRoutes(func(r fiber.Router) {
    v1 := r.Group("/api/v1")
    partyHandler.Register(v1)   // mounts /api/v1/parties
    userHandler.Register(v1)    // mounts /api/v1/users
})
```

### Pattern: reading request context in a handler

```go
func (h *PartyHandler) Create(c *fiber.Ctx) error {
    requestID := server.RequestID(c)  // set by request-ID middleware
    // ...
}
```

---

## Integration Map

```
config.Config
      │
      ▼
server.New(cfg, log)               ← constructed once in bootstrap.go
      │
      ├──▶ App.server               ← lifecycle owner (Start, Shutdown)
      │
      ├──▶ srv.RegisterRoutes(...)  ← API modules attach routes here
      │         │
      │         ├──▶ /health/live
      │         ├──▶ /health/ready
      │         ├──▶ /api/v1/parties
      │         ├──▶ /api/v1/users
      │         └──▶ (future routes)
      │
      ├──▶ srv.App()                ← direct Fiber access (middleware registration)
      │         │
      │         ├──▶ Recovery middleware
      │         ├──▶ Request-ID middleware
      │         ├──▶ Logging middleware
      │         ├──▶ CORS middleware
      │         └──▶ (future middleware)
      │
      └──▶ ErrorHandler             ← all unhandled errors flow here
```

---

## Package Dependency Rule

```
✅  server  ←  app
✅  server  ←  handlers/*          (handlers receive fiber.Router via RegisterRoutes)

❌  server  →  handlers/*
❌  server  →  services/*
❌  server  →  repositories/*
❌  server  →  database
❌  server  →  redis
❌  fiber.App  ←  any package except server (use RegisterRoutes or App())
```

The server package depends only on `config`, `logger`, `fiber/v2`, and the standard library.

---

## Testing

### Unit tests (no network required)

Fiber supports in-process testing via `app.Test`. Instantiate a server directly with a test config and logger:

```go
func newTestServer(t *testing.T) *server.Server {
    t.Helper()
    cfg := &config.Config{
        App:    config.AppConfig{Name: "test", Host: "127.0.0.1", Port: 0},
        Server: config.ServerConfig{BodyLimit: 1 << 20},
    }
    srv, err := server.New(cfg, logger.Nop())
    if err != nil {
        t.Fatalf("server.New: %v", err)
    }
    return srv
}
```

Test a handler without binding a port:

```go
func TestHealthRoute(t *testing.T) {
    srv := newTestServer(t)
    srv.RegisterRoutes(func(r fiber.Router) {
        r.Get("/health/live", func(c *fiber.Ctx) error {
            return c.SendStatus(fiber.StatusOK)
        })
    })

    req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
    resp, err := srv.App().Test(req)
    if err != nil {
        t.Fatal(err)
    }
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
}
```

### Integration tests (requires running server)

```bash
make up            # start postgres + redis via docker compose
make test-e2e      # run end-to-end API tests
```

---

## Future Extensions

The following can be added **without changing the server's public API**:

| Extension | How |
|---|---|
| Recovery middleware | `srv.App().Use(recover.New())` in bootstrap, before `RegisterRoutes` |
| Request-ID middleware | `srv.App().Use(requestid.New())` — writes to `server.KeyRequestID` Locals |
| Structured request logging | Middleware using `server.RequestID(c)` and the logger |
| CORS | `srv.App().Use(cors.New(cors.Config{...}))` driven by `config.CORSConfig` |
| Rate limiting | `srv.App().Use(limiter.New(...))` driven by `config.RateLimitConfig` |
| TLS | Replace `app.Listen` with `app.ListenMutualTLS` in `lifecycle.go` |
| JSON encoder swap | Set `fiber.Config.JSONEncoder` / `JSONDecoder` in `buildFiberConfig` |
