// Package repository provides the PostgreSQL-backed persistence implementation
// for the profile domain. All SQL is encapsulated here; callers interact
// only with the ProfileRepository interface.
package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/AstroWalker24/Streamtogether-backend/internal/profile/domain"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// ProfileRepository defines the persistence contract for Profile records.
type ProfileRepository interface {
	// Create inserts a new profile row and returns the record with all
	// DB-populated fields set. Returns repo.ErrDuplicateKey if a profile
	// already exists for profile.UserID.
	Create(ctx context.Context, profile *domain.Profile, opts ...repo.Option) (*domain.Profile, error)

	// FindByID retrieves a profile by its surrogate UUID.
	// Returns repo.ErrNotFound when no matching row exists.
	FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Profile, error)

	// FindByUserID retrieves a profile by the owning user's UUID.
	// This is the primary access pattern. Returns repo.ErrNotFound when
	// no profile exists for the given user.
	FindByUserID(ctx context.Context, userID uuid.UUID, opts ...repo.Option) (*domain.Profile, error)

	// Update replaces the mutable presentation fields (display_name, bio,
	// avatar_url) of the profile identified by profile.ID.
	// user_id and created_at are never mutated. updated_at is maintained
	// by the database trigger. Returns repo.ErrNotFound when no row matches.
	Update(ctx context.Context, profile *domain.Profile, opts ...repo.Option) (*domain.Profile, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type profileRepository struct {
	repo.Base
}

// NewProfileRepository constructs a ProfileRepository backed by db.
func NewProfileRepository(db *database.Database, log logger.Logger) ProfileRepository {
	return &profileRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ─────────────────────────────────────────────────────────────

const profileCols = `id, user_id, display_name, bio, avatar_url, created_at, updated_at`

// ─── scan helper ─────────────────────────────────────────────────────────────

func scanProfile(row pgx.Row) (*domain.Profile, error) {
	var p domain.Profile
	err := row.Scan(
		&p.ID,
		&p.UserID,
		&p.DisplayName,
		&p.Bio,
		&p.AvatarURL,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &p, nil
}

// ─── read operations ─────────────────────────────────────────────────────────

const sqlProfileFindByID = `
SELECT ` + profileCols + `
FROM   profiles
WHERE  id = $1`

func (r *profileRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Profile, error) {
	o := repo.NewOptions(opts...)
	return scanProfile(r.Exec(o).QueryRow(ctx, sqlProfileFindByID, id))
}

const sqlProfileFindByUserID = `
SELECT ` + profileCols + `
FROM   profiles
WHERE  user_id = $1`

func (r *profileRepository) FindByUserID(ctx context.Context, userID uuid.UUID, opts ...repo.Option) (*domain.Profile, error) {
	o := repo.NewOptions(opts...)
	return scanProfile(r.Exec(o).QueryRow(ctx, sqlProfileFindByUserID, userID))
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlProfileCreate = `
INSERT INTO profiles (id, user_id, display_name, bio, avatar_url)
VALUES ($1, $2, $3, $4, $5)
RETURNING ` + profileCols

func (r *profileRepository) Create(ctx context.Context, profile *domain.Profile, opts ...repo.Option) (*domain.Profile, error) {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlProfileCreate,
		profile.ID,
		profile.UserID,
		profile.DisplayName,
		profile.Bio,
		profile.AvatarURL,
	)
	return scanProfile(row)
}

// sqlProfileUpdate writes only the user-editable presentation fields.
// user_id and created_at are immutable and are never written here.
// updated_at is maintained by trg_profiles_updated_at.
const sqlProfileUpdate = `
UPDATE profiles
SET    display_name = $2,
       bio          = $3,
       avatar_url   = $4
WHERE  id = $1
RETURNING ` + profileCols

func (r *profileRepository) Update(ctx context.Context, profile *domain.Profile, opts ...repo.Option) (*domain.Profile, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlProfileUpdate,
		profile.ID,
		profile.DisplayName,
		profile.Bio,
		profile.AvatarURL,
	)
	return scanProfile(row)
}
