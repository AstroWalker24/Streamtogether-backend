package app

import (
	"context"
	"fmt"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/handlers"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	redisx "github.com/AstroWalker24/Streamtogether-backend/internal/redis"
	"github.com/AstroWalker24/Streamtogether-backend/internal/routes"
	"github.com/AstroWalker24/Streamtogether-backend/internal/server"
)

func New() (*App, error) {
	// 1. Configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
	}

	// 2. Logger
	log := logger.New(
		logger.WithLevel(cfg.Logging.Level),
		logger.WithPretty(cfg.App.Environment == config.EnvDevelopment),
	)
	log.Info("configuration loaded",
		logger.String("env", string(cfg.App.Environment)),
		logger.String("version", cfg.App.Version),
	)

	ctx := context.Background()

	// 3. PostgreSQL
	db, err := database.New(ctx, cfg.Database, log)
	if err != nil {
		return nil, fmt.Errorf("app: init postgres: %w", err)
	}

	// 4. Redis
	redisInstance, err := redisx.New(ctx, cfg, log)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("app: init redis: %w", err)
	}

	// 5. Routes + handlers
	healthHandler := handlers.NewHealthHandler()
	handler := routes.Register(healthHandler)

	// 6. HTTP Server
	srv := server.New(cfg.Server, cfg.App, handler)
	log.Info("http server configured", logger.String("addr", cfg.App.Address()))

	return &App{
		cfg:    cfg,
		log:    log,
		db:     db,
		redis:  redisInstance,
		server: srv,
	}, nil
}
