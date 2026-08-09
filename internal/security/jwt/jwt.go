// Package jwt provides the technical mechanics of JSON Web Tokens for the
// application. It is the ONLY package that may import the third-party JWT
// library (github.com/golang-jwt/jwt/v5). All other packages interact with
// the types exported from this package exclusively.
//
// The package covers:
//   - Token creation and signing (Manager.Issue)
//   - Signature verification (Manager.Verify)
//   - Unverified claim parsing (Manager.ParseUnverified)
//   - Registered claim validation (Manager.ValidateClaims)
//   - Combined verify-and-validate (Manager.VerifyAndValidate)
//   - Cryptographically secure JTI generation (Manager.GenerateJTI)
//
// This package does NOT implement authentication business logic. It has no
// knowledge of users, sessions, roles, or permissions.
//
// Thread safety: Manager and all its internal components are safe for
// concurrent use after construction. No mutable state is modified during
// token operations.
package jwt

import (
	"fmt"
	"time"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// Clock is a function that returns the current time. The default is time.Now.
// Tests may inject a deterministic alternative via NewWithClock.
type Clock func() time.Time

// Manager is the single entry point for all JWT operations. It composes a
// Signer, Verifier, Validator, and Parser configured from the application's
// JWTConfig.
//
// Construct a Manager with New or NewWithClock. Do not copy a Manager after
// first use.
type Manager struct {
	cfg       Config
	signer    *Signer
	verifier  *Verifier
	validator *Validator
	parser    *Parser
	rng       *random.Generator
	clock     Clock
}

// New constructs a Manager from the application JWTConfig using the real
// system clock.
func New(cfg config.JWTConfig, rng *random.Generator) (*Manager, error) {
	return NewWithClock(cfg, rng, time.Now)
}

// NewWithClock is like New but accepts a Clock injection point. Use this in
// tests to produce deterministic token validation behaviour.
func NewWithClock(cfg config.JWTConfig, rng *random.Generator, clock Clock) (*Manager, error) {
	if rng == nil {
		return nil, fmt.Errorf("jwt.NewWithClock: random generator must not be nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("jwt.NewWithClock: clock must not be nil")
	}

	c, err := NewConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("jwt.NewWithClock: %w", err)
	}

	signer, err := newSigner(c)
	if err != nil {
		return nil, fmt.Errorf("jwt.NewWithClock: %w", err)
	}

	return &Manager{
		cfg:       c,
		signer:    signer,
		verifier:  newVerifier(c),
		validator: newValidator(c),
		parser:    newParser(),
		rng:       rng,
		clock:     clock,
	}, nil
}

// Issue signs the given Claims and returns a compact JWT string.
func (m *Manager) Issue(claims Claims) (string, error) {
	return m.signer.Sign(claims)
}

// Verify parses the token string and verifies the cryptographic signature.
// It returns the extracted Claims on success.
//
// This method does NOT validate registered claims (exp, nbf, iss, aud).
// Use VerifyAndValidate when you need full authentication-path validation.
func (m *Manager) Verify(tokenString string) (Claims, error) {
	return m.verifier.Verify(tokenString)
}

// ValidateClaims checks the registered claims in c against the Manager's
// configured issuer, audience, and clock skew, using the Manager's clock
// as the reference time.
func (m *Manager) ValidateClaims(c Claims) error {
	return m.validator.Validate(c, m.clock())
}

// VerifyAndValidate is the standard token authentication path. It verifies
// the signature and validates all configured registered claims in one call.
// This is what middleware and protected handlers should use.
func (m *Manager) VerifyAndValidate(tokenString string) (Claims, error) {
	claims, err := m.verifier.Verify(tokenString)
	if err != nil {
		return Claims{}, err
	}
	if err := m.validator.Validate(claims, m.clock()); err != nil {
		return Claims{}, err
	}
	return claims, nil
}

// ParseUnverified decodes the token without verifying the signature.
// The returned Claims MUST NOT be used for authentication or authorization.
//
// Use this only to inspect claims (e.g. the "jti") before deciding whether
// to perform a full verification.
func (m *Manager) ParseUnverified(tokenString string) (Claims, error) {
	return m.parser.ParseUnverified(tokenString)
}

// GenerateJTI returns a cryptographically secure, URL-safe unique token ID
// suitable for use as the "jti" claim. It produces 32 bytes (256 bits) of
// entropy encoded as unpadded Base64URL.
func (m *Manager) GenerateJTI() (string, error) {
	jti, err := m.rng.Base64URL(32)
	if err != nil {
		return "", fmt.Errorf("jwt: failed to generate JTI: %w", err)
	}
	return jti, nil
}
