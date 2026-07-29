package config

import (
    "strings"
    "testing"
)

func TestDatabaseConfig_DSN(t *testing.T) {
    db := DatabaseConfig{
        Host:     "localhost",
        Port:     5432,
        User:     "admin",
        Password: "secret",
        Database: "mydb",
        SSLMode:  "disable",
    }

    dsn := db.DSN()

    cases := []string{"host=localhost", "port=5432", "user=admin", "password=secret", "dbname=mydb", "sslmode=disable"}
    for _, want := range cases {
        if !strings.Contains(dsn, want) {
            t.Errorf("DSN missing %q\ngot: %s", want, dsn)
        }
    }
}

func TestRedisConfig_Address(t *testing.T) {
    r := RedisConfig{Host: "redis-host", Port: 6380}
    if got := r.Address(); got != "redis-host:6380" {
        t.Errorf("expected redis-host:6380, got %s", got)
    }
}

func TestAppConfig_Address(t *testing.T) {
    a := AppConfig{Host: "0.0.0.0", Port: 9090}
    if got := a.Address(); got != "0.0.0.0:9090" {
        t.Errorf("expected 0.0.0.0:9090, got %s", got)
    }
}

func TestDatabaseConfig_DSN_SpecialCharsInPassword(t *testing.T) {
    // Password with spaces and symbols — DSN format does not URL-encode,
    // ensure the raw value is passed through unchanged.
    db := DatabaseConfig{
        Host: "localhost", Port: 5432,
        User: "u", Password: "p@$$w0rd!", Database: "db", SSLMode: "disable",
    }
    if !strings.Contains(db.DSN(), "password=p@$$w0rd!") {
        t.Errorf("DSN did not preserve special characters in password: %s", db.DSN())
    }
}