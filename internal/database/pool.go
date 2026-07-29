package database

import (
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
)

func buildPoolConfig(cfg config.DatabaseConfig) (*pgxpool.Config, error) {
    poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
    if err != nil {
        return nil, fmt.Errorf("failed to parse postgres DSN: %w", err)
    }

    poolCfg.MaxConns = int32(cfg.MaxOpenConns)
    poolCfg.MinConns = int32(cfg.MaxIdleConns)
    poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime

    return poolCfg, nil
}
