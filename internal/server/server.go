// Package server provides a production-ready HTTP server built on Fiber v2.
package server

import (
    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
    "github.com/gofiber/fiber/v2"
)


type Server struct {
    app *fiber.App
    cfg *config.Config
    log logger.Logger
}

func New(cfg *config.Config, log logger.Logger) (*Server, error) {
    app := fiber.New(buildFiberConfig(cfg, log))

    s := &Server{
        app: app,
        cfg: cfg,
        log: log,
    }

    log.Info("server created",
        logger.String("name", cfg.App.Name),
        logger.String("address", cfg.App.Address()),
    )

    return s, nil
}


func (s *Server) App() *fiber.App {
    return s.app
}