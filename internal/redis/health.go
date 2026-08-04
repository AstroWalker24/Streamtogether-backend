package redis

import (
	"context"
	"fmt"
)

// Health pings Redis and returns nil if the connection is healthy.
func (r *Redis) Health(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to ping redis: %w", err)
	}
	return nil
}
