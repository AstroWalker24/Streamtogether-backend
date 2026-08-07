package middleware

import (
    "fmt"
    "runtime/debug"

    "github.com/gofiber/fiber/v2"

    "github.com/AstroWalker24/Streamtogether-backend/internal/api"
    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
    apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// Recovery catches panics, logs them, and returns a standardized 500 response.
// Stack traces appear in logs only in the development environment.
func Recovery(deps Deps) fiber.Handler {
    isDev := deps.Config.App.Environment == config.EnvDevelopment
    log := deps.Logger
    return func(c *fiber.Ctx) (err error) {
        defer func() {
            r := recover()
            if r == nil {
                return
            }
            fields := []logger.Field{
                logger.String("panic", fmt.Sprintf("%v", r)),
                logger.String("method", c.Method()),
                logger.String("path", c.Path()),
            }
            if isDev {
                fields = append(fields, logger.String("stack", string(debug.Stack())))
            }
            log.Error("panic recovered", fields...)
            c.Response().ResetBody()
            err = api.Error(c, apperrors.NewInternal("internal server error"))
        }()
        return c.Next()
    }
}