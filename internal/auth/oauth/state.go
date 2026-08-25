package oauth

import (
	"crypto/subtle"

	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// stateBytes is the number of random bytes used for an OAuth state token (256-bit).
const stateBytes = 32

// GenerateState produces a cryptographically secure state string for use as
// an OAuth CSRF token. The caller is responsible for associating the returned
// value with the current user session before redirecting to the provider.
func GenerateState(rng *random.Generator) (string, error) {
	return rng.Base64URL(stateBytes)
}

// ValidateState compares the received state from the provider callback against
// the expected value stored at flow initiation. Uses constant-time comparison
// to prevent timing attacks.
func ValidateState(received, expected string) bool {
	if received == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(received), []byte(expected)) == 1
}
