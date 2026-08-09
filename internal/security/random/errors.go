package random

import "errors"

// Sentinel errors returned by the random package.
var (
	// ErrInvalidLength is returned when a requested length is zero or negative.
	ErrInvalidLength = errors.New("random: length must be greater than zero")

	// ErrEmptyAlphabet is returned when an empty alphabet string is provided.
	ErrEmptyAlphabet = errors.New("random: alphabet must not be empty")

	// ErrAlphabetTooLarge is returned when an alphabet exceeds 256 characters.
	ErrAlphabetTooLarge = errors.New("random: alphabet must not exceed 256 characters")

	// ErrReadFailed is returned when crypto/rand fails to produce bytes.
	ErrReadFailed = errors.New("random: failed to read from crypto/rand")
)
