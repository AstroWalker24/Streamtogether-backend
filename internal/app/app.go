package app

import (
	"log/slog"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)


type App struct {
	cfg *config.Config
	log *slog.Logger
	db *pgxpool.Pool
	redis *redis.Client
	server *server.Server
}

