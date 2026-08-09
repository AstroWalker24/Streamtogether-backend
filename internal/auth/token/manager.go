package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	jwtpkg "github.com/AstroWalker24/Streamtogether-backend/internal/security/jwt"
)

// Clock is a source of the current time. The default is time.Now.
// Inject a deterministic alternative via NewWithClock for testing.
type Clock func() time.Time

// TokenManager is the application-facing interface for access token operations.
// Authentication Services and middleware depend on this interface, not on the
// concrete Manager or the JWT Foundation, so the underlying JWT infrastructure
// can be replaced without touching business logic.
type TokenManager interface {
	// CreateAccessToken builds and signs an access token for an authenticated
	// principal. The caller must have already verified the user's credentials
	// and must not invoke this before authentication succeeds.
	CreateAccessToken(in CreateAccessTokenInput) (AccessToken, error)

	// ValidateAccessToken verifies the token's signature and all standard
	// registered claims, then extracts the authenticated identity. A non-nil
	// error means the token must be rejected; do not use the returned claims
	// on error.
	ValidateAccessToken(tokenStr string) (AccessTokenClaims, error)
}

// Manager implements TokenManager by coordinating the Authentication Claims
// layer (internal/auth/token) and the JWT Foundation (internal/security/jwt).
//
// Manager is safe for concurrent use after construction. No mutable state is
// modified during token operations.
type Manager struct {
	jwt      *jwtpkg.Manager
	issuer   string
	audience []string
	expiry   time.Duration
	clock    Clock
}

// New constructs a Manager from a pre-built jwt.Manager and application config.
//
// The jwtManager and cfg must be derived from the same JWTConfig so that the
// issuer and audience used during token creation match those enforced during
// validation. Passing mismatched configurations is a programming error and will
// cause all tokens to fail validation.
func New(jwtManager *jwtpkg.Manager, cfg config.JWTConfig) (*Manager, error) {
	return NewWithClock(jwtManager, cfg, time.Now)
}

// NewWithClock is like New but injects a custom Clock. Use this in tests to
// produce deterministic IssuedAt and ExpiresAt timestamps in created tokens.
//
// The jwt.Manager has its own independent clock (set at jwt construction time)
// for signature and claim validation. Both clocks can differ; see
// jwtpkg.NewWithClock for the jwt.Manager clock.
func NewWithClock(jwtManager *jwtpkg.Manager, cfg config.JWTConfig, clock Clock) (*Manager, error) {
	if jwtManager == nil {
		return nil, errors.New("token.Manager: jwt manager must not be nil")
	}
	if clock == nil {
		return nil, errors.New("token.Manager: clock must not be nil")
	}
	if cfg.AccessTokenExpiry <= 0 {
		return nil, errors.New("token.Manager: access token expiry must be positive")
	}

	audience := make([]string, 0, 1)
	if cfg.Audience != "" {
		audience = append(audience, cfg.Audience)
	}

	return &Manager{
		jwt:      jwtManager,
		issuer:   cfg.Issuer,
		audience: audience,
		expiry:   cfg.AccessTokenExpiry,
		clock:    clock,
	}, nil
}

// CreateAccessTokenInput carries the caller-supplied parameters for token
// creation. All infrastructure concerns — JTI generation, issuance time,
// expiry calculation, issuer, and audience — are handled internally by Manager.
type CreateAccessTokenInput struct {
	// UserID is the authenticated user's unique identifier.
	UserID uuid.UUID

	// SessionID identifies the server-side session this token is bound to.
	SessionID uuid.UUID

	// Roles are the role names held by the user at issuance time.
	// nil and empty slices are both accepted.
	Roles []string
}

// AccessToken is the result of a successful CreateAccessToken call.
type AccessToken struct {
	// Token is the compact signed JWT string to be returned to the client.
	// This value is a bearer credential; never log or persist it.
	Token string

	// ExpiresAt is the absolute UTC time at which the token becomes invalid.
	ExpiresAt time.Time
}

// CreateAccessToken builds and signs an access token for the given principal.
// It generates a cryptographically secure JTI, records the current time as
// the issuance timestamp, and applies the configured access-token lifetime.
//
// CreateAccessToken makes no database or network calls. Ensuring the user is
// active and the session is valid is the caller's responsibility.
func (m *Manager) CreateAccessToken(in CreateAccessTokenInput) (AccessToken, error) {
	if in.UserID == uuid.Nil {
		return AccessToken{}, fmt.Errorf("%w: user ID must not be the nil UUID", ErrAccessTokenCreation)
	}
	if in.SessionID == uuid.Nil {
		return AccessToken{}, fmt.Errorf("%w: session ID must not be the nil UUID", ErrAccessTokenCreation)
	}

	now := m.clock().UTC()
	expiresAt := now.Add(m.expiry)

	// uuid.New uses crypto/rand internally; this satisfies the cryptographically
	// secure JTI requirement without re-implementing random generation here.
	tokenID := uuid.New()

	buildIn := BuildInput{
		UserID:    in.UserID,
		SessionID: in.SessionID,
		Roles:     in.Roles,
		TokenID:   tokenID,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
		Issuer:    m.issuer,
		Audience:  m.audience,
	}

	jwtClaims, err := Build(buildIn)
	if err != nil {
		// Build only fails for zero-valued required fields; all are set above.
		// Wrap and surface as a creation failure for the caller.
		return AccessToken{}, fmt.Errorf("%w: %v", ErrAccessTokenCreation, err)
	}

	tokenStr, err := m.jwt.Issue(jwtClaims)
	if err != nil {
		// Do not include the raw token or any claim values in the error.
		return AccessToken{}, fmt.Errorf("%w: signing failed", ErrAccessTokenCreation)
	}

	return AccessToken{
		Token:     tokenStr,
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateAccessToken verifies the token's cryptographic signature and all
// standard registered claims (exp, iss, aud), then extracts the authenticated
// identity as a strongly-typed value.
//
// A non-nil error means the token must be rejected; the returned
// AccessTokenClaims is the zero value and must not be used for authorization.
//
// ValidateAccessToken makes no database or network calls. Determining whether
// the user or session is still active is the caller's responsibility.
func (m *Manager) ValidateAccessToken(tokenStr string) (AccessTokenClaims, error) {
	jwtClaims, err := m.jwt.VerifyAndValidate(tokenStr)
	if err != nil {
		return AccessTokenClaims{}, m.mapValidationError(err)
	}

	claims, err := FromJWTClaims(jwtClaims)
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("%w: %v", ErrInvalidAuthClaims, err)
	}

	return claims, nil
}

// mapValidationError translates JWT Foundation errors into Token Manager
// sentinels so callers do not need to import the JWT Foundation for error
// checking. The original error message is preserved as context.
func (m *Manager) mapValidationError(err error) error {
	switch {
	case errors.Is(err, jwtpkg.ErrExpiredToken):
		return fmt.Errorf("%w: %v", ErrAccessTokenExpired, err)
	case errors.Is(err, jwtpkg.ErrMalformedToken):
		return fmt.Errorf("%w: %v", ErrAccessTokenMalformed, err)
	case errors.Is(err, jwtpkg.ErrInvalidSignature):
		return fmt.Errorf("%w: %v", ErrInvalidAccessTokenSignature, err)
	case errors.Is(err, jwtpkg.ErrUnsupportedAlgorithm):
		return fmt.Errorf("%w: %v", ErrUnsupportedTokenAlgorithm, err)
	default:
		return fmt.Errorf("%w: %v", ErrInvalidAccessToken, err)
	}
}
