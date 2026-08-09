package domain

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken is the server-side record for every refresh token issued.
// It enables validation, single-use rotation, replay detection, and revocation.
// Only the SHA-256 hash of the plaintext token is ever stored.
type RefreshToken struct {
	ID           uuid.UUID
	SessionID    uuid.UUID
	UserID       uuid.UUID // denormalized for efficient per-user revocation
	DeviceID     uuid.UUID // denormalized for efficient per-device revocation
	TokenHash    string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
	Revoked      bool
	RevokedAt    *time.Time
	ReplacedByID *uuid.UUID // successor token in the rotation chain
}

// IsActive reports whether the token may still be used for a refresh.
func (rt *RefreshToken) IsActive() bool {
	return rt.ConsumedAt == nil && !rt.Revoked && time.Now().Before(rt.ExpiresAt)
}

// IsConsumed reports whether the token has already been used in a rotation.
// A consumed token presented again signals a replay attack.
func (rt *RefreshToken) IsConsumed() bool {
	return rt.ConsumedAt != nil
}
