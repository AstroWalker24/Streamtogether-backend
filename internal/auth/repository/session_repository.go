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

// SessionRepository defines the persistence contract for Session records.
// It does not manage refresh tokens or generate JWTs.
type SessionRepository interface {
	// FindByID retrieves a session that is neither revoked nor expired.
	// Returns ErrNotFound for sessions that are revoked, expired, or absent.
	FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Session, error)

	// FindActiveByDevice returns the most recent non-revoked, non-expired session
	// for the given device, or ErrNotFound when none exists.
	FindActiveByDevice(ctx context.Context, deviceID uuid.UUID, opts ...repo.Option) (*domain.Session, error)

	// Create inserts a new session and returns the record with DB-populated fields.
	Create(ctx context.Context, session *domain.Session, opts ...repo.Option) (*domain.Session, error)

	// UpdateLastActive refreshes last_active_at. Called on every successful token
	// refresh. Returns ErrNotFound when the session is revoked or expired.
	UpdateLastActive(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

	// Revoke marks a non-revoked session as revoked.
	// Returns ErrNotFound when the session is already revoked or does not exist.
	Revoke(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

	// RevokeAllForUser revokes all active sessions belonging to userID.
	// A no-op (not an error) when the user has no active sessions.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error

	// RevokeAllForDevice revokes all active sessions for a device.
	// A no-op (not an error) when the device has no active sessions.
	RevokeAllForDevice(ctx context.Context, deviceID uuid.UUID, opts ...repo.Option) error

	// ListActiveByUser returns paginated non-revoked, non-expired sessions for a
	// user, ordered by creation time descending.
	ListActiveByUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Session, repo.PageMeta, error)

	// ListByDevice returns all sessions for a device (active and revoked),
	// ordered by creation time descending. Used for security audit displays.
	ListByDevice(ctx context.Context, deviceID uuid.UUID, opts ...repo.Option) ([]*domain.Session, repo.PageMeta, error)

	// IsActive reports whether a session is currently non-revoked and non-expired.
	IsActive(ctx context.Context, id uuid.UUID) (bool, error)

	// PurgeExpired hard-deletes sessions that are past their expiry and not yet
	// revoked. Intended for background cleanup jobs. Returns the number of rows removed.
	PurgeExpired(ctx context.Context) (int64, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type sessionRepository struct {
	repo.Base
}

// NewSessionRepository constructs a SessionRepository backed by db.
func NewSessionRepository(db *database.Database, log logger.Logger) SessionRepository {
	return &sessionRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ──────────────────────────────────────────────────────────────

// sessionCols casts ip_address to TEXT so the domain layer stays free of
// pgx-specific INET types.
const sessionCols = `id, user_id, device_id, ip_address::TEXT, user_agent,
       created_at, last_active_at, expires_at, revoked, revoked_at, remember_me`

// ─── scan helper ──────────────────────────────────────────────────────────────

func scanSession(row pgx.Row) (*domain.Session, error) {
	var s domain.Session
	err := row.Scan(
		&s.ID,
		&s.UserID,
		&s.DeviceID,
		&s.IPAddress,
		&s.UserAgent,
		&s.CreatedAt,
		&s.LastActiveAt,
		&s.ExpiresAt,
		&s.Revoked,
		&s.RevokedAt,
		&s.RememberMe,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &s, nil
}

// ─── read operations ──────────────────────────────────────────────────────────

const sqlSessionFindByID = `
SELECT ` + sessionCols + `
FROM   sessions
WHERE  id         = $1
  AND  revoked    = FALSE
  AND  expires_at > NOW()`

func (r *sessionRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Session, error) {
	o := repo.NewOptions(opts...)
	return scanSession(r.Exec(o).QueryRow(ctx, sqlSessionFindByID, id))
}

// sqlSessionFindActiveByDevice returns the most recent active session for a device.
const sqlSessionFindActiveByDevice = `
SELECT ` + sessionCols + `
FROM   sessions
WHERE  device_id  = $1
  AND  revoked    = FALSE
  AND  expires_at > NOW()
ORDER  BY created_at DESC
LIMIT  1`

func (r *sessionRepository) FindActiveByDevice(ctx context.Context, deviceID uuid.UUID, opts ...repo.Option) (*domain.Session, error) {
	o := repo.NewOptions(opts...)
	return scanSession(r.Exec(o).QueryRow(ctx, sqlSessionFindActiveByDevice, deviceID))
}

const sqlSessionIsActive = `
SELECT EXISTS (
    SELECT 1 FROM sessions
    WHERE  id         = $1
      AND  revoked    = FALSE
      AND  expires_at > NOW()
)`

func (r *sessionRepository) IsActive(ctx context.Context, id uuid.UUID) (bool, error) {
	var active bool
	if err := r.Pool().QueryRow(ctx, sqlSessionIsActive, id).Scan(&active); err != nil {
		return false, repo.MapError(err)
	}
	return active, nil
}

// sqlCountActiveSessions is used by ListActiveByUser.
const sqlCountActiveSessions = `
SELECT COUNT(*)
FROM   sessions
WHERE  user_id    = $1
  AND  revoked    = FALSE
  AND  expires_at > NOW()`

// sqlListActiveSessions uses idx_sessions_user_id_active on
// (user_id, created_at DESC) WHERE revoked = FALSE.
const sqlListActiveSessions = `
SELECT ` + sessionCols + `
FROM   sessions
WHERE  user_id    = $1
  AND  revoked    = FALSE
  AND  expires_at > NOW()
ORDER  BY created_at DESC
LIMIT  $2 OFFSET $3`

func (r *sessionRepository) ListActiveByUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Session, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, sqlCountActiveSessions, userID).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlListActiveSessions, userID, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	sessions := make([]*domain.Session, 0, p.PageSize)
	for rows.Next() {
		var s domain.Session
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.DeviceID, &s.IPAddress, &s.UserAgent,
			&s.CreatedAt, &s.LastActiveAt, &s.ExpiresAt,
			&s.Revoked, &s.RevokedAt, &s.RememberMe,
		); err != nil {
			return nil, repo.PageMeta{}, repo.MapError(err)
		}
		sessions = append(sessions, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	return sessions, repo.NewPageMeta(p, total), nil
}

const sqlCountSessionsByDevice = `SELECT COUNT(*) FROM sessions WHERE device_id = $1`

// sqlListSessionsByDevice uses idx_sessions_device_id. Returns all sessions
// (active and revoked) for security audit displays.
const sqlListSessionsByDevice = `
SELECT ` + sessionCols + `
FROM   sessions
WHERE  device_id = $1
ORDER  BY created_at DESC
LIMIT  $2 OFFSET $3`

func (r *sessionRepository) ListByDevice(ctx context.Context, deviceID uuid.UUID, opts ...repo.Option) ([]*domain.Session, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, sqlCountSessionsByDevice, deviceID).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlListSessionsByDevice, deviceID, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	sessions := make([]*domain.Session, 0, p.PageSize)
	for rows.Next() {
		var s domain.Session
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.DeviceID, &s.IPAddress, &s.UserAgent,
			&s.CreatedAt, &s.LastActiveAt, &s.ExpiresAt,
			&s.Revoked, &s.RevokedAt, &s.RememberMe,
		); err != nil {
			return nil, repo.PageMeta{}, repo.MapError(err)
		}
		sessions = append(sessions, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	return sessions, repo.NewPageMeta(p, total), nil
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlSessionCreate = `
INSERT INTO sessions (id, user_id, device_id, ip_address, user_agent, expires_at, remember_me)
VALUES ($1, $2, $3, $4::INET, $5, $6, $7)
RETURNING ` + sessionCols

func (r *sessionRepository) Create(ctx context.Context, session *domain.Session, opts ...repo.Option) (*domain.Session, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlSessionCreate,
		session.ID,
		session.UserID,
		session.DeviceID,
		session.IPAddress,
		session.UserAgent,
		session.ExpiresAt,
		session.RememberMe,
	)
	return scanSession(row)
}

const sqlSessionUpdateLastActive = `
UPDATE sessions
SET    last_active_at = NOW()
WHERE  id         = $1
  AND  revoked    = FALSE
  AND  expires_at > NOW()`

func (r *sessionRepository) UpdateLastActive(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlSessionUpdateLastActive, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlSessionRevoke = `
UPDATE sessions
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  id      = $1
  AND  revoked = FALSE`

func (r *sessionRepository) Revoke(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlSessionRevoke, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlSessionRevokeAllForUser = `
UPDATE sessions
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  user_id = $1
  AND  revoked = FALSE`

func (r *sessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlSessionRevokeAllForUser, userID)
	return repo.MapError(err)
}

const sqlSessionRevokeAllForDevice = `
UPDATE sessions
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  device_id = $1
  AND  revoked   = FALSE`

func (r *sessionRepository) RevokeAllForDevice(ctx context.Context, deviceID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlSessionRevokeAllForDevice, deviceID)
	return repo.MapError(err)
}

// sqlSessionPurgeExpired uses idx_sessions_expires_at on (expires_at)
// WHERE revoked = FALSE.
const sqlSessionPurgeExpired = `
DELETE FROM sessions
WHERE  expires_at <= NOW()
  AND  revoked    = FALSE`

func (r *sessionRepository) PurgeExpired(ctx context.Context) (int64, error) {
	tag, err := r.Pool().Exec(ctx, sqlSessionPurgeExpired)
	if err != nil {
		return 0, repo.MapError(err)
	}
	return tag.RowsAffected(), nil
}
