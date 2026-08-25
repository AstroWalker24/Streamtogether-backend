package domain

import (
    "time"

    "github.com/google/uuid"
)



// EmailVerificationToken is a time-bounded, single-use proof that a user
// controls their registered email address. Only the SHA-256 hash is stored.
type EmailVerificationToken struct {
    ID        uuid.UUID
    UserID    uuid.UUID
    TokenHash string
    ExpiresAt time.Time
    UsedAt    *time.Time
    CreatedAt time.Time
}

// IsActive reports whether the token is unconsumed and not yet expired.
func (t *EmailVerificationToken) IsActive() bool {
    return t.UsedAt == nil && time.Now().Before(t.ExpiresAt)
}



