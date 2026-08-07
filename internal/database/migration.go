package database

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// MigrationManager handles applying and rolling back database migrations.
type MigrationManager struct {
	m   *migrate.Migrate
	log logger.Logger
	mu  sync.Mutex
}

// NewMigrationManager creates a MigrationManager that loads SQL files from cfg.MigrationDir.
func NewMigrationManager(cfg config.DatabaseConfig, log logger.Logger) (*MigrationManager, error) {
	sourceURL := fmt.Sprintf("file://%s", cfg.MigrationDir)

	m, err := migrate.New(sourceURL, cfg.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize migration manager: %w", err)
	}

	return &MigrationManager{m: m, log: log}, nil
}

// Up applies all pending migrations.
func (mm *MigrationManager) Up(_ context.Context) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.log.Info("migration started", logger.String("direction", "up"))

	if err := mm.m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			mm.log.Info("migration skipped: no pending migrations")
			return nil
		}
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	mm.log.Info("migration completed", logger.String("direction", "up"))
	return nil
}

// Down rolls back all applied migrations.
func (mm *MigrationManager) Down(_ context.Context) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.log.Info("migration started", logger.String("direction", "down"))

	if err := mm.m.Down(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			mm.log.Info("migration skipped: nothing to roll back")
			return nil
		}
		return fmt.Errorf("failed to rollback migrations: %w", err)
	}

	mm.log.Info("migration completed", logger.String("direction", "down"))
	return nil
}

// Steps applies (positive n) or rolls back (negative n) exactly n migration steps.
func (mm *MigrationManager) Steps(_ context.Context, n int) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	direction := "up"
	if n < 0 {
		direction = "down"
	}

	mm.log.Info("migration started",
		logger.String("direction", direction),
		logger.Int("steps", n),
	)

	if err := mm.m.Steps(n); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			mm.log.Info("migration skipped: no changes for requested steps")
			return nil
		}
		return fmt.Errorf("failed to run migration steps: %w", err)
	}

	mm.log.Info("migration completed",
		logger.String("direction", direction),
		logger.Int("steps", n),
	)
	return nil
}

// Version returns the current migration version and whether the database is in a dirty state.
// Returns version 0 and no error when no migrations have been applied yet.
func (mm *MigrationManager) Version(_ context.Context) (uint, bool, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	version, dirty, err := mm.m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			mm.log.Info("current migration version: none applied")
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to determine migration version: %w", err)
	}

	mm.log.Info("current migration version",
		logger.Int("version", int(version)),
		logger.Bool("dirty", dirty),
	)

	if dirty {
		mm.log.Warn("database migration state is dirty",
			logger.Int("version", int(version)),
		)
	}

	return version, dirty, nil
}

// Force sets the migration version without running any SQL. Use to recover from a dirty state.
func (mm *MigrationManager) Force(_ context.Context, version int) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.log.Info("forcing migration version", logger.Int("version", version))

	if err := mm.m.Force(version); err != nil {
		return fmt.Errorf("failed to force migration version: %w", err)
	}

	mm.log.Info("migration version forced", logger.Int("version", version))
	return nil
}

// Close releases the source and database connections held by the manager.
func (mm *MigrationManager) Close() error {
	srcErr, dbErr := mm.m.Close()
	if srcErr != nil {
		return fmt.Errorf("failed to close migration source: %w", srcErr)
	}
	if dbErr != nil {
		return fmt.Errorf("failed to close migration database connection: %w", dbErr)
	}
	return nil
}
