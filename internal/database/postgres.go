package database

import (
	"context"
	"fmt"
	"time"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)


func NewPostgres(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pingError := pool.Ping(pingCtx)
	if pingError != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", pingError)
	}
	return pool, nil
}