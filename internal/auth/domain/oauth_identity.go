package domain

import (
	"time"

	"github.com/google/uuid"
)

// OAuthIdentity links an external OAuth/OIDC provider identity to a local
// User account. The (Provider, ProviderUserID) pair is globally unique.
type OAuthIdentity struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Provider       string
	ProviderUserID string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
