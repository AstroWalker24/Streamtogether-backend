package redis

import goredis "github.com/redis/go-redis/v9"

// Option is a functional option for configuring a Redis instance.
type Option func(*Redis)

// WithClient replaces the underlying go-redis client with the provided one.
// Intended for testing to inject a pre-configured or miniredis-backed client.
func WithClient(c *goredis.Client) Option {
	return func(r *Redis) {
		r.client = c
	}
}
