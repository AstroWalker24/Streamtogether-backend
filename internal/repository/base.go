package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// Base is the common foundation embedded by every repository.
// It provides access to the database pool, a scoped logger, and helpers
// for transaction-aware query execution.
type Base struct {
	db  *database.Database
	log logger.Logger
}

// NewBase constructs a Base with the provided database and logger.
func NewBase(db *database.Database, log logger.Logger) Base {
	return Base{db: db, log: log}
}

// DB returns the database connection manager.
func (b *Base) DB() *database.Database {
	return b.db
}

// Log returns the scoped logger.
func (b *Base) Log() logger.Logger {
	return b.log
}

// Pool returns the raw pgx connection pool.
func (b *Base) Pool() *pgxpool.Pool {
	return b.db.Pool()
}

// Exec returns the active transaction from opts when one is present, or
// falls back to the connection pool, so callers can use a single interface
// for all queries regardless of transaction state.
func (b *Base) Exec(opts Options) Executor {
	if tx := opts.TX(); tx != nil {
		return tx
	}
	return b.db.Pool()
}

// RunInTx executes fn within a new database transaction, mapping driver
// errors to repository sentinels on failure.
func (b *Base) RunInTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return RunInTransaction(ctx, b.db.Pool(), fn)
}
