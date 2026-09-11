// Package dto contains the request and response data-transfer objects for the
// Search HTTP API.
package dto

// SearchQuery captures search query parameters.
type SearchQuery struct {
	Query    string `query:"q" validate:"required"`
	Page     int    `query:"page" validate:"omitempty,min=1"`
	PageSize int    `query:"page_size" validate:"omitempty,min=1,max=100"`
}

// SearchUserResponse is the public representation of a search result.
type SearchUserResponse struct {
	UserID      string  `json:"user_id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}
