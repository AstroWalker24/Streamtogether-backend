package crypto

import "errors"

// Sentinel errors returned by the crypto package.
var (
	// ErrDecodeFailed is returned when a hex or Base64URL decode operation fails.
	ErrDecodeFailed = errors.New("crypto: failed to decode input")

	// ErrInvalidInput is returned when nil or zero-length input is rejected.
	ErrInvalidInput = errors.New("crypto: invalid input")
)
