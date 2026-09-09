// Package dto contains the request and response data-transfer objects for the
// Friends HTTP API.
package dto

// EstablishFriendshipRequest is the inbound body for POST /me/friends.
type EstablishFriendshipRequest struct {
	UserID string `json:"user_id" validate:"required,uuid4"`
}

// FriendshipResponse is the outbound representation of a friendship from the
// authenticated user's perspective. UserID is always the other endpoint.
type FriendshipResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	CreatedAt string `json:"created_at"` // RFC 3339
}

// FriendshipStatusResponse is the outbound representation of a friendship check.
type FriendshipStatusResponse struct {
	AreFriends bool `json:"are_friends"`
}

// FriendsQuery captures page-based friend-list query parameters.
type FriendsQuery struct {
	Page     int `query:"page" validate:"omitempty,min=1"`
	PageSize int `query:"page_size" validate:"omitempty,min=1,max=100"`
}
