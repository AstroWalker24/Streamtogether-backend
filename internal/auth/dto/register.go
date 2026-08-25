// Package dto contains the data transfer objects for the authentication domain.
// DTOs cross layer boundaries (HTTP ↔ Service) and must never reference domain entities.
package dto

import "strings"

// RegisterRequest is the inbound DTO for the user registration flow.
type RegisterRequest struct {
	Email    string `json:"email"    validate:"required,email,max=254"`
	Username string `json:"username" validate:"required,min=3,max=30"`
	Password string `json:"password" validate:"required,min=8"`
}

// Normalize lowercases and trims email and username before validation runs.
func (r *RegisterRequest) Normalize() {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	r.Username = strings.ToLower(strings.TrimSpace(r.Username))
}

// UserResponse is the outbound representation of a user's public identity.
// It never carries sensitive fields such as PasswordHash.
type UserResponse struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Username      string   `json:"username"`
	EmailVerified bool     `json:"email_verified"`
	Roles         []string `json:"roles"`
	CreatedAt     string   `json:"created_at"` // RFC 3339
}
