package crypto

import "hash"

// Digest returns the digest of data produced by h. The hash is reset before use.
func Digest(data []byte, h hash.Hash) []byte {
	h.Reset()
	h.Write(data)
	return h.Sum(nil)
}

// VerifyDigest reports whether the digest of data matches expected,
// using constant-time comparison to prevent timing attacks.
func VerifyDigest(data, expected []byte, h hash.Hash) bool {
	return Equal(Digest(data, h), expected)
}
