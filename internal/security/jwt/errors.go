package jwt

import "errors"

// Sentinel errors returned by the jwt package. Callers should use errors.Is
// to check for specific failure categories.
//
// Third-party JWT library error types are never returned directly; they are
// always translated into one of the sentinels below.
var (
	// ErrInvalidToken is returned when a token fails validation for a reason
	// not covered by a more specific error.
	ErrInvalidToken = errors.New("jwt: token is invalid")

	// ErrMalformedToken is returned when the token string cannot be decoded
	// (e.g. wrong number of segments, bad base64, invalid JSON payload).
	ErrMalformedToken = errors.New("jwt: token is malformed")

	// ErrInvalidSignature is returned when the token signature does not match
	// the expected value for the configured key.
	ErrInvalidSignature = errors.New("jwt: signature verification failed")

	// ErrUnsupportedAlgorithm is returned when the token header declares an
	// algorithm other than the one configured. This error is also returned
	// during configuration if an unsupported algorithm is specified.
	//
	// Security: this error is returned before any key material is applied,
	// preventing algorithm-confusion attacks.
	ErrUnsupportedAlgorithm = errors.New("jwt: algorithm is not supported")

	// ErrExpiredToken is returned when the current time is after the token's
	// "exp" claim (accounting for configured clock skew).
	ErrExpiredToken = errors.New("jwt: token has expired")

	// ErrTokenNotYetValid is returned when the current time is before the
	// token's "nbf" claim (accounting for configured clock skew).
	ErrTokenNotYetValid = errors.New("jwt: token is not yet valid")

	// ErrInvalidIssuer is returned when the token's "iss" claim does not
	// match the configured issuer.
	ErrInvalidIssuer = errors.New("jwt: issuer is invalid")

	// ErrInvalidAudience is returned when the configured audience is not
	// present in the token's "aud" claim.
	ErrInvalidAudience = errors.New("jwt: audience is invalid")

	// ErrInvalidClaims is returned for general claim-level validation failures
	// not covered by a more specific error.
	ErrInvalidClaims = errors.New("jwt: claims are invalid")
)
