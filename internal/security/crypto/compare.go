package crypto

import "crypto/subtle"

// Equal reports whether a and b contain identical bytes, using constant-time comparison.
func Equal(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// EqualStrings reports whether a and b are identical strings, using constant-time comparison.
func EqualStrings(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
