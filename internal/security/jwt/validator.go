package jwt

import (
	"fmt"
	"time"
)

// Validator checks registered JWT claims against configured expectations and
// the current time. It does not interact with the JWT library.
//
// Validator is safe for concurrent use.
type Validator struct {
	cfg Config
}

// newValidator creates a Validator from a validated Config.
func newValidator(cfg Config) *Validator {
	return &Validator{cfg: cfg}
}

// Validate checks the registered claims in c against the supplied now time.
//
// Clock skew (Config.ClockSkew) is applied symmetrically:
//   - "exp" is accepted while now ≤ exp + skew
//   - "nbf" is accepted while now ≥ nbf − skew
//
// Issuer and Audience validation are skipped when the corresponding Config
// fields are empty, allowing the JWT package to be used without requiring
// those claims.
func (v *Validator) Validate(c Claims, now time.Time) error {
	skew := v.cfg.ClockSkew

	if !c.ExpiresAt.IsZero() {
		deadline := c.ExpiresAt.Add(skew)
		if now.After(deadline) {
			return ErrExpiredToken
		}
	}

	if !c.NotBefore.IsZero() {
		earliest := c.NotBefore.Add(-skew)
		if now.Before(earliest) {
			return ErrTokenNotYetValid
		}
	}

	if v.cfg.Issuer != "" && c.Issuer != v.cfg.Issuer {
		// Do not include the configured issuer in the error to avoid leaking
		// internal topology details to callers who propagate errors to clients.
		return fmt.Errorf("%w: got %q", ErrInvalidIssuer, c.Issuer)
	}

	if v.cfg.Audience != "" && !audienceContains(c.Audience, v.cfg.Audience) {
		return fmt.Errorf("%w: required audience not present", ErrInvalidAudience)
	}

	return nil
}

// audienceContains reports whether want appears in the "aud" slice.
func audienceContains(aud []string, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}
