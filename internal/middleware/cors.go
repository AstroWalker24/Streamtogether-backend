package middleware

import (
    "strings"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
)

// CORS configures cross-origin resource sharing from application config.
func CORS(deps Deps) fiber.Handler {
    cc := deps.Config.CORS
    return cors.New(cors.Config{
        AllowOrigins:     joinOrWildcard(cc.AllowedOrigins),
        AllowMethods:     strings.Join(cc.AllowedMethods, ","),
        AllowHeaders:     strings.Join(cc.AllowedHeaders, ","),
        AllowCredentials: cc.AllowCredentials,
        ExposeHeaders:    strings.Join(cc.ExposeHeaders, ","),
        MaxAge:           cc.MaxAge,
    })
}

// joinOrWildcard joins a slice into a comma-separated string, defaulting to "*".
func joinOrWildcard(origins []string) string {
    if len(origins) == 0 {
        return "*"
    }
    return strings.Join(origins, ",")
}