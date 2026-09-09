// Package domain contains the pure business entities of the profile domain.
// No external framework, database, or transport dependencies are permitted here.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Profile is the social-facing identity of an application user.
// It is separate from authentication credentials, which are owned by auth.User.
// Every Profile belongs to exactly one User; the relationship is 1:1 and immutable.
type Profile struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	DisplayName *string
	Bio         *string
	AvatarURL   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
