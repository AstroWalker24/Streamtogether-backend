package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Executor is the minimal query interface satisfied by both *pgxpool.Pool and
// pgx.Tx, allowing repositories to execute queries without branching on
// whether a transaction is active.
type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Lister is a generic interface for repository list operations.
type Lister[T any] interface {
	List(ctx context.Context, opts ...Option) ([]T, PageMeta, error)
}

// Finder is a generic interface for single-entity lookups by ID.
type Finder[T any, ID any] interface {
	FindByID(ctx context.Context, id ID, opts ...Option) (T, error)
}

// Creator is a generic interface for entity creation.
type Creator[T any] interface {
	Create(ctx context.Context, entity T, opts ...Option) (T, error)
}

// Updater is a generic interface for entity updates.
type Updater[T any] interface {
	Update(ctx context.Context, entity T, opts ...Option) (T, error)
}

// Deleter is a generic interface for entity deletion by ID.
type Deleter[ID any] interface {
	Delete(ctx context.Context, id ID, opts ...Option) error
}
