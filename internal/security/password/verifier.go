package password

import (
    "crypto/subtle"
    "encoding/base64"
    "fmt"
    "strings"

    "golang.org/x/crypto/argon2"
)

// verify reports whether plaintext matches the PHC-encoded Argon2id hash.
func verify(plaintext, encoded string, cfg Config) (bool, error) {
    params, salt, storedHash, err := decode(encoded)
    if err != nil {
        return false, err
    }
    if params.Version != argon2.Version {
        return false, ErrIncompatibleVersion
    }

    candidate := argon2.IDKey(
        []byte(plaintext),
        salt,
        params.Iterations,
        params.Memory,
        params.Parallelism,
        params.KeyLength,
    )

    match := subtle.ConstantTimeCompare(storedHash, candidate) == 1
    return match, nil
}

// needsRehash reports whether the encoded hash was produced with parameters
// that differ from cfg or uses an outdated Argon2 version.
func needsRehash(encoded string, cfg Config) (bool, error) {
    params, _, _, err := decode(encoded)
    if err != nil {
        return false, err
    }
    return params.Version != argon2.Version ||
        params.Memory != cfg.Memory ||
        params.Iterations != cfg.Iterations ||
        params.Parallelism != cfg.Parallelism ||
        params.KeyLength != cfg.KeyLength, nil
}

// hashParams holds the decoded Argon2id parameters from a PHC string.
type hashParams struct {
    Version     int
    Memory      uint32
    Iterations  uint32
    Parallelism uint8
    KeyLength   uint32
}

// decode parses a PHC-format Argon2id string.
// Expected: $argon2id$v=<v>$m=<m>,t=<t>,p=<p>$<b64salt>$<b64hash>
func decode(encoded string) (params hashParams, salt, hash []byte, err error) {
    parts := strings.SplitN(encoded, "$", 6)
    if len(parts) != 6 || parts[1] != "argon2id" {
        return params, nil, nil, ErrInvalidHash
    }

    var version int
    if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
        return params, nil, nil, ErrInvalidHash
    }

    var memory, iterations uint32
    var parallelism uint8
    if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
        return params, nil, nil, ErrInvalidHash
    }

    salt, err = base64.RawStdEncoding.DecodeString(parts[4])
    if err != nil {
        return params, nil, nil, ErrInvalidHash
    }

    hash, err = base64.RawStdEncoding.DecodeString(parts[5])
    if err != nil {
        return params, nil, nil, ErrInvalidHash
    }

    params = hashParams{
        Version:     version,
        Memory:      memory,
        Iterations:  iterations,
        Parallelism: parallelism,
        KeyLength:   uint32(len(hash)),
    }
    return params, salt, hash, nil
}