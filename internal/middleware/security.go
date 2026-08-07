package middleware

import (
    "fmt"

    "github.com/gofiber/fiber/v2"
)

// Security sets HTTP security headers on every response.
func Security(deps Deps) fiber.Handler {
    mc := deps.Config.Middleware
    return func(c *fiber.Ctx) error {
        c.Set("X-Content-Type-Options", "nosniff")
        c.Set("X-Frame-Options", "DENY")
        c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Set("X-XSS-Protection", "0")
        if mc.ContentSecurityPolicy != "" {
            c.Set("Content-Security-Policy", mc.ContentSecurityPolicy)
        }
        if mc.HSTSEnabled {
            c.Set("Strict-Transport-Security",
                fmt.Sprintf("max-age=%d; includeSubDomains", mc.HSTSMaxAge))
        }
        return c.Next()
    }
}