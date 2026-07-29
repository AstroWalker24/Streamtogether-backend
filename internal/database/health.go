package database

import (
	"context"
	"fmt"
	"time"
)

const healthPingTimeout = 3 * time.Second


func (db *Database) Health(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, healthPingTimeout)
	defer cancel()

	if err := db.pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("postgres health check failed: %w", err)
	}

	return nil
}
