// Package oauth defines the provider-agnostic foundation for OAuth 2.0 / OIDC
// authentication. Concrete provider adapters (Google, GitHub, Discord) satisfy
// the Provider interface and live outside this package.
package oauth

import "context"

// Provider is the contract every OAuth / OIDC adapter must satisfy.
type Provider interface {
	// Name returns the canonical lowercase identifier (e.g. "google", "discord").
	Name() string

	// BuildAuthorizationURL constructs the provider authorization URL embedding
	// the given state token for CSRF protection.
	BuildAuthorizationURL(state string) string

	// ExchangeCode exchanges the authorization code returned by the provider
	// for a verified ProviderIdentity. redirectURI must match the value used
	// when building the authorization URL.
	ExchangeCode(ctx context.Context, code, redirectURI string) (ProviderIdentity, error)
}

// ProviderIdentity is the normalized user identity returned by a successful
// OAuth exchange. It contains only the fields required by the authentication
// domain. Provider access tokens and refresh tokens are deliberately excluded.
type ProviderIdentity struct {
	// Provider is the canonical name of the issuing provider.
	Provider string

	// ProviderUserID is the stable, unique identifier within the provider's
	// namespace. Does not change when the user updates their email or display name.
	ProviderUserID string

	// Email is the email address reported by the provider.
	// May be empty when the requested scopes do not include email.
	Email string

	// EmailVerified indicates whether the provider has confirmed the email address.
	EmailVerified bool

	// DisplayName is the user's preferred name from the provider. May be empty.
	// Never used as a unique identifier.
	DisplayName string

	// AvatarURL is the URL of the user's profile picture. May be empty.
	AvatarURL string
}

// ProviderConfig holds the standard OAuth 2.0 configuration shared by most
// providers. Concrete adapters embed or receive this to avoid field repetition.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
	AuthURL      string // authorization endpoint
	TokenURL     string // token exchange endpoint
}
