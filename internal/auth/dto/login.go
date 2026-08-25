package dto

import "strings"

// LoginRequest is the inbound DTO for the user login flow.
type LoginRequest struct {
	Identifier        string `json:"identifier"         validate:"required"`
	Password          string `json:"password"           validate:"required"`
	DeviceFingerprint string `json:"device_fingerprint" validate:"required"`
	DeviceName        string `json:"device_name"`
	Platform          string `json:"platform"`
	Browser           string `json:"browser"`
	OS                string `json:"os"`
	RememberMe        bool   `json:"remember_me"`
}

// Normalize trims whitespace and lowercases an email-style identifier.
func (r *LoginRequest) Normalize() {
	r.Identifier = strings.TrimSpace(r.Identifier)
	if strings.ContainsRune(r.Identifier, '@') {
		r.Identifier = strings.ToLower(r.Identifier)
	}
}

// TokenPair groups the two tokens issued together after a successful login.
type TokenPair struct {
	AccessToken           string `json:"access_token"`
	AccessTokenExpiresAt  string `json:"access_token_expires_at"` // RFC 3339
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresAt string `json:"refresh_token_expires_at"` // RFC 3339
	TokenType             string `json:"token_type"`
}

// DeviceResponse is the public representation of an authenticated device.
type DeviceResponse struct {
	ID            string `json:"id"`
	FriendlyName  string `json:"friendly_name"`
	Platform      string `json:"platform"`
	Browser       string `json:"browser"`
	OS            string `json:"os"`
	LastIPAddress string `json:"last_ip_address"`
	LastActiveAt  string `json:"last_active_at"` // RFC 3339
	Trusted       bool   `json:"trusted"`
}

// SessionResponse is metadata about the authenticated session.
type SessionResponse struct {
	ID           string         `json:"id"`
	Device       DeviceResponse `json:"device"`
	IPAddress    string         `json:"ip_address"`
	CreatedAt    string         `json:"created_at"`     // RFC 3339
	LastActiveAt string         `json:"last_active_at"` // RFC 3339
	ExpiresAt    string         `json:"expires_at"`     // RFC 3339
	RememberMe   bool           `json:"remember_me"`
	IsCurrent    bool           `json:"is_current"`
}

// AuthResponse is the primary response returned after a successful login.
type AuthResponse struct {
	TokenPair TokenPair       `json:"token"`
	User      UserResponse    `json:"user"`
	Session   SessionResponse `json:"session"`
}
