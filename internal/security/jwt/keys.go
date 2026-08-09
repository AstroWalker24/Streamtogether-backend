package jwt

import "errors"

// Key management sentinel errors. Callers use errors.Is to check for specific
// failure categories. The signing secret is never present in any of these
// error values or their messages.
var (
	// ErrMissingSigningKey is returned when no signing secret is configured.
	// Fail fast at application start rather than on the first token request.
	ErrMissingSigningKey = errors.New("jwt: signing key is not configured")

	// ErrInvalidSigningKey is returned when the configured secret fails
	// validation (e.g. below the minimum length for the algorithm in use).
	ErrInvalidSigningKey = errors.New("jwt: signing key is invalid")

	// ErrKeyProviderUnavailable is returned when a KeyProvider cannot supply
	// key material. Reserved for future remote providers (HSM, KMS, Vault).
	ErrKeyProviderUnavailable = errors.New("jwt: key provider is unavailable")
)

// KeyProvider supplies signing and verification key material to the JWT
// Foundation. Implementations must be safe for concurrent use after
// construction.
//
// The interface is intentionally minimal for the current HS256 implementation.
// Future providers — RS256/ES256, key rotation, JWKS, external KMS — satisfy
// this interface without requiring any changes to the JWT Foundation, Token
// Manager, or Authentication Service.
type KeyProvider interface {
	// Algorithm returns the JWT signing algorithm identifier this provider
	// supports (e.g. AlgorithmHS256). The Foundation enforces that every token
	// is signed and verified with exactly this algorithm.
	Algorithm() string

	// SigningKey returns the key material used to sign JWTs.
	//   HS256: []byte         (HMAC secret)
	//   RS256: *rsa.PrivateKey
	//   ES256: *ecdsa.PrivateKey
	//
	// The returned value is passed directly to the JWT library. Callers must
	// not log or persist it.
	SigningKey() (any, error)

	// VerificationKey returns the key material used to verify JWT signatures.
	// For symmetric algorithms (HS256) this is identical to SigningKey.
	// For asymmetric algorithms (RS256, ES256) this is the corresponding
	// public key, enabling verification without access to the private key.
	VerificationKey() (any, error)
}
