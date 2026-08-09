// Package repository provides persistence implementations for the
// authentication domain. Each exported type is backed by PostgreSQL.
package repository

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// UserRepository defines the persistence contract for User records.
// All methods exclude soft-deleted users unless the operation is explicitly
// targeted at them (SoftDelete, Restore).
type UserRepository interface {
	// FindByID retrieves a non-deleted user by their system-assigned UUID.
	FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.User, error)

	// FindByEmail retrieves a non-deleted user by canonical email (case-insensitive).
	FindByEmail(ctx context.Context, email string, opts ...repo.Option) (*domain.User, error)

	// FindByUsername retrieves a non-deleted user by canonical username (case-insensitive).
	FindByUsername(ctx context.Context, username string, opts ...repo.Option) (*domain.User, error)

	// FindByIdentifier resolves either an email address or a username.
	// The form is detected by the presence of '@'; used exclusively in the login flow.
	FindByIdentifier(ctx context.Context, identifier string, opts ...repo.Option) (*domain.User, error)

	// ExistsEmail reports whether an email is already registered (includes soft-deleted rows).
	ExistsEmail(ctx context.Context, email string) (bool, error)

	// ExistsUsername reports whether a username is already taken (includes soft-deleted rows).
	ExistsUsername(ctx context.Context, username string) (bool, error)

	// Create inserts a new user row and returns the record with all DB-populated fields set.
	Create(ctx context.Context, user *domain.User, opts ...repo.Option) (*domain.User, error)

	// UpdateStatus changes the lifecycle status of a non-deleted user.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UserStatus, opts ...repo.Option) error

	// UpdateEmailVerified sets the email_verified flag for a non-deleted user.
	UpdateEmailVerified(ctx context.Context, id uuid.UUID, verified bool, opts ...repo.Option) error

	// UpdatePasswordHash replaces the stored password hash for a non-deleted user.
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string, opts ...repo.Option) error

	// SoftDelete marks a user as deleted by setting deleted_at and status = 'deleted'.
	SoftDelete(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

	// Restore reverses a soft delete, assigning the appropriate active status.
	Restore(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

	// List returns a paginated slice of non-deleted users with accompanying metadata.
	List(ctx context.Context, opts ...repo.Option) ([]*domain.User, repo.PageMeta, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type userRepository struct {
	repo.Base
}

// NewUserRepository constructs a UserRepository backed by db.
func NewUserRepository(db *database.Database, log logger.Logger) UserRepository {
	return &userRepository{Base: repo.NewBase(db, log)}
}

// ─── column list (used in every SELECT) ──────────────────────────────────────

const userCols = `id, email, username, password_hash, status,
       email_verified, created_at, updated_at, deleted_at`

// ─── scan helper ─────────────────────────────────────────────────────────────

// scanUser hydrates a User from a single pgx row.
func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Username,
		&u.PasswordHash,
		(*string)(&u.Status),
		&u.EmailVerified,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.DeletedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &u, nil
}

// ─── read operations ─────────────────────────────────────────────────────────

const sqlFindByID = `
SELECT ` + userCols + `
FROM   users
WHERE  id = $1
  AND  deleted_at IS NULL`

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.User, error) {
	o := repo.NewOptions(opts...)
	return scanUser(r.Exec(o).QueryRow(ctx, sqlFindByID, id))
}

const sqlFindByEmail = `
SELECT ` + userCols + `
FROM   users
WHERE  lower(email) = lower($1)
  AND  deleted_at IS NULL`

func (r *userRepository) FindByEmail(ctx context.Context, email string, opts ...repo.Option) (*domain.User, error) {
	o := repo.NewOptions(opts...)
	return scanUser(r.Exec(o).QueryRow(ctx, sqlFindByEmail, email))
}

const sqlFindByUsername = `
SELECT ` + userCols + `
FROM   users
WHERE  lower(username) = lower($1)
  AND  deleted_at IS NULL`

func (r *userRepository) FindByUsername(ctx context.Context, username string, opts ...repo.Option) (*domain.User, error) {
	o := repo.NewOptions(opts...)
	return scanUser(r.Exec(o).QueryRow(ctx, sqlFindByUsername, username))
}

// FindByIdentifier detects the identifier form by the presence of '@'.
func (r *userRepository) FindByIdentifier(ctx context.Context, identifier string, opts ...repo.Option) (*domain.User, error) {
	if strings.ContainsRune(identifier, '@') {
		return r.FindByEmail(ctx, identifier, opts...)
	}
	return r.FindByUsername(ctx, identifier, opts...)
}

const sqlExistsEmail = `
SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1))`

func (r *userRepository) ExistsEmail(ctx context.Context, email string) (bool, error) {
	var exists bool
	if err := r.Pool().QueryRow(ctx, sqlExistsEmail, email).Scan(&exists); err != nil {
		return false, repo.MapError(err)
	}
	return exists, nil
}

const sqlExistsUsername = `
SELECT EXISTS (SELECT 1 FROM users WHERE lower(username) = lower($1))`

func (r *userRepository) ExistsUsername(ctx context.Context, username string) (bool, error) {
	var exists bool
	if err := r.Pool().QueryRow(ctx, sqlExistsUsername, username).Scan(&exists); err != nil {
		return false, repo.MapError(err)
	}
	return exists, nil
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlCreate = `
INSERT INTO users (id, email, username, password_hash, status, email_verified)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING ` + userCols

func (r *userRepository) Create(ctx context.Context, user *domain.User, opts ...repo.Option) (*domain.User, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlCreate,
		user.ID,
		user.Email,
		user.Username,
		user.PasswordHash,
		string(user.Status),
		user.EmailVerified,
	)
	return scanUser(row)
}

const sqlUpdateStatus = `
UPDATE users SET status = $2 WHERE id = $1 AND deleted_at IS NULL`

func (r *userRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UserStatus, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlUpdateStatus, id, string(status))
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlUpdateEmailVerified = `
UPDATE users SET email_verified = $2 WHERE id = $1 AND deleted_at IS NULL`

func (r *userRepository) UpdateEmailVerified(ctx context.Context, id uuid.UUID, verified bool, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlUpdateEmailVerified, id, verified)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlUpdatePasswordHash = `
UPDATE users SET password_hash = $2 WHERE id = $1 AND deleted_at IS NULL`

func (r *userRepository) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlUpdatePasswordHash, id, hash)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlSoftDelete = `
UPDATE users
SET    deleted_at = NOW(),
       status     = 'deleted'
WHERE  id = $1
  AND  deleted_at IS NULL`

func (r *userRepository) SoftDelete(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlSoftDelete, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

// sqlRestore sets deleted_at back to NULL and derives the appropriate status
// from email_verified to satisfy the domain invariant.
const sqlRestore = `
UPDATE users
SET    deleted_at = NULL,
       status     = CASE WHEN email_verified
                         THEN 'active'::user_status
                         ELSE 'pending_verification'::user_status
                    END
WHERE  id = $1
  AND  deleted_at IS NOT NULL`

func (r *userRepository) Restore(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlRestore, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

// ─── list ─────────────────────────────────────────────────────────────────────

const sqlCountUsers = `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`

const sqlListUsers = `
SELECT ` + userCols + `
FROM   users
WHERE  deleted_at IS NULL
ORDER  BY created_at DESC
LIMIT  $1 OFFSET $2`

func (r *userRepository) List(ctx context.Context, opts ...repo.Option) ([]*domain.User, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, sqlCountUsers).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlListUsers, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	users := make([]*domain.User, 0, p.PageSize)
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&u.Username,
			&u.PasswordHash,
			(*string)(&u.Status),
			&u.EmailVerified,
			&u.CreatedAt,
			&u.UpdatedAt,
			&u.DeletedAt,
		); err != nil {
			return nil, repo.PageMeta{}, repo.MapError(err)
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	return users, repo.NewPageMeta(p, total), nil
}
