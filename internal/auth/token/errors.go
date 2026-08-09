package token

import "errors"

// Sentinel errors returned by the token package. Callers should use errors.Is
// to distinguish failure categories.
//
// These errors describe structural claim failures and are distinct from the
// cryptographic errors returned by the JWT Foundation (internal/security/jwt).
var (
	// ErrInvalidAuthClaims is returned when BuildInput fails structural validation.
	ErrInvalidAuthClaims = errors.New("token: authentication claims are invalid")

	// ErrMissingSubject is returned when the "sub" claim is absent or empty.
	ErrMissingSubject = errors.New("token: subject claim is missing")

	// ErrMissingSessionID is returned when the "sid" claim is absent from the
	// custom claims map.
	ErrMissingSessionID = errors.New("token: session ID claim is missing")

	// ErrMissingTokenID is returned when the "jti" claim is absent or empty.
	ErrMissingTokenID = errors.New("token: token ID claim is missing")

	// ErrInvalidUserID is returned when the "sub" claim cannot be parsed as
	// a UUID.
	ErrInvalidUserID = errors.New("token: user ID is not a valid UUID")

	// ErrInvalidSessionID is returned when the "sid" claim cannot be parsed
	// as a UUID.
	ErrInvalidSessionID = errors.New("token: session ID is not a valid UUID")

	// ErrInvalidTokenID is returned when the "jti" claim cannot be parsed as
	// a UUID.
	ErrInvalidTokenID = errors.New("token: token ID is not a valid UUID")

	// ErrInvalidClaimType is returned when a custom claim carries a value of
	// the wrong Go type (e.g. "roles" is a number instead of an array, or
	// "sid" is not a string).
	ErrInvalidClaimType = errors.New("token: claim value has unexpected type")
)
