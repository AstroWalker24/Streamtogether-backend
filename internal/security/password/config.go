package password

import "github.com/AstroWalker24/Streamtogether-backend/internal/config"

// Config holds Argon2id parameters used by the Hasher.
type Config struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// defaults returns OWASP-recommended Argon2id parameters suitable for production.
func defaults() Config {
	return Config{
		Memory:      64 * 1024, // 64 MiB
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// fromAppConfig converts an application PasswordConfig into the package Config.
func fromAppConfig(c config.PasswordConfig) Config {
	return Config{
		Memory:      c.Memory,
		Iterations:  c.Iterations,
		Parallelism: c.Parallelism,
		SaltLength:  c.SaltLength,
		KeyLength:   c.KeyLength,
	}
}
