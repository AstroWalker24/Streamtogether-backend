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

// OAuthIdentityRepository defines the persistence contract for OAuth identity links.
type OAuthIdentityRepository interface {
	// FindByProvider retrieves an identity link for the given (provider, providerUserID) pair.
	// Returns repo.ErrNotFound if no matching record exists.
	FindByProvider(ctx context.Context, provider, providerUserID string, opts ...repo.Option) (*domain.OAuthIdentity, error)

	// Create persists a new identity link and returns the record with DB-populated fields.
	Create(ctx context.Context, identity *domain.OAuthIdentity, opts ...repo.Option) (*domain.OAuthIdentity, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type oauthIdentityRepository struct {
	repo.Base
}

// NewOAuthIdentityRepository constructs an OAuthIdentityRepository backed by db.
func NewOAuthIdentityRepository(db *database.Database, log logger.Logger) OAuthIdentityRepository {
	return &oauthIdentityRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ─────────────────────────────────────────────────────────────

const oauthIdentityCols = `id, user_id, provider, provider_user_id, created_at, updated_at`

// ─── scan helper ─────────────────────────────────────────────────────────────

func scanOAuthIdentity(row pgx.Row) (*domain.OAuthIdentity, error) {
	var oi domain.OAuthIdentity
	err := row.Scan(
		&oi.ID,
		&oi.UserID,
		&oi.Provider,
		&oi.ProviderUserID,
		&oi.CreatedAt,
		&oi.UpdatedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &oi, nil
}

// ─── read operations ─────────────────────────────────────────────────────────

const sqlFindOAuthIdentityByProvider = `
SELECT ` + oauthIdentityCols + `
FROM   oauth_identities
WHERE  provider = $1
  AND  provider_user_id = $2`

func (r *oauthIdentityRepository) FindByProvider(ctx context.Context, provider, providerUserID string, opts ...repo.Option) (*domain.OAuthIdentity, error) {
	o := repo.NewOptions(opts...)
	return scanOAuthIdentity(r.Exec(o).QueryRow(ctx, sqlFindOAuthIdentityByProvider, provider, providerUserID))
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlCreateOAuthIdentity = `
INSERT INTO oauth_identities (id, user_id, provider, provider_user_id)
VALUES ($1, $2, $3, $4)
RETURNING ` + oauthIdentityCols

func (r *oauthIdentityRepository) Create(ctx context.Context, identity *domain.OAuthIdentity, opts ...repo.Option) (*domain.OAuthIdentity, error) {
	if identity.ID == uuid.Nil {
		identity.ID = uuid.New()
	}
	o := repo.NewOptions(opts...)
	return scanOAuthIdentity(r.Exec(o).QueryRow(
		ctx,
		sqlCreateOAuthIdentity,
		identity.ID,
		identity.UserID,
		identity.Provider,
		identity.ProviderUserID,
	))
}
