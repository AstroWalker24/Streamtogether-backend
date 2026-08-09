// Package repository provides the foundational database access layer.
// All domain-specific repositories build upon this package.
package repository

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinel errors returned by all repositories.
var (
	// ErrNotFound is returned when a queried entity does not exist.
	ErrNotFound = errors.New("entity not found")

	// ErrDuplicateKey is returned on a unique constraint violation.
	ErrDuplicateKey = errors.New("duplicate key violation")

	// ErrConstraintViolation is returned on a foreign-key, not-null, or check violation.
	ErrConstraintViolation = errors.New("constraint violation")

	// ErrTransactionFailed is returned when a transaction cannot be started, committed, or rolled back.
	ErrTransactionFailed = errors.New("transaction failed")

	// ErrDatabaseUnavailable is returned when the database cannot be reached.
	ErrDatabaseUnavailable = errors.New("database unavailable")
)

// PostgreSQL error class codes used for mapping.
const (
	pgErrUniqueViolation     = "23505"
	pgErrForeignKeyViolation = "23503"
	pgErrNotNullViolation    = "23502"
	pgErrCheckViolation      = "23514"
)

// MapError translates a low-level pgx driver error into a repository sentinel error.
// If the error is already a sentinel, it is returned unchanged.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgErrUniqueViolation:
			return fmt.Errorf("%w: %s", ErrDuplicateKey, pgErr.Detail)
		case pgErrForeignKeyViolation, pgErrNotNullViolation, pgErrCheckViolation:
			return fmt.Errorf("%w: %s", ErrConstraintViolation, pgErr.Detail)
		}

		// Connection-class errors (08xxx) and crash recovery (57P01).
		if len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08" || pgErr.Code == "57P01" {
			return fmt.Errorf("%w: %s", ErrDatabaseUnavailable, pgErr.Message)
		}
	}

	return err
}
