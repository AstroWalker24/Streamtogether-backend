package database

import (
	"context"
    "fmt"
    "sync"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)


const (
    maxRetries     = 5
    retryBaseDelay = time.Second
    retryMaxDelay  = 30 * time.Second
    pingTimeout    = 5 * time.Second
)

type Database struct {
    pool      *pgxpool.Pool
    log       logger.Logger
    closeOnce sync.Once
}


func New(ctx context.Context, cfg config.DatabaseConfig, log logger.Logger) (*Database, error) {
    log.Info("connecting to postgres",
        logger.String("host", cfg.Host),
        logger.Int("port", cfg.Port),
        logger.String("database", cfg.Database),
    )

    poolCfg, err := buildPoolConfig(cfg)
    if err != nil {
        return nil, err
    }

    pool, err := connectWithRetry(ctx, poolCfg, log)
    if err != nil {
        return nil, err
    }

    log.Info("postgres connected",
        logger.String("host", cfg.Host),
        logger.Int("port", cfg.Port),
    )

    return &Database{
        pool: pool,
        log:  log,
    }, nil
}

func (db *Database) Pool() *pgxpool.Pool {
    return db.pool
}

func (db *Database) Close() {
    db.closeOnce.Do(func() {
        db.log.Info("closing postgres pool")
        db.pool.Close()
        db.log.Info("postgres pool closed")
    })
}

func connectWithRetry(ctx context.Context, cfg *pgxpool.Config, log logger.Logger) (*pgxpool.Pool, error) {
    var lastErr error
    delay := retryBaseDelay

    for attempt := 1; attempt <= maxRetries; attempt++ {
        pool, err := pgxpool.NewWithConfig(ctx, cfg)
        if err != nil {
            lastErr = fmt.Errorf("failed to create postgres pool: %w", err)
        } else {
            pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
            err = pool.Ping(pingCtx)
            cancel()

            if err == nil {
                return pool, nil
            }
            pool.Close()
            lastErr = fmt.Errorf("failed to ping postgres: %w", err)
        }

        if attempt == maxRetries {
            break
        }

        log.Warn("postgres connection attempt failed, retrying",
            logger.Int("attempt", attempt),
            logger.Int("max_retries", maxRetries),
            logger.Err(lastErr),
            logger.Duration("retry_in", delay),
        )

        select {
        case <-ctx.Done():
            return nil, fmt.Errorf("postgres connection cancelled: %w", ctx.Err())
        case <-time.After(delay):
        }

        delay *= 2
        if delay > retryMaxDelay {
            delay = retryMaxDelay
        }
    }

    return nil, fmt.Errorf("postgres: all %d connection attempts failed: %w", maxRetries, lastErr)
}