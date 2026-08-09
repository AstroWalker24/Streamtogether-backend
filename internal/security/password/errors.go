package password

import "errors"

// Sentinel errors returned by the password package.
var (
    // ErrHashFailed is returned when salt generation or key derivation fails.
    ErrHashFailed = errors.New("password: hashing failed")

    // ErrInvalidHash is returned when the encoded hash string is malformed.
    ErrInvalidHash = errors.New("password: invalid hash format")

    // ErrIncompatibleVersion is returned when the hash uses an unsupported Argon2 version.
    ErrIncompatibleVersion = errors.New("password: incompatible argon2 version")
)