// Package crypto provides low-level cryptographic helpers for use throughout
// the application. It does not implement passwords, tokens, or business logic.
package crypto

import (
	"crypto/sha256"
	"crypto/sha512"
)

// SHA256 returns the SHA-256 digest of data.
func SHA256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// SHA256Hex returns the hex-encoded SHA-256 digest of data.
func SHA256Hex(data []byte) string {
	return HexEncode(SHA256(data))
}

// SHA512 returns the SHA-512 digest of data.
func SHA512(data []byte) []byte {
	h := sha512.Sum512(data)
	return h[:]
}

// SHA512Hex returns the hex-encoded SHA-512 digest of data.
func SHA512Hex(data []byte) string {
	return HexEncode(SHA512(data))
}

// HashBytes returns the SHA-256 digest of data. Use this when the specific
// algorithm is an implementation detail the caller need not know.
func HashBytes(data []byte) []byte {
	return SHA256(data)
}
