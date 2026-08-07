package middleware

import (
    "time"

    "github.com/gofiber/fiber/v2"

    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
)

// Limiter is the contract for pluggable rate limiter backends.
// Wire a concrete implementation into the Registry once available.
type Limiter interface {
    // Middleware returns a Fiber handler that enforces the rate limit.
    Middleware() fiber.Handler
}

// LimiterConfig holds the parameters for constructing a Limiter.
type LimiterConfig struct {
    Enabled  bool
    Requests int
    Window   time.Duration
}

// NewLimiterConfig maps application config to LimiterConfig.
func NewLimiterConfig(cfg *config.Config) LimiterConfig {
    return LimiterConfig{
        Enabled:  cfg.RateLimit.Enabled,
        Requests: cfg.RateLimit.Requests,
        Window:   cfg.RateLimit.Duration,
    }
}