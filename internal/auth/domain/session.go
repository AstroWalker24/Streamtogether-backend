package domain

import (
	"time"

	"github.com/google/uuid"
)

// Session is the server-side revocation anchor for stateless JWTs.
// It binds a user, a device, and a bounded time window.
// Termination uses Revoked rather than a deleted_at column.
type Session struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	DeviceID     uuid.UUID
	IPAddress    string // stored as INET; held as text in the domain layer
	UserAgent    string
	CreatedAt    time.Time
	LastActiveAt time.Time
	ExpiresAt    time.Time
	Revoked      bool
	RevokedAt    *time.Time
	RememberMe   bool
}

// IsActive reports whether the session is neither revoked nor expired.
func (s *Session) IsActive() bool {
	return !s.Revoked && time.Now().Before(s.ExpiresAt)
}
