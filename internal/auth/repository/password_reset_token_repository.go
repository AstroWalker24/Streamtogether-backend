package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// PasswordResetTokenRepository defines the persistence contract for
// password reset tokens (§4.8).
type PasswordResetTokenRepository interface {
	// FindActiveByHash retrieves an unconsumed, unexpired token by its SHA-256 hash.
	FindActiveByHash(ctx context.Context, tokenHash string, opts ...repo.Option) (*domain.PasswordResetToken, error)

	// Create inserts a new token record and returns it with DB-populated fields.
	Create(ctx context.Context, token *domain.PasswordResetToken, opts ...repo.Option) (*domain.PasswordResetToken, error)

	// InvalidateAllForUser marks all unconsumed tokens for userID as used (INV-16).
	// A no-op when the user has no active tokens.
	InvalidateAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error

	// MarkUsed records the time the token was consumed, preventing reuse.
	// Returns ErrNotFound when the token is already consumed or does not exist.
	MarkUsed(ctx context.Context, id uuid.UUID, opts ...repo.Option) error
}

type passwordResetTokenRepository struct {
	repo.Base
}

// NewPasswordResetTokenRepository constructs a PasswordResetTokenRepository backed by db.
func NewPasswordResetTokenRepository(db *database.Database, log logger.Logger) PasswordResetTokenRepository {
	return &passwordResetTokenRepository{Base: repo.NewBase(db, log)}
}

const prtCols = `id, user_id, token_hash, expires_at, used_at, created_at`

func scanPRT(row pgx.Row) (*domain.PasswordResetToken, error) {
	var t domain.PasswordResetToken
	err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &t, nil
}

const sqlPRTFindActiveByHash = `
SELECT ` + prtCols + `
FROM   password_reset_tokens
WHERE  token_hash = $1
  AND  used_at    IS NULL
  AND  expires_at > NOW()`

func (r *passwordResetTokenRepository) FindActiveByHash(ctx context.Context, tokenHash string, opts ...repo.Option) (*domain.PasswordResetToken, error) {
	o := repo.NewOptions(opts...)
	return scanPRT(r.Exec(o).QueryRow(ctx, sqlPRTFindActiveByHash, tokenHash))
}

const sqlPRTCreate = `
INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING ` + prtCols

func (r *passwordResetTokenRepository) Create(ctx context.Context, token *domain.PasswordResetToken, opts ...repo.Option) (*domain.PasswordResetToken, error) {
	o := repo.NewOptions(opts...)
	return scanPRT(r.Exec(o).QueryRow(ctx, sqlPRTCreate, token.ID, token.UserID, token.TokenHash, token.ExpiresAt))
}

// sqlPRTInvalidateAllForUser targets idx_password_reset_user_id (partial on used_at IS NULL).
const sqlPRTInvalidateAllForUser = `
UPDATE password_reset_tokens
SET    used_at = NOW()
WHERE  user_id = $1
  AND  used_at IS NULL`

func (r *passwordResetTokenRepository) InvalidateAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlPRTInvalidateAllForUser, userID)
	return repo.MapError(err)
}

const sqlPRTMarkUsed = `
UPDATE password_reset_tokens
SET    used_at = NOW()
WHERE  id      = $1
  AND  used_at IS NULL`

func (r *passwordResetTokenRepository) MarkUsed(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	tag, err := r.Exec(o).Exec(ctx, sqlPRTMarkUsed, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}
