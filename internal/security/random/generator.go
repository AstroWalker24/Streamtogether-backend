package random

import (
    "crypto/rand"
    "fmt"
)

// generateBytes returns n cryptographically secure random bytes.
func generateBytes(n int) ([]byte, error) {
    if n <= 0 {
        return nil, ErrInvalidLength
    }
    b := make([]byte, n)
    if _, err := rand.Read(b); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrReadFailed, err)
    }
    return b, nil
}

// generateString builds a random string of the given length using characters
// from alphabet. It uses rejection sampling with a power-of-two bitmask to
// eliminate modulo bias.
func generateString(length int, alphabet string) (string, error) {
    if length <= 0 {
        return "", ErrInvalidLength
    }
    if len(alphabet) == 0 {
        return "", ErrEmptyAlphabet
    }
    if len(alphabet) > 256 {
        return "", ErrAlphabetTooLarge
    }

    n := len(alphabet)

    // Find smallest k where 2^k >= n, then mask = 2^k - 1.
    k := 0
    for (1 << k) < n {
        k++
    }
    mask := (1 << k) - 1

    result := make([]byte, length)
    generated := 0

    // Over-allocate to amortise rand.Read calls across the rejection loop.
    buf := make([]byte, length*2+16)

    for generated < length {
        if _, err := rand.Read(buf); err != nil {
            return "", fmt.Errorf("%w: %v", ErrReadFailed, err)
        }
        for _, b := range buf {
            if generated == length {
                break
            }
            if idx := int(b) & mask; idx < n {
                result[generated] = alphabet[idx]
                generated++
            }
        }
    }

    return string(result), nil
}