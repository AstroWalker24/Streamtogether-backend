package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
)

// AlgorithmHS256 is the only signing algorithm accepted by this package.
// The constant is exported so callers can reference it without a string literal.
const AlgorithmHS256 = "HS256"

// Config is the resolved JWT configuration consumed by the Manager.
// Construct it via NewConfig or NewConfigWithKeyProvider; do not initialize
// this struct directly.
type Config struct {
	// keyProvider is the source of signing and verification key material.
	// Unexported to prevent direct access from outside this package.
	keyProvider KeyProvider

	// Algorithm is the signing algorithm enforced for all operations.
	// Derived from keyProvider.Algorithm() at construction time.
	Algorithm string

	// Issuer is the expected "iss" claim value. Validation skipped when empty.
	Issuer string

	// Audience is the expected "aud" claim value. Validation skipped when empty.
	Audience string

	// AccessTokenExpiry is the lifetime applied when building new access tokens.
	AccessTokenExpiry time.Duration

	// ClockSkew is the symmetric tolerance for clock differences between this
	// process and the token's registered time claims. Zero means strict.
	ClockSkew time.Duration
}

// NewConfig constructs a Config from the application JWTConfig, building an
// HS256KeyProvider from the configured secret. It fails if the secret is
// absent or too short, the declared algorithm is not HS256, or the access
// token expiry is not positive.
func NewConfig(cfg config.JWTConfig) (Config, error) {
	kp, err := NewHS256KeyProvider(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("jwt.NewConfig: %w", err)
	}
	return buildConfig(kp, cfg)
}

// NewConfigWithKeyProvider constructs a Config using an explicitly supplied
// KeyProvider. Use this when key material comes from a source other than the
// application config (e.g. a secrets manager, an HSM, or a rotating key set).
//
// kp must not be nil and must be safe for concurrent use.
func NewConfigWithKeyProvider(kp KeyProvider, cfg config.JWTConfig) (Config, error) {
	if kp == nil {
		return Config{}, errors.New("jwt: key provider must not be nil")
	}
	return buildConfig(kp, cfg)
}

// buildConfig assembles the Config value shared by both constructors.
func buildConfig(kp KeyProvider, cfg config.JWTConfig) (Config, error) {
	// Resolve the algorithm; default to what the provider declares.
	alg := cfg.Algorithm
	if alg == "" {
		alg = kp.Algorithm()
	}

	// Reject mismatches between the declared algorithm and the provider.
	if alg != kp.Algorithm() {
		return Config{}, fmt.Errorf("%w: config declares %q but key provider supplies %q",
			ErrUnsupportedAlgorithm, alg, kp.Algorithm())
	}

	// Only HS256 is currently supported.
	if alg != AlgorithmHS256 {
		return Config{}, fmt.Errorf("%w: %q is not in the supported set", ErrUnsupportedAlgorithm, alg)
	}

	if cfg.AccessTokenExpiry <= 0 {
		return Config{}, errors.New("jwt: access token expiry must be positive")
	}

	return Config{
		keyProvider:       kp,
		Algorithm:         alg,
		Issuer:            cfg.Issuer,
		Audience:          cfg.Audience,
		AccessTokenExpiry: cfg.AccessTokenExpiry,
		ClockSkew:         cfg.ClockSkew,
	}, nil
}
