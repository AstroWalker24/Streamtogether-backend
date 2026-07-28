package app

import (
"context"
    "fmt"

    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
    "github.com/AstroWalker24/Streamtogether-backend/internal/database"
    "github.com/AstroWalker24/Streamtogether-backend/internal/handlers"
    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
    "github.com/AstroWalker24/Streamtogether-backend/internal/routes"
    "github.com/AstroWalker24/Streamtogether-backend/internal/server"
)


func New() (*App, error) {
    // 1. Configuration
    cfg, err := config.Load()
    if err != nil {
        return nil, fmt.Errorf("app: load config: %w", err)
    }

    // 2. Logger — pretty output in development, JSON in all other environments.
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
    db, err := database.NewPostgres(ctx, cfg.Database)
    if err != nil {
        return nil, fmt.Errorf("app: init postgres: %w", err)
    }
    log.Info("postgres connected",
        logger.String("host", cfg.Database.Host),
        logger.Int("port", cfg.Database.Port),
    )

    // 4. Redis
    redisClient, err := database.NewRedis(ctx, cfg.Redis)
    if err != nil {
        db.Close()
        return nil, fmt.Errorf("app: init redis: %w", err)
    }
    log.Info("redis connected", logger.String("addr", cfg.Redis.Address()))

    // 5. Routes + handlers (no I/O; cannot fail)
    healthHandler := handlers.NewHealthHandler()
    handler := routes.Register(healthHandler)

    // 6. HTTP Server (no I/O at construction time; cannot fail)
    srv := server.New(cfg.Server, cfg.App, handler)
    log.Info("http server configured", logger.String("addr", cfg.App.Address()))

    return &App{
        cfg:    cfg,
        log:    log,
        db:     db,
        redis:  redisClient,
        server: srv,
    }, nil
}