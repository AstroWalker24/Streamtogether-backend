package validator

import "strings"

// Normalizer is implemented by request structs that want automatic
// field normalisation before validation runs.
type Normalizer interface {
    Normalize()
}

// NormalizeEmail trims surrounding whitespace and lowercases an email string.
func NormalizeEmail(email string) string {
    return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeName trims surrounding whitespace from a display or user name.
func NormalizeName(name string) string {
    return strings.TrimSpace(name)
}

// NormalizeString trims surrounding whitespace from any string field.
func NormalizeString(s string) string {
    return strings.TrimSpace(s)
}