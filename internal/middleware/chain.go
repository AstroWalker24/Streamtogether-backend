package middleware

import "github.com/gofiber/fiber/v2"

// Chain is an ordered, reusable collection of Fiber handlers.
type Chain struct {
	handlers []fiber.Handler
}

// New returns an empty Chain ready for composition.
func New() *Chain {
	return &Chain{}
}

// Use appends one or more handlers to the chain and returns it for fluent chaining.
func (c *Chain) Use(handlers ...fiber.Handler) *Chain {
	c.handlers = append(c.handlers, handlers...)
	return c
}

// Handlers returns a defensive copy of the handler slice for use with app.Use or router.Use.
func (c *Chain) Handlers() []fiber.Handler {
	out := make([]fiber.Handler, len(c.handlers))
	copy(out, c.handlers)
	return out
}

// Apply registers all handlers in the chain onto a Fiber app.
func (c *Chain) Apply(app *fiber.App) {
	for _, h := range c.handlers {
		app.Use(h)
	}
}
