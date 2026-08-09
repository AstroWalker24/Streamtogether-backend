package jwt

import (
	"fmt"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Signer signs JWT payloads using the configured algorithm and key material.
// It is safe for concurrent use; no mutable state is modified after construction.
type Signer struct {
	cfg    Config
	method jwtlib.SigningMethod
}

// newSigner creates a Signer from a validated Config.
func newSigner(cfg Config) (*Signer, error) {
	method, err := resolveSigningMethod(cfg.Algorithm)
	if err != nil {
		return nil, err
	}
	return &Signer{cfg: cfg, method: method}, nil
}

// Sign encodes and signs the given Claims, returning a compact serialized JWT.
// The signing key is never included in returned errors or logs.
func (s *Signer) Sign(claims Claims) (string, error) {
	key, err := s.cfg.keyProvider.SigningKey()
	if err != nil {
		return "", fmt.Errorf("%w: failed to obtain signing key", ErrInvalidToken)
	}
	token := jwtlib.NewWithClaims(s.method, claims.toLibClaims())
	signed, err := token.SignedString(key)
	if err != nil {
		// Do not wrap the library error directly; it might contain key material.
		return "", fmt.Errorf("%w: failed to produce signed token", ErrInvalidToken)
	}
	return signed, nil
}

// resolveSigningMethod maps an algorithm name to the corresponding library
// signing method. Only AlgorithmHS256 is accepted.
func resolveSigningMethod(alg string) (jwtlib.SigningMethod, error) {
	switch alg {
	case AlgorithmHS256:
		return jwtlib.SigningMethodHS256, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, alg)
	}
}
