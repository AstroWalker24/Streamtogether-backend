package repository

import "github.com/jackc/pgx/v5"

// Options carries optional query-time parameters for any repository operation.
// All fields are private; callers configure them through functional Option values.
type Options struct {
	tx            pgx.Tx
	pagination    *Pagination
	filters       *Filters
	lockForUpdate bool
}

// Option is a functional option that configures an Options value.
type Option func(*Options)

// WithTransaction instructs the repository to execute the operation inside tx
// instead of acquiring a new connection from the pool.
func WithTransaction(tx pgx.Tx) Option {
	return func(o *Options) { o.tx = tx }
}

// WithPagination applies p to the operation.
func WithPagination(p Pagination) Option {
	return func(o *Options) { o.pagination = &p }
}

// WithFilters applies f to the operation.
func WithFilters(f Filters) Option {
	return func(o *Options) { o.filters = &f }
}

// WithLock adds a SELECT FOR UPDATE clause to the query.
func WithLock() Option {
	return func(o *Options) { o.lockForUpdate = true }
}

// NewOptions builds an Options from zero or more functional options.
func NewOptions(opts ...Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// TX returns the active transaction, or nil when none was provided.
func (o Options) TX() pgx.Tx {
	return o.tx
}

// Pagination returns the pagination parameters, or nil when not set.
func (o Options) Pagination() *Pagination {
	return o.pagination
}

// Filters returns the filter parameters, or nil when not set.
func (o Options) Filters() *Filters {
	return o.filters
}

// LockForUpdate reports whether a SELECT FOR UPDATE clause should be emitted.
func (o Options) LockForUpdate() bool {
	return o.lockForUpdate
}
