// Package domain contains read models owned by the Search domain.
package domain

import "github.com/google/uuid"

// SearchResult is the user-discovery projection returned by Search.
type SearchResult struct {
	UserID      uuid.UUID
	Username    string
	DisplayName *string
	AvatarURL   *string
}
