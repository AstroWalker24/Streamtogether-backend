package database

import (
    "context"
    "os"
    "strings"
    "testing"
    "time"

    "github.com/AstroWalker24/Streamtogether-backend/internal/config"
    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// ── shared test helpers ────────────────────────────────────────────────────────

// validDatabaseConfig returns a DatabaseConfig pointing at the test postgres
// instance. Individual tests may override specific fields.
func validDatabaseConfig() config.DatabaseConfig {
    return config.DatabaseConfig{
        Host:            getEnvOrDefault("TEST_POSTGRES_HOST", "localhost"),
        Port:            5432,
        User:            getEnvOrDefault("TEST_POSTGRES_USER", "streamtogether"),
        Password:        getEnvOrDefault("TEST_POSTGRES_PASSWORD", "streamtogether"),
        Database:        getEnvOrDefault("TEST_POSTGRES_DB", "streamtogether"),
        SSLMode:         "disable",
        MaxOpenConns:    5,
        MaxIdleConns:    2,
        ConnMaxLifetime: 30 * time.Minute,
    }
}

// requirePostgres skips the test when TEST_POSTGRES_DSN is not set, then
// connects and registers cleanup via t.Cleanup.
func requirePostgres(t *testing.T) *Database {
    t.Helper()
    if os.Getenv("TEST_POSTGRES_DSN") == "" {
        t.Skip("set TEST_POSTGRES_DSN to run postgres integration tests")
    }
    db, err := New(context.Background(), validDatabaseConfig(), logger.Nop())
    if err != nil {
        t.Fatalf("failed to connect to test postgres: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return db
}

func getEnvOrDefault(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

// ── unit tests (no real database required) ────────────────────────────────────

// TestConnectWithRetry_CancelledContext verifies that a pre-cancelled context
// causes connectWithRetry to return quickly (well under 2s) without blocking
// on the retry delay.
func TestConnectWithRetry_CancelledContext(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // pre-cancel before first attempt

    // MinConns=0 ensures pgxpool.NewWithConfig returns immediately without
    // attempting any real network connections.
    cfg := validDatabaseConfig()
    cfg.MaxIdleConns = 0
    cfg.Host = "localhost"
    cfg.Port = 1 // closed port; connection refused

    poolCfg, err := buildPoolConfig(cfg)
    if err != nil {
        t.Fatalf("buildPoolConfig: %v", err)
    }

    start := time.Now()
    _, err = connectWithRetry(ctx, poolCfg, logger.Nop())
    elapsed := time.Since(start)

    if err == nil {
        t.Fatal("expected error from cancelled context, got nil")
    }
    if elapsed > 2*time.Second {
        t.Errorf("expected fast return for cancelled context, took %v", elapsed)
    }
    if !strings.Contains(err.Error(), "cancel") && !strings.Contains(err.Error(), "deadline") {
        t.Errorf("expected context-related error, got: %v", err)
    }
}

// TestConnectWithRetry_UnreachableHost_ExhaustsRetries verifies that an
// unreachable host with a short context timeout causes all retries to be
// abandoned and an error to be returned. Marked long because it involves
// at least one network timeout.
func TestConnectWithRetry_UnreachableHost_Timeout(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping slow retry test in short mode")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    cfg := validDatabaseConfig()
    cfg.MaxIdleConns = 0
    cfg.Host = "192.0.2.1" // TEST-NET-1 (RFC 5737) — routable but unreachable
    cfg.Port = 5432

    poolCfg, err := buildPoolConfig(cfg)
    if err != nil {
        t.Fatalf("buildPoolConfig: %v", err)
    }

    _, err = connectWithRetry(ctx, poolCfg, logger.Nop())
    if err == nil {
        t.Fatal("expected error from unreachable host, got nil")
    }
}

// ── integration tests ─────────────────────────────────────────────────────────

func TestNew_ConnectsSuccessfully(t *testing.T) {
    db := requirePostgres(t)
    if db == nil {
        t.Fatal("expected non-nil Database")
    }
}

func TestDatabase_Pool_ReturnsNonNil(t *testing.T) {
    db := requirePostgres(t)
    if db.Pool() == nil {
        t.Error("Pool() returned nil on a connected Database")
    }
}

func TestDatabase_Close_IsIdempotent(t *testing.T) {
    db := requirePostgres(t)

    // Three calls must not panic or deadlock.
    db.Close()
    db.Close()
    db.Close()
}

func TestDatabase_Close_PoolBecomesUnusable(t *testing.T) {
    db := requirePostgres(t)
    db.Close()

    // After closing, Ping must return an error — pool is drained.
    err := db.pool.Ping(context.Background())
    if err == nil {
        t.Error("expected error after Close(), pool should be unusable")
    }
}