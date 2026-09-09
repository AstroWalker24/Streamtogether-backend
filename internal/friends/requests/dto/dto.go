// Package dto contains the request and response data-transfer objects for the
// Friend Requests HTTP API.
package dto

// SendFriendRequest is the inbound body for POST /me/friend-requests.
type SendFriendRequest struct {
	UserID string `json:"user_id" validate:"required,uuid4"`
}

// FriendRequestResponse is the public representation of a Friend Request.
type FriendRequestResponse struct {
	ID              string  `json:"id"`
	RequesterUserID string  `json:"requester_user_id"`
	RecipientUserID string  `json:"recipient_user_id"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	RespondedAt     *string `json:"responded_at,omitempty"`
}

// FriendRequestsQuery captures page-based request-list query parameters.
type FriendRequestsQuery struct {
	Page     int `query:"page" validate:"omitempty,min=1"`
	PageSize int `query:"page_size" validate:"omitempty,min=1,max=100"`
}
