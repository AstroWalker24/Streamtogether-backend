// Package password provides Argon2id-based password hashing and verification.
//
// It is algorithm-agnostic from the caller's perspective: callers create a
// Hasher and call Hash, Verify, and NeedsRehash without any knowledge of
// Argon2id internals.
package password

import "github.com/AstroWalker24/Streamtogether-backend/internal/config"

// Hasher hashes and verifies passwords using Argon2id.
// It is safe for concurrent use.
type Hasher struct {
    cfg Config
}

// New creates a Hasher configured from the application PasswordConfig.
func New(c config.PasswordConfig) *Hasher {
    return &Hasher{cfg: fromAppConfig(c)}
}

// NewWithDefaults creates a Hasher using built-in OWASP-recommended parameters.
// Use this when a Config is not available (e.g. in tests).
func NewWithDefaults() *Hasher {
    return &Hasher{cfg: defaults()}
}

// Hash returns a PHC-format Argon2id hash of plaintext.
func (h *Hasher) Hash(plaintext string) (string, error) {
    return hash(plaintext, h.cfg)
}

// Verify reports whether plaintext matches the Argon2id-encoded hash.
// It returns (true, nil) on a match, (false, nil) on a mismatch, and
// (false, err) when the encoded string is malformed.
func (h *Hasher) Verify(plaintext, encoded string) (bool, error) {
    return verify(plaintext, encoded, h.cfg)
}

// NeedsRehash reports whether the encoded hash was produced with parameters
// that differ from the current Hasher configuration or an outdated Argon2
// version. When true, rehash the password on the next successful login.
func (h *Hasher) NeedsRehash(encoded string) (bool, error) {
    return needsRehash(encoded, h.cfg)
}