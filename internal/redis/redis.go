package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

const (
	maxConnAttempts = 5
	retryBaseDelay  = 500 * time.Millisecond
	retryMaxDelay   = 15 * time.Second
	pingTimeout     = 5 * time.Second
)

// Redis wraps a go-redis client with lifecycle management and generic operations.
// It is safe for concurrent use across the entire application.
// Access the underlying client via Client() for advanced features such as
// pipelines, transactions, pub/sub, Lua scripts, and streams.
type Redis struct {
	client    *goredis.Client
	log       logger.Logger
	closeOnce sync.Once
}

// New creates a Redis instance, pings the server with exponential backoff retries,
// and fails if Redis remains unavailable after all attempts.
func New(ctx context.Context, cfg *config.Config, log logger.Logger, opts ...Option) (*Redis, error) {
	r := &Redis{
		client: goredis.NewClient(buildOptions(cfg.Redis)),
		log:    log,
	}

	for _, opt := range opts {
		opt(r)
	}

	log.Info("connecting to redis",
		logger.String("host", cfg.Redis.Host),
		logger.Int("port", cfg.Redis.Port),
		logger.Int("db", cfg.Redis.DB),
	)

	if err := connectWithRetry(ctx, r, log); err != nil {
		_ = r.client.Close()
		return nil, err
	}

	log.Info("redis connected",
		logger.String("addr", cfg.Redis.Address()),
	)

	return r, nil
}

// Client returns the underlying go-redis client for advanced usage such as
// pipelines, transactions, pub/sub, Lua scripts, streams, Sentinel, and Cluster mode.
func (r *Redis) Client() *goredis.Client {
	return r.client
}

// Close gracefully closes the Redis client. Safe to call multiple times.
func (r *Redis) Close() error {
	var err error
	r.closeOnce.Do(func() {
		r.log.Info("closing redis client")
		err = r.client.Close()
		if err != nil {
			r.log.Error("redis client close error", logger.Err(err))
		} else {
			r.log.Info("redis client closed")
		}
	})
	return err
}

// Get retrieves the string value of key.
// Returns goredis.Nil if the key does not exist.
func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("failed to execute GET %q: %w", key, err)
	}
	return val, nil
}

// Set stores value for key with the given TTL. A zero TTL means no expiry.
func (r *Redis) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("failed to execute SET %q: %w", key, err)
	}
	return nil
}

// Delete removes one or more keys and returns the number of keys deleted.
func (r *Redis) Delete(ctx context.Context, keys ...string) (int64, error) {
	n, err := r.client.Del(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to execute DEL: %w", err)
	}
	return n, nil
}

// Exists reports how many of the given keys exist in Redis.
func (r *Redis) Exists(ctx context.Context, keys ...string) (int64, error) {
	n, err := r.client.Exists(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to execute EXISTS: %w", err)
	}
	return n, nil
}

// Expire sets a TTL on key. Returns true if the timeout was applied.
func (r *Redis) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := r.client.Expire(ctx, key, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to execute EXPIRE %q: %w", key, err)
	}
	return ok, nil
}

// TTL returns the remaining time-to-live of key.
// Returns -1 if the key has no expiry, -2 if the key does not exist.
func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to execute TTL %q: %w", key, err)
	}
	return d, nil
}

// Increment atomically increments the integer stored at key by one.
func (r *Redis) Increment(ctx context.Context, key string) (int64, error) {
	n, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to execute INCR %q: %w", key, err)
	}
	return n, nil
}

// Decrement atomically decrements the integer stored at key by one.
func (r *Redis) Decrement(ctx context.Context, key string) (int64, error) {
	n, err := r.client.Decr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to execute DECR %q: %w", key, err)
	}
	return n, nil
}

// FlushDB removes all keys from the currently selected database.
// Use with caution; intended for testing and administrative tasks only.
func (r *Redis) FlushDB(ctx context.Context) error {
	if err := r.client.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("failed to execute FLUSHDB: %w", err)
	}
	return nil
}

func buildOptions(cfg config.RedisConfig) *goredis.Options {
	return &goredis.Options{
		Addr:         cfg.Address(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolTimeout:  cfg.PoolTimeout,
		MaxRetries:   cfg.MaxRetries,
	}
}

func connectWithRetry(ctx context.Context, r *Redis, log logger.Logger) error {
	var lastErr error
	delay := retryBaseDelay

	for attempt := 1; attempt <= maxConnAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		_, err := r.client.Ping(pingCtx).Result()
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err

		if attempt == maxConnAttempts {
			break
		}

		log.Warn("redis connection attempt failed, retrying",
			logger.Int("attempt", attempt),
			logger.Int("max_attempts", maxConnAttempts),
			logger.Duration("retry_in", delay),
			logger.Err(err),
		)

		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to connect to redis: context cancelled: %w", ctx.Err())
		case <-time.After(delay):
		}

		delay = min(delay*2, retryMaxDelay)
	}

	return fmt.Errorf("failed to connect to redis after %d attempts: %w", maxConnAttempts, lastErr)
}
