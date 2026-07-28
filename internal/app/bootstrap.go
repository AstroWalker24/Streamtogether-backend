package app

import (
	"context"
	"fmt"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/handlers"
	applogger "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/AstroWalker24/Streamtogether-backend/internal/routes"
	"github.com/AstroWalker24/Streamtogether-backend/internal/server"
)

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
	}

	log := applogger.New(cfg.Logging)
	log.Info("configuration loaded", "env", cfg.App.Environment, "version", cfg.App.Version)

	ctx := context.Background()
	db, err := database.NewPostgres(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("app: init postgres: %w", err)
	}
	log.Info("postgres connected", "host", cfg.Database.Host, "port", cfg.Database.Port)

	redisClient, err := database.NewRedis(ctx, cfg.Redis)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("app: init redis: %w", err)
	}
	log.Info("redis connected", "addr", cfg.Redis.Address())

	healthHandler := handlers.NewHealthHandler()
	handler := routes.Register(healthHandler)

	srv := server.New(cfg.Server, cfg.App, handler) 
	log.Info("http server configured", "addr", cfg.App.Address())

	return &App{
		cfg: cfg,
		log: log,
		db: db,
		redis: redisClient,
		server: srv,
	}, nil
}

