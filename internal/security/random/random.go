// Package random provides cryptographically secure random value generation.
//
// All values are produced using crypto/rand. This package is the single
// approved source of randomness in the application; callers must not use
// crypto/rand or math/rand directly.
package random

// Generator produces cryptographically secure random values.
// It carries no mutable state and is safe for concurrent use.
type Generator struct{}

// New returns a Generator ready for use.
func New() *Generator {
	return &Generator{}
}

// Bytes returns n cryptographically secure random bytes.
func (g *Generator) Bytes(n int) ([]byte, error) {
	return generateBytes(n)
}

// Hex returns the hex encoding of n random bytes, producing a 2n-character string.
func (g *Generator) Hex(n int) (string, error) {
	b, err := generateBytes(n)
	if err != nil {
		return "", err
	}
	return encodeHex(b), nil
}

// Base64URL returns the unpadded Base64URL encoding of n random bytes.
func (g *Generator) Base64URL(n int) (string, error) {
	b, err := generateBytes(n)
	if err != nil {
		return "", err
	}
	return encodeBase64URL(b), nil
}

// Numeric returns a string of length random decimal digits (0–9).
func (g *Generator) Numeric(length int) (string, error) {
	return generateString(length, AlphabetNumeric)
}

// String returns a string of length random characters drawn from alphabet.
// Use the Alphabet* constants for common character sets.
func (g *Generator) String(length int, alphabet string) (string, error) {
	return generateString(length, alphabet)
}

// UUIDBytes returns 16 cryptographically secure random bytes suitable
// for constructing a UUID. Callers are responsible for formatting.
func (g *Generator) UUIDBytes() ([]byte, error) {
	return generateBytes(16)
}
