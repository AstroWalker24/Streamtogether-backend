package config

import (
	"strings"
	"testing"
	"time"
)

// validConfig returns a fully populated Config that passes all validation.
// Individual tests mutate a copy of it to exercise specific failure paths.
func validConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:        "test-app",
			Environment: EnvDevelopment,
			Host:        "0.0.0.0",
			Port:        8080,
		},
		Server: ServerConfig{
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    15 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			User:            "user",
			Password:        "pass",
			Database:        "db",
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
		},
		JWT: JWTConfig{
			Secret:             "supersecret",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 168 * time.Hour,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		RateLimit: RateLimitConfig{
			Enabled:  true,
			Requests: 100,
			Duration: 1 * time.Minute,
		},
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	if err := Validate(validConfig()); err != nil {
		t.Fatalf("expected no error for valid config, got: %v", err)
	}
}

// ── App ──────────────────────────────────────────────────────────────────────

func TestValidate_App_EmptyName(t *testing.T) {
	cfg := validConfig()
	cfg.App.Name = ""
	assertContains(t, Validate(cfg), "app.name")
}

func TestValidate_App_EmptyHost(t *testing.T) {
	cfg := validConfig()
	cfg.App.Host = ""
	assertContains(t, Validate(cfg), "app.host")
}

func TestValidate_App_InvalidEnvironment(t *testing.T) {
	cfg := validConfig()
	cfg.App.Environment = "staging"
	assertContains(t, Validate(cfg), "app.environment")
}

func TestValidate_App_AllEnvironmentsValid(t *testing.T) {
	for _, env := range []Environment{EnvDevelopment, EnvTest, EnvProduction} {
		cfg := validConfig()
		cfg.App.Environment = env
		if err := Validate(cfg); err != nil {
			t.Errorf("expected valid for env=%s, got: %v", env, err)
		}
	}
}

func TestValidate_App_PortBoundaries(t *testing.T) {
	cases := []struct {
		port    int
		wantErr bool
	}{
		{0, true},
		{-1, true},
		{65536, true},
		{1, false},     // lower boundary
		{65535, false}, // upper boundary
		{8080, false},
	}
	for _, tc := range cases {
		cfg := validConfig()
		cfg.App.Port = tc.port
		err := Validate(cfg)
		if tc.wantErr && err == nil {
			t.Errorf("port=%d: expected error, got nil", tc.port)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("port=%d: expected no error, got %v", tc.port, err)
		}
	}
}

// ── Server ───────────────────────────────────────────────────────────────────

func TestValidate_Server_ZeroTimeouts(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"readTimeout", func(c *Config) { c.Server.ReadTimeout = 0 }, "server.read_timeout"},
		{"writeTimeout", func(c *Config) { c.Server.WriteTimeout = 0 }, "server.write_timeout"},
		{"idleTimeout", func(c *Config) { c.Server.IdleTimeout = 0 }, "server.idle_timeout"},
		{"shutdownTimeout", func(c *Config) { c.Server.ShutdownTimeout = 0 }, "server.shutdown_timeout"},
	}
	for _, tc := range cases {
		cfg := validConfig()
		tc.mutate(cfg)
		assertContains(t, Validate(cfg), tc.want)
	}
}

func TestValidate_Server_NegativeTimeout(t *testing.T) {
	cfg := validConfig()
	cfg.Server.ReadTimeout = -1 * time.Second
	assertContains(t, Validate(cfg), "server.read_timeout")
}

// ── Database ─────────────────────────────────────────────────────────────────

func TestValidate_Database_EmptyHost(t *testing.T) {
	cfg := validConfig()
	cfg.Database.Host = ""
	assertContains(t, Validate(cfg), "database.host")
}

func TestValidate_Database_EmptyUser(t *testing.T) {
	cfg := validConfig()
	cfg.Database.User = ""
	assertContains(t, Validate(cfg), "database.user")
}

func TestValidate_Database_EmptyName(t *testing.T) {
	cfg := validConfig()
	cfg.Database.Database = ""
	assertContains(t, Validate(cfg), "database.database")
}

func TestValidate_Database_InvalidPort(t *testing.T) {
	cfg := validConfig()
	cfg.Database.Port = 0
	assertContains(t, Validate(cfg), "database.port")
}

func TestValidate_Database_MaxOpenConnsZero(t *testing.T) {
	cfg := validConfig()
	cfg.Database.MaxOpenConns = 0
	assertContains(t, Validate(cfg), "database.max_open_conns")
}

func TestValidate_Database_NegativeMaxIdleConns(t *testing.T) {
	cfg := validConfig()
	cfg.Database.MaxIdleConns = -1
	assertContains(t, Validate(cfg), "database.max_idle_conns")
}

func TestValidate_Database_ZeroConnLifetime(t *testing.T) {
	cfg := validConfig()
	cfg.Database.ConnMaxLifetime = 0
	assertContains(t, Validate(cfg), "database.conn_max_lifetime")
}

// ── Redis ────────────────────────────────────────────────────────────────────

func TestValidate_Redis_EmptyHost(t *testing.T) {
	cfg := validConfig()
	cfg.Redis.Host = ""
	assertContains(t, Validate(cfg), "redis.host")
}

func TestValidate_Redis_InvalidPort(t *testing.T) {
	cfg := validConfig()
	cfg.Redis.Port = 99999
	assertContains(t, Validate(cfg), "redis.port")
}

func TestValidate_Redis_NegativeDB(t *testing.T) {
	cfg := validConfig()
	cfg.Redis.DB = -1
	assertContains(t, Validate(cfg), "redis.db")
}

// ── JWT ──────────────────────────────────────────────────────────────────────

func TestValidate_JWT_EmptySecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWT.Secret = ""
	assertContains(t, Validate(cfg), "jwt.secret")
}

func TestValidate_JWT_ZeroAccessExpiry(t *testing.T) {
	cfg := validConfig()
	cfg.JWT.AccessTokenExpiry = 0
	assertContains(t, Validate(cfg), "jwt.access_token_expiry")
}

func TestValidate_JWT_ZeroRefreshExpiry(t *testing.T) {
	cfg := validConfig()
	cfg.JWT.RefreshTokenExpiry = 0
	assertContains(t, Validate(cfg), "jwt.refresh_token_expiry")
}

func TestValidate_Logging_InvalidLevel(t *testing.T) {
	cfg := validConfig()
	cfg.Logging.Level = "verbose"
	assertContains(t, Validate(cfg), "logging.level")
}

func TestValidate_Logging_InvalidFormat(t *testing.T) {
	cfg := validConfig()
	cfg.Logging.Format = "yaml"
	assertContains(t, Validate(cfg), "logging.format")
}

func TestValidate_Logging_AllLevelsValid(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		cfg := validConfig()
		cfg.Logging.Level = level
		if err := Validate(cfg); err != nil {
			t.Errorf("level=%s: expected valid, got: %v", level, err)
		}
	}
}

// ── RateLimit ────────────────────────────────────────────────────────────────

func TestValidate_RateLimit_EnabledWithZeroRequests(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.Requests = 0
	assertContains(t, Validate(cfg), "ratelimit.requests")
}

func TestValidate_RateLimit_EnabledWithZeroDuration(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.Duration = 0
	assertContains(t, Validate(cfg), "ratelimit.duration")
}

func TestValidate_RateLimit_DisabledIgnoresValues(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimit.Enabled = false
	cfg.RateLimit.Requests = 0
	cfg.RateLimit.Duration = 0
	if err := Validate(cfg); err != nil {
		t.Errorf("disabled rate limit with zero values should be valid, got: %v", err)
	}
}

// ── Multiple errors ───────────────────────────────────────────────────────────

func TestValidate_ReportsAllErrors(t *testing.T) {
	cfg := validConfig()
	cfg.App.Name = ""
	cfg.Database.Host = ""
	cfg.JWT.Secret = ""

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected errors, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"app.name", "database.host", "jwt.secret"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to mention %q, got: %s", want, msg)
		}
	}
}

// ── helper ───────────────────────────────────────────────────────────────────

func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error to contain %q\ngot: %s", substr, err.Error())
	}
}
