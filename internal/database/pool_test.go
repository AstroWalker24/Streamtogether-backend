package database

import (
    "strings"
    "testing"
    "time"
)

func TestBuildPoolConfig_AppliesMaxConns(t *testing.T) {
    cfg := validDatabaseConfig()
    cfg.MaxOpenConns = 42

    poolCfg, err := buildPoolConfig(cfg)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if poolCfg.MaxConns != 42 {
        t.Errorf("expected MaxConns=42, got %d", poolCfg.MaxConns)
    }
}

func TestBuildPoolConfig_AppliesMinConns(t *testing.T) {
    cfg := validDatabaseConfig()
    cfg.MaxIdleConns = 7

    poolCfg, err := buildPoolConfig(cfg)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if poolCfg.MinConns != 7 {
        t.Errorf("expected MinConns=7, got %d", poolCfg.MinConns)
    }
}

func TestBuildPoolConfig_AppliesConnMaxLifetime(t *testing.T) {
    cfg := validDatabaseConfig()
    cfg.ConnMaxLifetime = 45 * time.Minute

    poolCfg, err := buildPoolConfig(cfg)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if poolCfg.MaxConnLifetime != 45*time.Minute {
        t.Errorf("expected MaxConnLifetime=45m, got %v", poolCfg.MaxConnLifetime)
    }
}

func TestBuildPoolConfig_DSN_ContainsAllFields(t *testing.T) {
    cfg := validDatabaseConfig()
    cfg.Host = "pghost"
    cfg.Port = 5433
    cfg.User = "pguser"
    cfg.Database = "pgdb"
    cfg.SSLMode = "require"

    dsn := cfg.DSN()

    for _, want := range []string{
        "host=pghost",
        "port=5433",
        "user=pguser",
        "dbname=pgdb",
        "sslmode=require",
    } {
        if !strings.Contains(dsn, want) {
            t.Errorf("DSN missing %q\ngot: %s", want, dsn)
        }
    }
}

func TestBuildPoolConfig_ZeroConnsAllowed(t *testing.T) {
    // MaxIdleConns=0 is valid — results in MinConns=0 (fully lazy pool).
    cfg := validDatabaseConfig()
    cfg.MaxIdleConns = 0

    poolCfg, err := buildPoolConfig(cfg)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if poolCfg.MinConns != 0 {
        t.Errorf("expected MinConns=0, got %d", poolCfg.MinConns)
    }
}