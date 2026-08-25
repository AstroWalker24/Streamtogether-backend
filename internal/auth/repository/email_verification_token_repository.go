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

// EmailVerificationTokenRepository defines the persistence contract for
// email verification tokens (§4.7).
type EmailVerificationTokenRepository interface {
	// FindActiveByHash retrieves an unconsumed, unexpired token by its SHA-256 hash.
	FindActiveByHash(ctx context.Context, tokenHash string, opts ...repo.Option) (*domain.EmailVerificationToken, error)

	// Create inserts a new token record and returns it with DB-populated fields.
	Create(ctx context.Context, token *domain.EmailVerificationToken, opts ...repo.Option) (*domain.EmailVerificationToken, error)

	// InvalidateAllForUser marks all unconsumed tokens for userID as used (INV-15).
	// A no-op when the user has no active tokens.
	InvalidateAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error

	// MarkUsed records the time the token was consumed, preventing reuse.
	// Returns ErrNotFound when the token is already consumed or does not exist.
	MarkUsed(ctx context.Context, id uuid.UUID, opts ...repo.Option) error
}

type emailVerificationTokenRepository struct {
	repo.Base
}

// NewEmailVerificationTokenRepository constructs an EmailVerificationTokenRepository backed by db.
func NewEmailVerificationTokenRepository(db *database.Database, log logger.Logger) EmailVerificationTokenRepository {
	return &emailVerificationTokenRepository{Base: repo.NewBase(db, log)}
}

const evtCols = `id, user_id, token_hash, expires_at, used_at, created_at`

func scanEVT(row pgx.Row) (*domain.EmailVerificationToken, error) {
	var t domain.EmailVerificationToken
	err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &t, nil
}

const sqlEVTFindActiveByHash = `
SELECT ` + evtCols + `
FROM   email_verification_tokens
WHERE  token_hash = $1
  AND  used_at    IS NULL
  AND  expires_at > NOW()`

func (r *emailVerificationTokenRepository) FindActiveByHash(ctx context.Context, tokenHash string, opts ...repo.Option) (*domain.EmailVerificationToken, error) {
	o := repo.NewOptions(opts...)
	return scanEVT(r.Exec(o).QueryRow(ctx, sqlEVTFindActiveByHash, tokenHash))
}

const sqlEVTCreate = `
INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING ` + evtCols

func (r *emailVerificationTokenRepository) Create(ctx context.Context, token *domain.EmailVerificationToken, opts ...repo.Option) (*domain.EmailVerificationToken, error) {
	o := repo.NewOptions(opts...)
	return scanEVT(r.Exec(o).QueryRow(ctx, sqlEVTCreate, token.ID, token.UserID, token.TokenHash, token.ExpiresAt))
}

// sqlEVTInvalidateAllForUser targets idx_email_verification_user_id (partial on used_at IS NULL).
const sqlEVTInvalidateAllForUser = `
UPDATE email_verification_tokens
SET    used_at = NOW()
WHERE  user_id = $1
  AND  used_at IS NULL`

func (r *emailVerificationTokenRepository) InvalidateAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlEVTInvalidateAllForUser, userID)
	return repo.MapError(err)
}

const sqlEVTMarkUsed = `
UPDATE email_verification_tokens
SET    used_at = NOW()
WHERE  id      = $1
  AND  used_at IS NULL`

func (r *emailVerificationTokenRepository) MarkUsed(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	tag, err := r.Exec(o).Exec(ctx, sqlEVTMarkUsed, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		// Token was already consumed by a concurrent request.
		return repo.ErrNotFound
	}
	return nil
}
