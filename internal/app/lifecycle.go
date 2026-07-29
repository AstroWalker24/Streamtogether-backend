package app

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)


func (a *App) Run() error {
    serverErr := make(chan error, 1)

    go func() {
        a.log.Info("http server starting", logger.String("addr", a.cfg.App.Address()))
        serverErr <- a.Start()
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

    select {
    case err := <-serverErr:
        return fmt.Errorf("app: server error: %w", err)
    case sig := <-quit:
        a.log.Info("shutdown signal received", logger.String("signal", sig.String()))
    }

    return a.Shutdown()
}


func (a *App) Start() error {
    return a.server.Start()
}


func (a *App) Shutdown() error {
    a.log.Info("shutting down application")

    ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Server.ShutdownTimeout)
    defer cancel()

    if err := a.server.Shutdown(ctx); err != nil {
        a.log.Error("http server shutdown error", logger.Err(err))
    }

    if err := a.redis.Close(); err != nil {
        a.log.Error("redis close error", logger.Err(err))
    } else {
        a.log.Info("redis connection closed")
    }

    a.db.Close() // logs internally: "closing postgres pool" + "postgres pool closed"

    a.log.Info("application stopped")
    return nil
}