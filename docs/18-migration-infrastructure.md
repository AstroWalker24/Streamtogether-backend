# 18 — Migration Infrastructure

## Overview

The migration subsystem lives entirely inside `internal/database`. It wraps [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate) and exposes a `MigrationManager` that applies, rolls back, versions, and force-recovers database schemas. Migrations are never run automatically on startup — they are invoked explicitly from the CLI, Makefile targets, or integration tests.

---

## Package Structure

```
internal/database/
├── migration.go                        — MigrationManager struct and all methods
└── migrations/
    └── postgres/                       — SQL migration files (*.up.sql / *.down.sql)
```

```
internal/database/seeds/               — Seed directory (populated in a future phase)
    development/
    test/
```

---

## Design Principles

| Principle | How it is applied |
|---|---|
| **No auto-run** | `MigrationManager` is never called from `bootstrap.go`; startup only connects the pool |
| **Constructor injection** | Takes `config.DatabaseConfig` and `logger.Logger` — no globals, no singletons |
| **Separate connection** | golang-migrate opens its own `database/sql` connection; the pgxpool is unaffected |
| **Thread safety** | A `sync.Mutex` prevents accidental parallel execution of migration steps |
| **Dirty-state awareness** | `Version()` warns on dirty state; `Force()` provides the recovery path |
| **Context-forwarded API** | Every method accepts `context.Context` for future cancellation support |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/golang-migrate/migrate/v4` | Core migration engine |
| `github.com/golang-migrate/migrate/v4/database/postgres` | PostgreSQL driver adapter (blank import) |
| `github.com/golang-migrate/migrate/v4/source/file` | File-system source adapter (blank import) |

Install:
```bash
go get github.com/golang-migrate/migrate/v4 \
       github.com/golang-migrate/migrate/v4/database/postgres \
       github.com/golang-migrate/migrate/v4/source/file
```

---

## Public API

```go
// Construct — opens a dedicated migration DB connection.
func NewMigrationManager(cfg config.DatabaseConfig, log logger.Logger) (*MigrationManager, error)

// Apply all pending migrations.
func (mm *MigrationManager) Up(ctx context.Context) error

// Roll back all applied migrations.
func (mm *MigrationManager) Down(ctx context.Context) error

// Apply or roll back exactly n steps (positive = up, negative = down).
func (mm *MigrationManager) Steps(ctx context.Context, n int) error

// Return the current version and dirty flag. Returns (0, false, nil) when no migration has run yet.
func (mm *MigrationManager) Version(ctx context.Context) (uint, bool, error)

// Set the recorded version without executing SQL. Use to recover from a dirty state.
func (mm *MigrationManager) Force(ctx context.Context, version int) error

// Release source and database connections.
func (mm *MigrationManager) Close() error
```

---

## Configuration

Migration config is part of `config.DatabaseConfig`.

| Field | Env var | Default |
|---|---|---|
| `MigrationDir` | `POSTGRES_MIGRATION_DIR` | `internal/database/migrations/postgres` |

The field is populated by the standard `config.Load()` path — no separate config file is needed.

`NewMigrationManager` builds the source URL as:
```
file://<cfg.MigrationDir>
```

The database URL is constructed by `cfg.URL()` (added to `config.DatabaseConfig`):
```
postgres://user:password@host:port/dbname?sslmode=<value>
```

`url.UserPassword` is used internally so special characters in passwords are safely percent-encoded.

---

## Migration File Naming

All SQL files live in `internal/database/migrations/postgres/` and follow the golang-migrate convention:

```
000001_create_users.up.sql
000001_create_users.down.sql
000002_create_sessions.up.sql
000002_create_sessions.down.sql
```

Rules:
- Version prefix is a zero-padded 6-digit integer.
- Each version **must** have both an `up` and a `down` file.
- Files are applied in ascending version order.

---

## Dirty State

A migration is marked **dirty** when a previous run failed mid-execution. golang-migrate sets the dirty flag in the `schema_migrations` table and refuses to run further migrations until the issue is resolved.

Recovery workflow:

```
1. Inspect the database to understand what partially ran.
2. Manually undo any partial changes if necessary.
3. Call Force(ctx, version) to clear the dirty flag at the target version.
4. Call Up(ctx) to resume from that point.
```

`Version()` logs a warning whenever it detects a dirty state:
```
WARN  database migration state is dirty  version=3
```

`Force()` does not execute any SQL — it only updates the version record.

---

## Error Handling

All errors are wrapped with context so call-site logs carry the full cause chain.

| Situation | Error message |
|---|---|
| Bad source path or DB URL | `failed to initialize migration manager: <cause>` |
| `Up` fails mid-run | `failed to apply migrations: <cause>` |
| `Down` fails mid-run | `failed to rollback migrations: <cause>` |
| `Steps` fails | `failed to run migration steps: <cause>` |
| `Version` DB error | `failed to determine migration version: <cause>` |
| `Force` DB error | `failed to force migration version: <cause>` |
| `Close` source error | `failed to close migration source: <cause>` |
| `Close` DB error | `failed to close migration database connection: <cause>` |

`migrate.ErrNoChange` is not surfaced as an error — it is silently logged as "migration skipped".

---

## Logging

| Event | Level | Fields |
|---|---|---|
| Migration started | `INFO` | `direction`, `steps` (Steps only) |
| Migration completed | `INFO` | `direction`, `steps` (Steps only) |
| Migration skipped (no change) | `INFO` | — |
| Current version queried | `INFO` | `version`, `dirty` |
| No version applied yet | `INFO` | — |
| Dirty state detected | `WARN` | `version` |
| Version forced | `INFO` | `version` |

---

## Thread Safety

`sync.Mutex` is held for the duration of every method call. This prevents two goroutines from running `Up` and `Down` simultaneously at the application level. golang-migrate additionally acquires a PostgreSQL advisory lock, providing a second safety layer when multiple process instances share the same database.

---

## Typical Usage Patterns

**From a Makefile target:**
```bash
# apply all pending migrations
go run ./cmd/migrate up

# roll back one step
go run ./cmd/migrate steps -- -1
```

**From an integration test:**
```go
func TestWithMigrations(t *testing.T) {
    mm, err := database.NewMigrationManager(cfg.Database, log)
    require.NoError(t, err)
    t.Cleanup(func() { mm.Down(ctx); mm.Close() })

    require.NoError(t, mm.Up(ctx))
    // run test against a fully migrated schema
}
```

**Dirty state recovery:**
```go
mm, _ := database.NewMigrationManager(cfg.Database, log)
defer mm.Close()

version, dirty, _ := mm.Version(ctx)
if dirty {
    mm.Force(ctx, int(version)-1)  // step back to last clean version
    mm.Up(ctx)                     // re-apply the failed migration
}
```

---

## Seed Architecture

`internal/database/seeds/` is reserved for environment-scoped seed data. No execution logic exists in Phase 1. Subdirectories are organised by environment:

```
internal/database/seeds/
├── development/    — fixtures for local development
└── test/           — fixtures for integration tests
```

Seed execution will be added in a later phase alongside a `SeedManager` following the same constructor-injection pattern as `MigrationManager`.
