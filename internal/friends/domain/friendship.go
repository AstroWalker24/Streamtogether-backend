// Package domain contains the pure business entities of the Friends domain.
// No external framework, database, or transport dependencies are permitted here.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Friendship is an established, mutual relationship between two users.
// Its user IDs are stored as an immutable canonical pair.
type Friendship struct {
	ID           uuid.UUID
	FirstUserID  uuid.UUID
	SecondUserID uuid.UUID
	CreatedAt    time.Time
}
