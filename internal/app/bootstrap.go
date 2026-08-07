package app

import (
	"context"
	"fmt"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/health"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/AstroWalker24/Streamtogether-backend/internal/middleware"
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

	// 5. HTTP Server
	srv, err := server.New(cfg, log)
	if err != nil {
		_ = redisInstance.Close()
		db.Close()
		return nil, fmt.Errorf("app: init server: %w", err)
	}

	mwRegistry := middleware.NewRegistry(middleware.Deps{
		Config: cfg,
		Logger: log,
	})
	mwRegistry.Register(srv.App())

	// 6. Health checkers — required dependencies that must be up for /ready
	checkers := []health.Checker{
		health.NewChecker("postgres", true, db.Health),
		health.NewChecker("redis", true, redisInstance.Health),
	}

	// 7. Health package wiring
	healthSvc := health.NewService(cfg, log, checkers...)
	healthHandler := health.NewHandler(healthSvc)

	// 8. Route registration
	routes.Register(srv.App(), healthHandler)

	return &App{
		cfg:    cfg,
		log:    log,
		db:     db,
		redis:  redisInstance,
		server: srv,
	}, nil
}
