package token

import "errors"

// Sentinel errors returned by Manager. Callers use errors.Is against these
// sentinels and do not need to import the JWT Foundation for error checking.
//
// JWT Foundation errors are always translated into one of these before being
// returned; the underlying cause string is captured in the error message.
var (
	// ErrAccessTokenCreation is returned when Manager cannot produce a signed
	// access token due to invalid input or a signing failure.
	ErrAccessTokenCreation = errors.New("token: failed to create access token")

	// ErrInvalidAccessToken is the root sentinel for validation failures not
	// covered by a more specific error below.
	ErrInvalidAccessToken = errors.New("token: access token is invalid")

	// ErrAccessTokenExpired indicates the token's "exp" claim is in the past
	// relative to the JWT Foundation's configured clock.
	ErrAccessTokenExpired = errors.New("token: access token has expired")

	// ErrAccessTokenMalformed indicates the token string is structurally
	// invalid (wrong number of segments, bad base64, unparseable JSON).
	ErrAccessTokenMalformed = errors.New("token: access token is malformed")

	// ErrInvalidAccessTokenSignature indicates the token failed cryptographic
	// signature verification against the configured signing key.
	ErrInvalidAccessTokenSignature = errors.New("token: access token signature is invalid")

	// ErrUnsupportedTokenAlgorithm indicates the token header declares an
	// algorithm other than the one the Manager is configured to accept.
	ErrUnsupportedTokenAlgorithm = errors.New("token: signing algorithm is not supported")
)
