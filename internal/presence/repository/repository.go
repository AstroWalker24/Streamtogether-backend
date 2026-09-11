// Package repository provides PostgreSQL and Redis persistence contracts and
// implementations for the Presence domain.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/presence/domain"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// DurablePresenceRepository defines PostgreSQL persistence for user presences,
// specifically durable historical last_seen_at timestamps.
type DurablePresenceRepository interface {
	// FindByUserID retrieves the durable presence record for a user.
	// Returns repo.ErrNotFound when no record exists.
	FindByUserID(ctx context.Context, userID uuid.UUID, opts ...repo.Option) (*domain.DurablePresence, error)

	// UpsertLastSeen records or updates last_seen_at for a user using an idempotent UPSERT.
	UpsertLastSeen(ctx context.Context, userID uuid.UUID, lastSeenAt time.Time, opts ...repo.Option) error

	// FindByUserIDs batch retrieves durable presence records for a list of user IDs.
	// Returns a map keyed by user ID. Missing users are simply omitted from the map.
	FindByUserIDs(ctx context.Context, userIDs []uuid.UUID, opts ...repo.Option) (map[uuid.UUID]*domain.DurablePresence, error)
}

// EphemeralPresenceRepository defines Redis operations for active presence connections,
// lease tracking, aggregate user presence, and flapping grace timers.
type EphemeralPresenceRepository interface {
	// RegisterConnection atomically registers an active connection, sets lease score,
	// writes connection metadata, aborts any active grace period, and recomputes aggregate presence.
	RegisterConnection(ctx context.Context, conn *domain.PresenceConnection, leaseDuration time.Duration) error

	// RecordHeartbeat refreshes the lease expiration for an active connection.
	// Returns presence.errors.CodeConnectionNotFound (or repo.ErrNotFound) if the connection does not exist.
	RecordHeartbeat(ctx context.Context, userID, connectionID uuid.UUID, leaseDuration time.Duration) error

	// RemoveConnection removes an active connection, prunes expired leases,
	// updates aggregate presence, and starts the 10-second grace period if 0 connections remain.
	// Returns the remaining active connection count.
	RemoveConnection(ctx context.Context, userID, connectionID uuid.UUID, graceDuration time.Duration) (int, error)

	// GetAggregatePresence retrieves the pre-computed aggregate presence for a single user.
	// Returns (nil, nil) if the user has no active presence keys in Redis.
	GetAggregatePresence(ctx context.Context, userID uuid.UUID) (*domain.UserPresence, error)

	// GetAggregatePresenceBatch retrieves pre-computed aggregate presence for multiple users
	// via a single pipelined round-trip. Missing keys return nil entries in the resulting map.
	GetAggregatePresenceBatch(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]*domain.UserPresence, error)

	// SetManualPreference sets an explicit manual presence override ("none", "away") and re-evaluates presence.
	SetManualPreference(ctx context.Context, userID uuid.UUID, pref domain.PresencePreference) error

	// GetConnectionMetadata retrieves metadata for a specific active connection.
	GetConnectionMetadata(ctx context.Context, userID, connectionID uuid.UUID) (*domain.PresenceConnection, error)

	// ListUserConnections retrieves all active connection metadata for a user.
	ListUserConnections(ctx context.Context, userID uuid.UUID) ([]*domain.PresenceConnection, error)

	// IsInGracePeriod reports whether a user currently has an active disconnect grace timer.
	IsInGracePeriod(ctx context.Context, userID uuid.UUID) (bool, error)

	// TrackNodeConnection registers a connection under a gateway cluster node's set.
	TrackNodeConnection(ctx context.Context, nodeID string, userID, connectionID uuid.UUID) error

	// UntrackNodeConnection removes a connection from a gateway cluster node's set.
	UntrackNodeConnection(ctx context.Context, nodeID string, userID, connectionID uuid.UUID) error

	// ListClusterNodeConnections lists all connection identifiers registered to a cluster node.
	ListClusterNodeConnections(ctx context.Context, nodeID string) ([]string, error)
}
