package middleware

import (
    "time"

    "github.com/gofiber/fiber/v2"

    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// Logging attaches a request-scoped logger to context and emits one structured log entry per request.
func Logging(deps Deps) fiber.Handler {
    log := deps.Logger
    return func(c *fiber.Ctx) error {
        start := time.Now()

        reqLog := log.With(
            logger.String("request_id", RequestID(c)),
            logger.String("method", c.Method()),
            logger.String("path", c.Path()),
            logger.String("ip", c.IP()),
        )
        setLogger(c, reqLog)

        err := c.Next()

        reqLog.Info("request completed",
            logger.Int("status", c.Response().StatusCode()),
            logger.Duration("duration", time.Since(start)),
            logger.String("user_agent", c.Get(fiber.HeaderUserAgent)),
            logger.Int("response_size", len(c.Response().Body())),
        )
        return err
    }
}