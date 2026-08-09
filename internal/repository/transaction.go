package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunInTransaction executes fn within a database transaction obtained from pool.
// If fn returns an error the transaction is rolled back; otherwise it is committed.
// All errors are wrapped with ErrTransactionFailed.
func RunInTransaction(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: begin: %w", ErrTransactionFailed, err)
	}

	if fnErr := fn(tx); fnErr != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("%w: rollback after (%w): %w", ErrTransactionFailed, fnErr, rbErr)
		}
		return fnErr
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: commit: %w", ErrTransactionFailed, err)
	}

	return nil
}
