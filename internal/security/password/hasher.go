package password

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// generateSalt returns a cryptographically secure random salt.
func generateSalt(length uint32) ([]byte, error) {
	salt := make([]byte, length)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// hash derives an Argon2id key and returns it in PHC string format:
// $argon2id$v=<v>$m=<m>,t=<t>,p=<p>$<base64salt>$<base64hash>
func hash(plaintext string, cfg Config) (string, error) {
	salt, err := generateSalt(cfg.SaltLength)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrHashFailed, err)
	}

	key := argon2.IDKey(
		[]byte(plaintext),
		salt,
		cfg.Iterations,
		cfg.Memory,
		cfg.Parallelism,
		cfg.KeyLength,
	)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		cfg.Memory,
		cfg.Iterations,
		cfg.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	return encoded, nil
}
