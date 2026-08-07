package middleware

import (
    "github.com/gofiber/fiber/v2"
    "github.com/google/uuid"
)

const headerXRequestID = "X-Request-ID"

// RequestID propagates an existing X-Request-ID header or generates a new UUID.
// The ID is stored in Fiber Locals, the Go context, and echoed in the response header.
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