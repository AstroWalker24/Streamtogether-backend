package server

import "github.com/gofiber/fiber/v2"

// contextKey is an unexported type for Fiber Locals keys to prevent collisions.
type contextKey string

const (
    // KeyRequestID is the Locals key written by request-ID middleware.
    KeyRequestID contextKey = "request_id"
)

// RequestID returns the request ID stored in Fiber Locals by middleware.
// Returns an empty string when no request ID is present.
func RequestID(c *fiber.Ctx) string {
    id, _ := c.Locals(KeyRequestID).(string)
    return id
}