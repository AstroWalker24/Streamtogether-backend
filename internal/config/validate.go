package config

import (
	"errors"
	"fmt"
)

func Validate(cfg *Config) error {
	var errs []error

	errs = append(errs, validateApp(cfg.App)...)
	errs = append(errs, validateServer(cfg.Server)...)
	errs = append(errs, validateDatabase(cfg.Database)...)
	errs = append(errs, validateRedis(cfg.Redis)...)
	errs = append(errs, validateJWT(cfg.JWT)...)
	errs = append(errs, validateLogging(cfg.Logging)...)
	errs = append(errs, validateRateLimit(cfg.RateLimit)...)

	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %v", errs)
	}
	return nil
}

func validateApp(a AppConfig) []error {
	var errs []error
	if a.Name == "" {
		errs = append(errs, errors.New("app.name must not be empty"))
	}

	switch a.Environment {
	case EnvDevelopment, EnvTest, EnvProduction:
		// valid
	default:
		errs = append(errs,fmt.Errorf("app.environment %q is not one of: development, test, production", a.Environment))
	}

	if a.Port < 1 || a.Port > 65535 {
		errs = append(errs, fmt.Errorf("app.port %d is not a valid port number", a.Port))
	}

	if a.Host == "" {
		errs = append(errs, errors.New("app.host must not be empty"))
	}
	return errs
}

func validateServer(s ServerConfig) []error {
	var errs []error

	if s.ReadTimeout <= 0 {
		errs = append(errs, fmt.Errorf("server.read_timeout %d must be greater than 0", s.ReadTimeout))
	}

	if s.WriteTimeout <= 0 {
		errs = append(errs, fmt.Errorf("server.write_timeout %d must be greater than 0", s.WriteTimeout))
	}

	if s.IdleTimeout <= 0 {
		errs = append(errs, fmt.Errorf("server.idle_timeout %d must be greater than 0", s.IdleTimeout))
	}

	if s.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("server.shutdown_timeout %d must be greater than 0", s.ShutdownTimeout))
	}

	return errs 
}

func validateDatabase(db DatabaseConfig) []error {
	var errs []error 

	if db.Host == "" {
		errs = append(errs, errors.New("database.host must not be empty"))
	}
	
	if db.Port < 1 || db.Port > 65535 {
		errs = append(errs, fmt.Errorf("database.port %d is not a valid port number", db.Port))
	}
	if db.User == "" {
		errs = append(errs, errors.New("database.user must not be empty"))
	}
	if db.Database == "" {
		errs = append(errs, errors.New("database.database must not be empty"))
	}
	if db.SSLMode == "" {
		errs = append(errs, errors.New("database.sslmode must not be empty"))
	}
	if db.MaxOpenConns < 1 {
		errs = append(errs, fmt.Errorf("database.max_open_conns %d must be greater than 0", db.MaxOpenConns))
	}
	if db.MaxIdleConns < 0 {
		errs = append(errs, fmt.Errorf("database.max_idle_conns %d must not be negative", db.MaxIdleConns))
	}
	if db.ConnMaxLifetime <= 0 {
		errs = append(errs, fmt.Errorf("database.conn_max_lifetime %d must be greater than 0", db.ConnMaxLifetime))
	}

	return errs
}

func validateRedis(r RedisConfig) []error {
	var errs []error 

	if r.Host == "" {
		errs = append(errs, errors.New("redis.host must not be empty"))
	}
	if r.Port < 1 || r.Port > 65535 {
		errs = append(errs, fmt.Errorf("redis.port %d is not a valid port number", r.Port))
	}
	if r.DB < 0 {
		errs = append(errs, fmt.Errorf("redis.db %d must not be negative", r.DB))
	}
	return errs
}

func validateJWT(j JWTConfig) []error {
	var errs []error

	if j.Secret == "" {
		errs = append(errs, errors.New("jwt.secret must not be empty"))	
	}

	if j.AccessTokenExpiry <= 0 {
		errs = append(errs, fmt.Errorf("jwt.access_token_expiry %d must be greater than 0", j.AccessTokenExpiry))
	}
	
	if j.RefreshTokenExpiry <= 0 {
		errs = append(errs, fmt.Errorf("jwt.refresh_token_expiry %d must be greater than 0", j.RefreshTokenExpiry))
	}
	return errs
}

func validateLogging(l LoggingConfig) []error {
	var errs []error
	switch l.Level {
		case "debug", "info", "warn", "error":
			// valid
		default:
			errs = append(errs, fmt.Errorf("logging.level %q is not one of: debug, info, warn, error", l.Level))
	}

	switch l.Format {
	case "json", "text":
		// valid
	default:
		errs = append(errs, fmt.Errorf("logging.format %q is not one of: json, text", l.Format))
	}
	return errs
}

func validateRateLimit(r RateLimitConfig) []error {
	var errs []error

	if r.Enabled {
		if r.Requests < 1 {
			errs = append(errs, fmt.Errorf("ratelimit.requests %d must be greater than 0", r.Requests))
		}
		if r.Duration <= 0 {
			errs = append(errs, fmt.Errorf("ratelimit.duration %d must be greater than 0", r.Duration))
		}
	}
	return errs
}