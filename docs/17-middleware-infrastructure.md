# Middleware Infrastructure

## Overview

The `internal/middleware` package provides a reusable, composable HTTP middleware framework built on top of [Fiber v2](https://github.com/gofiber/fiber). Every request that enters the server passes through this framework automatically before reaching a route handler.

The framework is designed around three principles:

- **No business logic** — middleware only concerns itself with cross-cutting concerns (logging, security, tracing).
- **Constructor injection** — every middleware receives its dependencies at startup, not at request time.
- **Composability** — chains of middleware can be built and reused across different route groups.

---

## Package Structure

```
internal/middleware/
├── middleware.go     — Package entry point; defines the shared Deps struct
├── chain.go          — Chain type for composing ordered handler lists
├── registry.go       — Wires the global chain onto the Fiber app
├── context.go        — Typed context keys and request-scoped value accessors
├── recovery.go       — Panic recovery
├── requestid.go      — Request ID generation and propagation
├── logging.go        — Structured request/response logging
├── security.go       — HTTP security headers
├── cors.go           — Cross-origin resource sharing
├── compression.go    — Gzip response compression
├── timeout.go        — Request deadline management
└── ratelimit.go      — Limiter interface and config (no implementation yet)
```

---

## How It Is Wired

The application bootstraps the framework once, in `internal/app/bootstrap.go`:

```go
mwRegistry := middleware.NewRegistry(middleware.Deps{
    Config: cfg,
    Logger: log,
})
mwRegistry.Register(srv.App())
```

The `Registry` builds a global `Chain`, populates it in the correct order, and applies it to the Fiber app. After this point every request automatically passes through all middleware before reaching any route handler.

---

## Execution Order

Every request flows through the global chain in this sequence:

```
Incoming request
       │
       ▼
┌─────────────┐
│  Recovery   │  catches any downstream panic
└──────┬──────┘
       ▼
┌─────────────┐
│ Request ID  │  generates / propagates X-Request-ID
└──────┬──────┘
       ▼
┌─────────────┐
│   Logging   │  attaches scoped logger; logs after response
└──────┬──────┘
       ▼
┌─────────────┐
│  Security   │  sets X-Content-Type-Options, CSP, HSTS, etc.
└──────┬──────┘
       ▼
┌─────────────┐
│    CORS     │  handles preflight and CORS headers
└──────┬──────┘
       ▼
┌─────────────┐
│ Compression │  gzip-encodes the response body
└──────┬──────┘
       ▼
┌─────────────┐
│   Timeout   │  sets context deadline; returns 504 on expiry
└──────┬──────┘
       ▼
  Route handler
```

The ordering is intentional:
- **Recovery first** so panics in every other middleware are caught.
- **Request ID second** so every subsequent log entry can carry the ID.
- **Logging third** so it can record the full request duration including all remaining middleware.
- **Security and CORS** before the body is produced, since they only set headers.
- **Compression last** (before the handler) so it compresses the final response, including any error bodies.
- **Timeout wraps the handler** so the deadline is active during actual business logic.

---

## Components

### `Deps` — Shared Dependencies

```go
type Deps struct {
    Config *config.Config
    Logger logger.Logger
}
```

A single struct that carries every shared dependency. All middleware constructors that need config or a logger accept `Deps`. This avoids long parameter lists and makes future dependency additions non-breaking.

**Go concept — Struct as parameter object:** Instead of passing `*config.Config` and `logger.Logger` individually to every constructor, they are grouped into `Deps`. This is sometimes called the *parameter object* pattern. It keeps call sites stable when new shared dependencies are introduced.

---

### `Chain` — Composable Handler List

```go
chain := middleware.New()
chain.Use(Auth())
chain.Use(RBAC())

router.Use(chain.Handlers()...)
// or
chain.Apply(app)
```

`Chain` is the core composition primitive. It stores an ordered list of `fiber.Handler` values and exposes three operations:

| Method | Purpose |
|---|---|
| `Use(...fiber.Handler) *Chain` | Append one or more handlers |
| `Handlers() []fiber.Handler` | Return a defensive copy of the handler slice |
| `Apply(*fiber.App)` | Register all handlers onto a Fiber app |

**Go concept — Fluent interface / method chaining:** `Use` returns `*Chain`, so calls can be chained:
```go
middleware.New().Use(A).Use(B).Use(C).Apply(app)
```
This is safe because `Use` mutates the receiver and then returns it.

**Go concept — Defensive copy:** `Handlers()` returns a fresh slice backed by a `make` + `copy` rather than exposing the internal slice directly. This prevents callers from accidentally mutating the chain's state after construction.

```go
func (c *Chain) Handlers() []fiber.Handler {
    out := make([]fiber.Handler, len(c.handlers))
    copy(out, c.handlers)
    return out
}
```

---

### `Registry` — Execution Order Owner

```go
func (r *Registry) Register(app *fiber.App) {
    global := New()
    global.Use(Recovery(r.deps))
    global.Use(NewRequestID())
    // ...
    global.Apply(app)
}
```

The `Registry` owns the decision of *which* middleware runs and *in what order*. Nothing outside this struct needs to know the ordering. The HTTP server only calls `registry.Register(app)`.

**Go concept — Single responsibility:** `Registry` is responsible purely for wiring. `Chain` is responsible purely for composition. Neither has the other's concern. This split allows `Chain` to be reused for route-group-level composition (e.g. an API chain, an admin chain) without coupling to global registration.

---

### `context.go` — Type-Safe Request-Scoped Values

This file manages how values (request ID, logger) travel through a request and remain accessible to any downstream handler.

The framework writes to two storage layers simultaneously:

| Layer | API | Used by |
|---|---|---|
| Fiber Locals | `c.Locals(key, value)` | Handlers using `*fiber.Ctx` |
| Go standard context | `context.WithValue(...)` | Any code receiving a `context.Context` |

**Go concept — Typed context keys to prevent collisions:**

Using a plain `string` as a context key is dangerous because any package could accidentally use the same string:

```go
// BAD — any package could collide on this string
ctx.Value("request_id")
```

Instead, a private named type is used:

```go
type fibLocalsKey string      // for Fiber Locals
type stdCtxKey struct{ name string }  // for std context
```

Because `fibLocalsKey` and `stdCtxKey` are unexported types defined in this package, no outside package can construct a matching key. Even if another package uses the string `"request_id"`, the types differ and `ctx.Value()` returns nil.

**Go concept — Type assertion with comma-ok idiom:**

```go
func RequestID(c *fiber.Ctx) string {
    id, _ := c.Locals(localsKeyRequestID).(string)
    return id
}
```

`c.Locals()` returns `interface{}`. The two-value type assertion `.(string)` returns the value and a boolean `ok`. When the key is not present, `ok` is `false` and `id` is the zero value (`""`). The blank identifier `_` discards the boolean since returning empty string is the correct fallback behaviour.

---

### `recovery.go` — Panic Recovery

```go
func Recovery(deps Deps) fiber.Handler {
    isDev := deps.Config.App.Environment == config.EnvDevelopment
    log := deps.Logger
    return func(c *fiber.Ctx) (err error) {
        defer func() {
            r := recover()
            if r == nil {
                return
            }
            // ...
            err = api.Error(c, apperrors.NewInternal("internal server error"))
        }()
        return c.Next()
    }
}
```

**Go concept — Named return value + defer to catch panics:**

The inner handler uses a *named return value* `(err error)`. This is essential for the pattern to work. When `recover()` fires inside a `defer`, the function has already begun unwinding. The only way to change what the function returns from inside a `defer` is to write to a named return variable. Without `(err error)`, the assignment `err = api.Error(...)` would write to a local variable that is discarded.

**Go concept — Closure over captured state:** The `isDev` boolean and `log` are captured from the constructor's scope. They are evaluated once at startup, not on every request. This means there is no config lookup overhead per request.

**Go concept — `runtime/debug.Stack()`:** Returns the goroutine stack trace at the point of the call as a `[]byte`. Only included in log output when `isDev` is true, so production logs never expose internal call stacks to operators who might inadvertently leak them.

---

### `requestid.go` — Request ID

```go
func NewRequestID() fiber.Handler {
    return func(c *fiber.Ctx) error {
        id := c.Get(headerXRequestID)
        if id == "" {
            id = uuid.New().String()
        }
        setRequestID(c, id)
        c.Set(headerXRequestID, id)
        return c.Next()
    }
}
```

Generates a [UUID v4](https://pkg.go.dev/github.com/google/uuid) or propagates an existing `X-Request-ID` header from the client (useful when a reverse proxy or mobile client sends one). The ID is written to three places: Fiber Locals, the Go context, and the response header.

**Go concept — Prefer reuse over generation:** Propagating a client-provided ID allows distributed tracing across services. A load balancer can stamp the ID and every downstream service in the call chain can log the same ID.

---

### `logging.go` — Structured Request Logging

```go
func Logging(deps Deps) fiber.Handler {
    log := deps.Logger
    return func(c *fiber.Ctx) error {
        start := time.Now()
        reqLog := log.With(
            logger.String("request_id", RequestID(c)),
            // ...
        )
        setLogger(c, reqLog)

        err := c.Next()   // ← all downstream runs here

        reqLog.Info("request completed",
            logger.Int("status", c.Response().StatusCode()),
            logger.Duration("duration", time.Since(start)),
            // ...
        )
        return err
    }
}
```

**Go concept — Wrap-around middleware pattern:** Code before `c.Next()` runs on the way *in* (before the handler). Code after `c.Next()` runs on the way *out* (after the response is assembled). This is the idiomatic way to measure request duration — capture `time.Now()` before, compute `time.Since(start)` after.

The enriched logger (`reqLog`) is stored in context via `setLogger`. Every downstream piece of code that calls `logger.FromContext(c.UserContext())` or `middleware.GetLogger(c)` receives this pre-enriched logger automatically, without needing to pass it explicitly.

**Note on response size:** `len(c.Response().Body())` after `c.Next()` returns the size of the *compressed* body if `Compression` middleware is active, because compression runs inside the `c.Next()` call relative to the logging middleware. This reflects the actual bytes sent over the wire.

---

### `security.go` — HTTP Security Headers

Sets the following headers unconditionally on every response:

| Header | Value | Purpose |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | Prevents browsers from MIME-sniffing responses |
| `X-Frame-Options` | `DENY` | Blocks the app from being embedded in iframes (clickjacking) |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Limits referrer leakage across origins |
| `X-XSS-Protection` | `0` | Explicitly disables the broken IE/Chrome XSS filter; CSP is used instead |

Conditionally:

| Header | Condition |
|---|---|
| `Content-Security-Policy` | When `MIDDLEWARE_CSP` env var is set |
| `Strict-Transport-Security` | When `MIDDLEWARE_HSTS_ENABLED=true` |

**Go concept — Closure captures computed values:** The `mc := deps.Config.Middleware` assignment happens once when the middleware is constructed. The returned handler closes over `mc`, so every request reads from a struct value already in memory rather than re-reading config.

---

### `cors.go` — Cross-Origin Resource Sharing

Delegates to Fiber's built-in `cors` middleware, mapping the application `CORSConfig` to `cors.Config`:

```go
func CORS(deps Deps) fiber.Handler {
    cc := deps.Config.CORS
    return cors.New(cors.Config{
        AllowOrigins:     joinOrWildcard(cc.AllowedOrigins),
        AllowMethods:     strings.Join(cc.AllowedMethods, ","),
        AllowHeaders:     strings.Join(cc.AllowedHeaders, ","),
        AllowCredentials: cc.AllowCredentials,
        ExposeHeaders:    strings.Join(cc.ExposeHeaders, ","),
        MaxAge:           cc.MaxAge,
    })
}
```

Fiber's CORS middleware handles preflight `OPTIONS` requests automatically and short-circuits before reaching route handlers.

**Design decision — Delegate, don't reimplement:** CORS is specified by a complex W3C standard with many edge cases. Using Fiber's tested implementation and mapping our config onto it is safer than a hand-rolled CORS handler.

---

### `compression.go` — Response Compression

```go
func Compression(deps Deps) fiber.Handler {
    if !deps.Config.Middleware.CompressionEnabled {
        return func(c *fiber.Ctx) error { return c.Next() }
    }
    return compress.New(compress.Config{Level: compress.LevelBestSpeed})
}
```

**Go concept — No-op handler as feature flag:** When compression is disabled, the constructor returns a handler that does nothing but call `c.Next()`. This is cheaper than adding an `if` check inside a handler that runs on every request. The branch is resolved once at startup.

`compress.LevelBestSpeed` trades some compression ratio for significantly lower CPU usage, which is the right default for a real-time application like Streamtogether.

---

### `timeout.go` — Request Deadline

```go
func Timeout(deps Deps) fiber.Handler {
    d := deps.Config.Middleware.RequestTimeout
    return func(c *fiber.Ctx) error {
        ctx, cancel := context.WithTimeout(c.UserContext(), d)
        defer cancel()
        c.SetUserContext(ctx)

        err := c.Next()

        if ctx.Err() == context.DeadlineExceeded {
            // ...
            return api.Error(c, apperrors.NewTimeout("request timed out"))
        }
        return err
    }
}
```

**Go concept — Context deadlines propagate automatically:** `context.WithTimeout` returns a new context that is automatically cancelled when the deadline expires. Any downstream code that passes this context to database queries (`db.QueryContext(ctx, ...)`), Redis calls, or HTTP client requests will have those calls cancelled at the deadline. The middleware does not need to know anything about the handler's internals.

**Go concept — Always `defer cancel()`:** `context.WithTimeout` allocates a timer. Calling `cancel()` via defer releases that timer even when the request completes before the deadline. Omitting `cancel()` would cause a goroutine leak for every fast request.

**Why no goroutine:** Some timeout middleware implementations spawn a goroutine, call `c.Next()` inside it, and race a timer against it. This creates goroutine leaks when the handler does not respect context and runs past the deadline. This implementation calls `c.Next()` synchronously — handlers that propagate context cancel cleanly, and the deadline check after `c.Next()` catches any that ran long.

---

### `ratelimit.go` — Limiter Interface

```go
type Limiter interface {
    Middleware() fiber.Handler
}

type LimiterConfig struct {
    Enabled  bool
    Requests int
    Window   time.Duration
}
```

No implementation is provided yet. The first real implementation will be Redis-backed (shared state across multiple server instances). An in-memory implementation would be deleted in a few weeks once Redis is wired in, so it is not worth building.

**Go concept — Interface as a seam for future implementations:** Defining the interface now lets the `Registry` be extended later with zero changes to its existing code:

```go
func (r *Registry) RegisterLimiter(l Limiter) {
    r.app.Use(l.Middleware())
}
```

Any struct that has a `Middleware() fiber.Handler` method satisfies `Limiter`. The registry does not care whether the backing store is Redis, an in-process map, or a third-party service.

**Go concept — Interface segregation:** `Limiter` has exactly one method. A type only needs to implement what it actually does. Small interfaces compose better than large ones and are easier to mock in tests.

---

## Go Concepts Summary

| Concept | Where Used |
|---|---|
| **Closure over captured state** | All constructors — config/logger captured once at startup |
| **Named return values** | `recovery.go` — required to assign `err` from inside `defer` |
| **Defer for cleanup** | `timeout.go` — `defer cancel()` releases the context timer |
| **Wrap-around middleware** | `logging.go` — code before and after `c.Next()` |
| **Type-safe context keys** | `context.go` — private named types prevent key collisions |
| **Comma-ok type assertion** | `context.go` — safe retrieval from `interface{}` storage |
| **Parameter object (Deps)** | `middleware.go` — groups shared dependencies into one struct |
| **No-op handler as feature flag** | `compression.go`, `timeout.go` — resolved at startup, zero per-request cost |
| **Defensive copy** | `chain.go` — `Handlers()` protects internal state |
| **Fluent interface** | `chain.go` — `Use()` returns `*Chain` for method chaining |
| **Interface as a seam** | `ratelimit.go` — `Limiter` decouples the registry from backend choice |
| **Interface segregation** | `ratelimit.go` — one-method interface, easy to implement and mock |
| **Delegate, don't reimplement** | `cors.go` — maps config onto Fiber's tested CORS implementation |
| **Context deadline propagation** | `timeout.go` — deadline flows to DB/Redis without explicit passing |

---

## Extending the Framework

### Adding a new global middleware

1. Create a new file `internal/middleware/mymiddleware.go`.
2. Define a constructor: `func MyMiddleware(deps Deps) fiber.Handler { ... }`.
3. Add `global.Use(MyMiddleware(r.deps))` in `registry.go` at the appropriate position.

### Adding a route-group-level chain

```go
// In a future handler registration function:
authChain := middleware.New()
authChain.Use(Auth(deps))
authChain.Use(RBAC(deps))

api := app.Group("/api/v1")
api.Use(authChain.Handlers()...)
```

The `Chain` type is designed exactly for this. The global registry and route-group chains are independent.

### Adding the Redis rate limiter

1. Create a struct (e.g. in `internal/middleware/ratelimit_redis.go`) that holds a Redis client.
2. Implement `Middleware() fiber.Handler` on it.
3. Wire it into the registry:
   ```go
   func (r *Registry) RegisterLimiter(l Limiter) {
       r.app.Use(l.Middleware())
   }
   ```
4. Call `mwRegistry.RegisterLimiter(redisLimiter)` in `bootstrap.go` after the Redis client is ready.

No existing middleware file changes are needed.
