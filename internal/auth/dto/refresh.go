package dto

// RefreshRequest is the inbound DTO for the token refresh flow.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
