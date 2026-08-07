package middleware

import (
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/compress"
)

// Compression enables gzip response compression when configured.
// Swap compress.LevelBestSpeed for other levels as needed.
func Compression(deps Deps) fiber.Handler {
    if !deps.Config.Middleware.CompressionEnabled {
        return func(c *fiber.Ctx) error { return c.Next() }
    }
    return compress.New(compress.Config{
        Level: compress.LevelBestSpeed,
    })
}