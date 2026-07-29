package app

import (
    "github.com/redis/go-redis/v9"

    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
    "github.com/AstroWalker24/Streamtogether-backend/internal/database"
    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
    "github.com/AstroWalker24/Streamtogether-backend/internal/server"
)

type App struct {
    cfg    *config.Config
    log    logger.Logger
    db     *database.Database
    redis  *redis.Client
    server *server.Server
}

