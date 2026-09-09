// Package dto contains the request and response data-transfer objects for the
// profile HTTP API.
package dto

// UpdateProfileRequest is the inbound body for PATCH /me/profile.
//
// All fields are optional pointers. A nil value (JSON null or absent field)
// clears the corresponding profile field; a non-nil string value sets it.
// Clients that want to retain an existing value must include it in the body.
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	AvatarURL   *string `json:"avatar_url"`
}

// ProfileResponse is the outbound representation of a user's profile.
type ProfileResponse struct {
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	AvatarURL   *string `json:"avatar_url"`
	UpdatedAt   string  `json:"updated_at"` // RFC 3339
}
