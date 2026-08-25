// Package google provides a Google OAuth 2.0 / OIDC adapter that satisfies the
// oauth.Provider interface. It uses the OIDC discovery document to obtain
// endpoints and the google JWKS to verify ID tokens.
package google

import (
	"context"
	"fmt"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	autherrors "github.com/AstroWalker24/Streamtogether-backend/internal/auth/errors"
	oauthpkg "github.com/AstroWalker24/Streamtogether-backend/internal/auth/oauth"
)

const googleIssuer = "https://accounts.google.com"

// Provider implements oauth.Provider for Google OAuth 2.0 / OIDC.
type Provider struct {
	cfg      oauth2.Config
	verifier *oidclib.IDTokenVerifier
}

// New creates a Google Provider by fetching the OIDC discovery document at
// construction time. The supplied ctx is used only for the initial HTTP request
// and is not retained by the returned Provider.
func New(ctx context.Context, clientID, clientSecret, redirectURI string, extraScopes []string) (*Provider, error) {
	oidcProvider, err := oidclib.NewProvider(ctx, googleIssuer)
	if err != nil {
		return nil, fmt.Errorf("google: OIDC discovery failed: %w", err)
	}

	scopes := []string{oidclib.ScopeOpenID, "email", "profile"}
	scopes = append(scopes, extraScopes...)

	cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       scopes,
	}

	verifier := oidcProvider.Verifier(&oidclib.Config{
		ClientID: clientID,
	})

	return &Provider{cfg: cfg, verifier: verifier}, nil
}

// Name returns the canonical provider name.
func (p *Provider) Name() string {
	return "google"
}

// BuildAuthorizationURL returns the Google consent page URL with the embedded
// state token for CSRF protection. access_type=online is used because the
// application does not need long-lived refresh tokens from Google.
func (p *Provider) BuildAuthorizationURL(state string) string {
	return p.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// ExchangeCode exchanges the authorization code for a verified Google identity.
// It verifies the OIDC ID token's signature, issuer, audience, and expiry using
// Google's published JWKS. The redirectURI parameter is accepted for interface
// compatibility but is not sent in the token request because the value was
// already embedded in the authorization URL.
func (p *Provider) ExchangeCode(ctx context.Context, code, _ string) (oauthpkg.ProviderIdentity, error) {
	oauth2Token, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return oauthpkg.ProviderIdentity{}, autherrors.NewOAuthProviderError(err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return oauthpkg.ProviderIdentity{}, autherrors.NewOAuthProviderError(
			fmt.Errorf("id_token missing from Google token response"),
		)
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return oauthpkg.ProviderIdentity{}, autherrors.NewOAuthIdentityInvalid(
			"ID token verification failed: " + err.Error(),
		)
	}

	var claims struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return oauthpkg.ProviderIdentity{}, autherrors.NewOAuthIdentityInvalid("failed to extract ID token claims")
	}

	if claims.Sub == "" {
		return oauthpkg.ProviderIdentity{}, autherrors.NewOAuthIdentityInvalid("provider user ID (sub) is empty")
	}

	return oauthpkg.ProviderIdentity{
		Provider:       p.Name(),
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		EmailVerified:  claims.EmailVerified,
		DisplayName:    claims.Name,
		AvatarURL:      claims.Picture,
	}, nil
}
