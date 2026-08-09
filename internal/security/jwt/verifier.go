package jwt

import (
	"errors"
	"fmt"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Verifier parses a compact JWT string, enforces the configured signing
// algorithm, and verifies the cryptographic signature.
//
// Registered claim validation (exp, nbf, iss, aud) is deliberately NOT
// performed here. Call Validator.Validate separately, or use
// Manager.VerifyAndValidate for the combined operation.
//
// Verifier is safe for concurrent use.
type Verifier struct {
	cfg Config
}

// newVerifier creates a Verifier from a validated Config.
func newVerifier(cfg Config) *Verifier {
	return &Verifier{cfg: cfg}
}

// Verify parses the token string, enforces the configured algorithm, and
// verifies the cryptographic signature. It returns the extracted Claims on
// success.
//
// Security: the keyFunc explicitly rejects any algorithm not matching the
// one declared in Config, preventing algorithm-confusion attacks where an
// attacker manipulates the token header to select a weaker algorithm.
func (v *Verifier) Verify(tokenString string) (Claims, error) {
	token, err := jwtlib.ParseWithClaims(
		tokenString,
		jwtlib.MapClaims{},
		v.keyFunc(),
		// Delegate registered-claim validation to Validator; the Verifier's
		// sole concern is the cryptographic signature.
		jwtlib.WithoutClaimsValidation(),
	)
	if err != nil {
		return Claims{}, mapVerifyError(err)
	}

	mc, ok := token.Claims.(jwtlib.MapClaims)
	if !ok || !token.Valid {
		return Claims{}, ErrInvalidToken
	}

	return fromLibClaims(mc), nil
}

// keyFunc returns the key function passed to the JWT library during parsing.
// It enforces the configured algorithm before requesting key material from the
// provider, preventing algorithm-confusion attacks.
func (v *Verifier) keyFunc() jwtlib.Keyfunc {
	return func(token *jwtlib.Token) (any, error) {
		if token.Method.Alg() != v.cfg.Algorithm {
			return nil, fmt.Errorf("%w: token uses %q, configured for %q",
				ErrUnsupportedAlgorithm, token.Method.Alg(), v.cfg.Algorithm)
		}
		key, err := v.cfg.keyProvider.VerificationKey()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrKeyProviderUnavailable, err)
		}
		return key, nil
	}
}

// mapVerifyError translates jwt library errors into the package's error
// vocabulary. Third-party error types are never returned to callers.
func mapVerifyError(err error) error {
	if errors.Is(err, ErrUnsupportedAlgorithm) {
		return ErrUnsupportedAlgorithm
	}
	if errors.Is(err, ErrKeyProviderUnavailable) {
		return ErrKeyProviderUnavailable
	}
	if errors.Is(err, jwtlib.ErrTokenMalformed) {
		return ErrMalformedToken
	}
	if errors.Is(err, jwtlib.ErrTokenSignatureInvalid) {
		return ErrInvalidSignature
	}
	if errors.Is(err, jwtlib.ErrTokenExpired) {
		return ErrExpiredToken
	}
	if errors.Is(err, jwtlib.ErrTokenNotValidYet) {
		return ErrTokenNotYetValid
	}
	return fmt.Errorf("%w: %v", ErrInvalidToken, err)
}
