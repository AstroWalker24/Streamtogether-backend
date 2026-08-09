package jwt

import (
	"fmt"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
)

// minHS256KeyBytes is the minimum accepted HMAC secret length.
// NIST SP 800-107 recommends the key be at least as long as the hash output:
// 32 bytes (256 bits) for SHA-256.
const minHS256KeyBytes = 32

// HS256KeyProvider implements KeyProvider using a symmetric HMAC secret for
// HS256 signing. Key material is immutable after construction; the provider
// is safe for concurrent use.
type HS256KeyProvider struct {
	key []byte // never exposed through exported fields or methods
}

// NewHS256KeyProvider constructs an HS256KeyProvider from the application
// JWTConfig. It validates the secret at construction time and fails fast if:
//   - The secret is absent.
//   - The secret is shorter than the 32-byte minimum required for HS256.
//
// The secret value is never included in returned errors.
func NewHS256KeyProvider(cfg config.JWTConfig) (*HS256KeyProvider, error) {
	if cfg.Secret == "" {
		return nil, ErrMissingSigningKey
	}
	raw := []byte(cfg.Secret)
	if len(raw) < minHS256KeyBytes {
		return nil, fmt.Errorf("%w: HS256 requires at least %d bytes; configured key has %d",
			ErrInvalidSigningKey, minHS256KeyBytes, len(raw))
	}
	// Defensive copy: the provider owns its key slice independently of the
	// caller's string backing array.
	key := make([]byte, len(raw))
	copy(key, raw)
	return &HS256KeyProvider{key: key}, nil
}

// Algorithm returns AlgorithmHS256.
func (p *HS256KeyProvider) Algorithm() string { return AlgorithmHS256 }

// SigningKey returns the HMAC secret as []byte. The returned slice must not
// be logged or persisted by the caller.
func (p *HS256KeyProvider) SigningKey() (any, error) { return p.key, nil }

// VerificationKey returns the same HMAC secret as SigningKey. HS256 is a
// symmetric algorithm; the same key both signs and verifies.
func (p *HS256KeyProvider) VerificationKey() (any, error) { return p.key, nil }
