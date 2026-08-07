package middleware

import (
    "context"

    "github.com/gofiber/fiber/v2"

    "github.com/AstroWalker24/Streamtogether-backend/internal/api"
    apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// Timeout sets a deadline on the request context and returns a 504 if it expires.
// No goroutines are spawned; handlers that propagate context will abort naturally.
func Timeout(deps Deps) fiber.Handler {
    d := deps.Config.Middleware.RequestTimeout
    if d <= 0 {
        return func(c *fiber.Ctx) error { return c.Next() }
    }
    return func(c *fiber.Ctx) error {
        ctx, cancel := context.WithTimeout(c.UserContext(), d)
        defer cancel()
        c.SetUserContext(ctx)

        err := c.Next()

        if ctx.Err() == context.DeadlineExceeded {
            GetLogger(c).Warn("request timed out",
                logger.String("method", c.Method()),
                logger.String("path", c.Path()),
                logger.Duration("timeout", d),
            )
            c.Response().ResetBody()
            return api.Error(c, apperrors.NewTimeout("request timed out"))
        }
        return err
    }
}