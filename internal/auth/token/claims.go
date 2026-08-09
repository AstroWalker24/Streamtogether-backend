// Package token defines the authentication-specific JWT claims for the
// Streamtogether access token. It sits between the authentication service
// and the generic JWT Foundation:
//
//	Auth Service → token.Build → jwt.Manager.Issue → signed token string
//	Auth Service ← token.FromJWTClaims ← jwt.Manager.VerifyAndValidate ← token string
//
// This package does not sign, verify, or cryptographically validate tokens.
// Those responsibilities belong entirely to the JWT Foundation
// (internal/security/jwt). Claims produced or consumed here are only safe to
// use after the JWT Foundation has confirmed the token's signature and
// registered claims (exp, iss, aud).
//
// Dependency rule: this package may import internal/security/jwt but must
// never import the third-party JWT library (github.com/golang-jwt/jwt/v5)
// directly.
package token

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	jwtpkg "github.com/AstroWalker24/Streamtogether-backend/internal/security/jwt"
)

// Custom claim key constants.
// Unexported: no package outside this one should reference raw JWT key strings.
const (
	// claimKeySID is the JWT custom claim key for the server-side session ID.
	// "sid" is compact and semantically unambiguous in the JWT ecosystem.
	claimKeySID = "sid"

	// claimKeyRoles is the JWT custom claim key for the role names array.
	claimKeyRoles = "roles"
)

// AccessTokenClaims is the strongly-typed, validated result of access token
// parsing. The authorization middleware uses it to populate the request context
// with the authenticated principal's identity.
//
// All UUID fields are required; a token missing any of them must be rejected
// by FromJWTClaims.
//
// Security: these claims are safe to consume ONLY after
// jwt.Manager.VerifyAndValidate has confirmed the signature and all standard
// registered claims. Never use claims sourced from jwt.Manager.ParseUnverified
// for authorization decisions.
type AccessTokenClaims struct {
	// UserID is the authenticated user's unique identifier.
	// Sourced from the standard "sub" claim.
	//
	// Design: "sub" serves directly as the user ID; no duplicate "user_id"
	// custom claim is added, keeping the token payload minimal per INV-17.
	UserID uuid.UUID

	// SessionID identifies the server-side session this token was issued under.
	// Enables WebSocket authentication and optional per-request session-validity
	// checks without a database round-trip on the hot path.
	// Sourced from the "sid" custom claim.
	SessionID uuid.UUID

	// Roles lists the role names the user held at token issuance time.
	// Used by the authorization layer for role-based access control.
	// Sourced from the "roles" custom claim.
	// May be nil when the token carries no roles.
	Roles []string

	// IssuedAt is the time the token was issued. Sourced from "iat".
	IssuedAt time.Time

	// ExpiresAt is the time after which the token must be rejected. Sourced from "exp".
	ExpiresAt time.Time

	// TokenID is the unique identifier for this specific token instance.
	// Supports revocation via a JTI deny-list. Sourced from "jti".
	TokenID uuid.UUID
}

// BuildInput carries all parameters required to construct a signed access token
// payload. Every UUID field and both time fields are required; zero values are
// rejected by Build.
//
// The caller is responsible for:
//   - Generating TokenID using a cryptographically secure source (e.g. uuid.New()).
//   - Verifying that the user is active and the session is valid before calling Build.
//   - Sourcing Issuer and Audience from the application JWTConfig.
type BuildInput struct {
	// UserID is serialized as the "sub" claim.
	UserID uuid.UUID

	// SessionID is serialized as the "sid" custom claim.
	SessionID uuid.UUID

	// Roles are serialized as the "roles" custom claim.
	// nil and empty slices are both accepted; both produce an empty JSON array.
	Roles []string

	// TokenID is serialized as the "jti" claim. Generate with uuid.New().
	TokenID uuid.UUID

	// IssuedAt is serialized as the "iat" claim.
	IssuedAt time.Time

	// ExpiresAt is serialized as the "exp" claim. Must be strictly after IssuedAt.
	ExpiresAt time.Time

	// Issuer is serialized as the "iss" claim. Sourced from the application JWTConfig.
	Issuer string

	// Audience is serialized as the "aud" claim. Sourced from the application JWTConfig.
	Audience []string
}

// Build constructs a jwtpkg.Claims value ready to be passed to
// jwt.Manager.Issue. It performs structural validation of BuildInput and
// returns ErrInvalidAuthClaims (wrapping a descriptive message) for any
// missing or invalid field.
//
// Build does not authenticate the user, query the database, sign anything,
// or generate cryptographic material. Those belong to the caller.
func Build(in BuildInput) (jwtpkg.Claims, error) {
	if err := validateBuildInput(in); err != nil {
		return jwtpkg.Claims{}, err
	}

	// Normalise nil to an empty slice so the JWT payload always has a
	// well-formed "roles" array rather than a JSON null.
	roles := in.Roles
	if roles == nil {
		roles = []string{}
	}

	return jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject:   in.UserID.String(),
			Issuer:    in.Issuer,
			Audience:  in.Audience,
			IssuedAt:  in.IssuedAt.UTC(),
			ExpiresAt: in.ExpiresAt.UTC(),
			JWTID:     in.TokenID.String(),
		},
		Custom: map[string]any{
			claimKeySID:   in.SessionID.String(),
			claimKeyRoles: roles,
		},
	}, nil
}

// FromJWTClaims converts a validated jwtpkg.Claims value into AccessTokenClaims.
// It must only be called with claims returned by jwt.Manager.VerifyAndValidate.
// Calling it with unverified claims from jwt.Manager.ParseUnverified is a
// security violation: unverified claims must never drive authorization.
//
// Returns a specific sentinel error for each category of structural failure.
func FromJWTClaims(claims jwtpkg.Claims) (AccessTokenClaims, error) {
	// Parse UserID from "sub".
	if claims.Subject == "" {
		return AccessTokenClaims{}, ErrMissingSubject
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("%w: %v", ErrInvalidUserID, err)
	}

	// Parse TokenID from "jti".
	if claims.JWTID == "" {
		return AccessTokenClaims{}, ErrMissingTokenID
	}
	tokenID, err := uuid.Parse(claims.JWTID)
	if err != nil {
		return AccessTokenClaims{}, fmt.Errorf("%w: %v", ErrInvalidTokenID, err)
	}

	// Parse SessionID from "sid".
	sessionID, err := extractSessionID(claims.Custom)
	if err != nil {
		return AccessTokenClaims{}, err
	}

	// Parse Roles from "roles". Absent key is accepted as nil roles.
	roles, err := extractRoles(claims.Custom)
	if err != nil {
		return AccessTokenClaims{}, err
	}

	return AccessTokenClaims{
		UserID:    userID,
		SessionID: sessionID,
		Roles:     roles,
		IssuedAt:  claims.IssuedAt,
		ExpiresAt: claims.ExpiresAt,
		TokenID:   tokenID,
	}, nil
}

// validateBuildInput checks that all required BuildInput fields are non-zero
// and that the expiry is strictly after the issuance time.
func validateBuildInput(in BuildInput) error {
	if in.UserID == uuid.Nil {
		return fmt.Errorf("%w: user ID must not be the nil UUID", ErrInvalidAuthClaims)
	}
	if in.SessionID == uuid.Nil {
		return fmt.Errorf("%w: session ID must not be the nil UUID", ErrInvalidAuthClaims)
	}
	if in.TokenID == uuid.Nil {
		return fmt.Errorf("%w: token ID must not be the nil UUID", ErrInvalidAuthClaims)
	}
	if in.IssuedAt.IsZero() {
		return fmt.Errorf("%w: issued-at must not be zero", ErrInvalidAuthClaims)
	}
	if in.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: expires-at must not be zero", ErrInvalidAuthClaims)
	}
	if !in.ExpiresAt.After(in.IssuedAt) {
		return fmt.Errorf("%w: expires-at must be strictly after issued-at", ErrInvalidAuthClaims)
	}
	return nil
}

// extractSessionID reads the "sid" claim from the custom claims map.
func extractSessionID(custom map[string]any) (uuid.UUID, error) {
	raw, ok := custom[claimKeySID]
	if !ok || raw == nil {
		return uuid.UUID{}, ErrMissingSessionID
	}
	s, ok := raw.(string)
	if !ok {
		return uuid.UUID{}, fmt.Errorf("%w: sid must be a string", ErrInvalidClaimType)
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("%w: %v", ErrInvalidSessionID, err)
	}
	return id, nil
}

// extractRoles reads the "roles" claim from the custom claims map.
// A missing or nil key is treated as an empty role set rather than an error,
// since roles are determined at issuance time and a zero-role token is valid.
//
// After a JWT round-trip the roles array comes back as []any (JSON decode);
// in direct calls (e.g. tests without serialisation) it may arrive as []string.
// Both forms are handled.
func extractRoles(custom map[string]any) ([]string, error) {
	raw, ok := custom[claimKeyRoles]
	if !ok || raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, r := range v {
			s, ok := r.(string)
			if !ok {
				return nil, fmt.Errorf("%w: roles element is not a string", ErrInvalidClaimType)
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("%w: roles must be an array", ErrInvalidClaimType)
	}
}
