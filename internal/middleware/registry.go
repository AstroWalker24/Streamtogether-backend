package middleware

import "github.com/gofiber/fiber/v2"

// Registry wires all global middleware onto a Fiber app in the correct execution order.
type Registry struct {
	deps Deps
}

// NewRegistry returns a Registry backed by the given dependencies.
func NewRegistry(deps Deps) *Registry {
	return &Registry{deps: deps}
}

// Register attaches the global middleware chain to app.
// Rate limiting is omitted here; wire a Limiter once an implementation is provided.
func (r *Registry) Register(app *fiber.App) {
	global := New()
	global.Use(Recovery(r.deps))
	global.Use(NewRequestID())
	global.Use(Logging(r.deps))
	global.Use(Security(r.deps))
	global.Use(CORS(r.deps))
	global.Use(Compression(r.deps))
	global.Use(Timeout(r.deps))
	global.Apply(app)
}
