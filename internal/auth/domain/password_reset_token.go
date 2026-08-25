package domain

import (
	"time"

	"github.com/google/uuid"
)

// PasswordResetToken is a time-bounded, single-use grant that allows the bearer
// to set a new password. Only the SHA-256 hash is stored; the plaintext is
// transmitted via email and never persisted.
type PasswordResetToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// IsActive reports whether the token is unconsumed and not yet expired.
func (t *PasswordResetToken) IsActive() bool {
	return t.UsedAt == nil && time.Now().Before(t.ExpiresAt)
}
