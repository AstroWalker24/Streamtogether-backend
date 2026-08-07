package middleware

import (
    "context"

    "github.com/gofiber/fiber/v2"

    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// fibLocalsKey is the private type for all Fiber Locals keys owned by this package.
type fibLocalsKey string

const (
    localsKeyRequestID fibLocalsKey = "request_id"
    localsKeyLogger    fibLocalsKey = "logger"
)

// stdCtxKey is the private type for all standard context.Context keys owned by this package.
type stdCtxKey struct{ name string }

var ctxKeyRequestID = stdCtxKey{"request_id"}

// RequestID returns the request ID stored in Fiber Locals, or an empty string.
func RequestID(c *fiber.Ctx) string {
    id, _ := c.Locals(localsKeyRequestID).(string)
    return id
}

// RequestIDFromContext retrieves the request ID from a standard context.Context.
func RequestIDFromContext(ctx context.Context) string {
    id, _ := ctx.Value(ctxKeyRequestID).(string)
    return id
}

// GetLogger returns the request-scoped logger from Fiber Locals, falling back to Nop.
func GetLogger(c *fiber.Ctx) logger.Logger {
    if l, ok := c.Locals(localsKeyLogger).(logger.Logger); ok {
        return l
    }
    return logger.Nop()
}

// setRequestID stores the request ID in Fiber Locals and the underlying Go context.
func setRequestID(c *fiber.Ctx, id string) {
    c.Locals(localsKeyRequestID, id)
    c.SetUserContext(context.WithValue(c.UserContext(), ctxKeyRequestID, id))
}

// setLogger stores a request-scoped logger in Fiber Locals and the underlying Go context.
func setLogger(c *fiber.Ctx, l logger.Logger) {
    c.Locals(localsKeyLogger, l)
    c.SetUserContext(logger.WithContext(c.UserContext(), l))
}