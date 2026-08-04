package server

import (
	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/gofiber/fiber/v2"
)

// buildFiberConfig maps application config values to a fiber.Config.
func buildFiberConfig(cfg *config.Config, log logger.Logger) fiber.Config {
	return fiber.Config{
		AppName:               cfg.App.Name,
		BodyLimit:             cfg.Server.BodyLimit,
		ReadBufferSize:        cfg.Server.ReadBufferSize,
		WriteBufferSize:       cfg.Server.WriteBufferSize,
		ReadTimeout:           cfg.Server.ReadTimeout,
		WriteTimeout:          cfg.Server.WriteTimeout,
		IdleTimeout:           cfg.Server.IdleTimeout,
		CaseSensitive:         cfg.Server.CaseSensitive,
		StrictRouting:         cfg.Server.StrictRouting,
		Immutable:             cfg.Server.Immutable,
		DisableStartupMessage: true,
		ErrorHandler:          newErrorHandler(log),
	}
}
