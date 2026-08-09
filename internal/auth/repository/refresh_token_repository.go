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

// RefreshTokenRepository defines the persistence contract for RefreshToken records.
// It does not generate, hash, or validate tokens — those are service responsibilities.
type RefreshTokenRepository interface {
    // FindByID retrieves a refresh token by UUID regardless of its state.
    // Used for audit and rotation-chain inspection.
    FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.RefreshToken, error)

    // FindActiveByHash retrieves a token that is unconsumed, unrevoked, and unexpired
    // by its SHA-256 hash. This is the primary lookup in the token refresh hot path.
    // Returns ErrNotFound for consumed, revoked, expired, or absent tokens.
    FindActiveByHash(ctx context.Context, tokenHash string, opts ...repo.Option) (*domain.RefreshToken, error)

    // FindActiveBySession returns the single unconsumed, unrevoked, unexpired token
    // for a session, or ErrNotFound when none exists.
    FindActiveBySession(ctx context.Context, sessionID uuid.UUID, opts ...repo.Option) (*domain.RefreshToken, error)

    // Create inserts a new refresh token record and returns it with DB-populated fields.
    Create(ctx context.Context, token *domain.RefreshToken, opts ...repo.Option) (*domain.RefreshToken, error)

    // MarkConsumed sets consumed_at and replaced_by_id atomically.
    // Returns ErrNotFound when the token is already consumed, revoked, or absent.
    MarkConsumed(ctx context.Context, id uuid.UUID, replacedByID uuid.UUID, opts ...repo.Option) error

    // Revoke marks a single token as revoked.
    // Returns ErrNotFound when the token is already revoked or does not exist.
    Revoke(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

    // RevokeAllForSession revokes all unrevoked tokens for a session.
    // A no-op (not an error) when the session has no unrevoked tokens.
    RevokeAllForSession(ctx context.Context, sessionID uuid.UUID, opts ...repo.Option) error

    // RevokeAllForUser revokes all unrevoked tokens for a user across all sessions.
    // Used in the high-urgency password-change and logout-all flows.
    // A no-op (not an error) when the user has no unrevoked tokens.
    RevokeAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error

    // ListBySession returns a paginated, issue-time-descending list of all tokens
    // for a session (all states). Used for security audit views.
    ListBySession(ctx context.Context, sessionID uuid.UUID, opts ...repo.Option) ([]*domain.RefreshToken, repo.PageMeta, error)

    // PurgeStale hard-deletes tokens that are both eligible for cleanup
    // (consumed or revoked) and past their expiry. Intended for background jobs.
    // Returns the number of rows removed.
    PurgeStale(ctx context.Context) (int64, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type refreshTokenRepository struct {
    repo.Base
}

// NewRefreshTokenRepository constructs a RefreshTokenRepository backed by db.
func NewRefreshTokenRepository(db *database.Database, log logger.Logger) RefreshTokenRepository {
    return &refreshTokenRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ──────────────────────────────────────────────────────────────

const rtCols = `id, session_id, user_id, device_id, token_hash,
       issued_at, expires_at, consumed_at, revoked, revoked_at, replaced_by_id`

// ─── scan helper ──────────────────────────────────────────────────────────────

func scanRefreshToken(row pgx.Row) (*domain.RefreshToken, error) {
    var rt domain.RefreshToken
    err := row.Scan(
        &rt.ID,
        &rt.SessionID,
        &rt.UserID,
        &rt.DeviceID,
        &rt.TokenHash,
        &rt.IssuedAt,
        &rt.ExpiresAt,
        &rt.ConsumedAt,
        &rt.Revoked,
        &rt.RevokedAt,
        &rt.ReplacedByID,
    )
    if err != nil {
        return nil, repo.MapError(err)
    }
    return &rt, nil
}

// ─── read operations ──────────────────────────────────────────────────────────

const sqlRTFindByID = `
SELECT ` + rtCols + `
FROM   refresh_tokens
WHERE  id = $1`

func (r *refreshTokenRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.RefreshToken, error) {
    o := repo.NewOptions(opts...)
    return scanRefreshToken(r.Exec(o).QueryRow(ctx, sqlRTFindByID, id))
}

// sqlRTFindActiveByHash uses the unique B-tree index on token_hash.
const sqlRTFindActiveByHash = `
SELECT ` + rtCols + `
FROM   refresh_tokens
WHERE  token_hash   = $1
  AND  consumed_at  IS NULL
  AND  revoked      = FALSE
  AND  expires_at   > NOW()`

func (r *refreshTokenRepository) FindActiveByHash(ctx context.Context, tokenHash string, opts ...repo.Option) (*domain.RefreshToken, error) {
    o := repo.NewOptions(opts...)
    return scanRefreshToken(r.Exec(o).QueryRow(ctx, sqlRTFindActiveByHash, tokenHash))
}

// sqlRTFindActiveBySession uses idx_refresh_tokens_session_id.
const sqlRTFindActiveBySession = `
SELECT ` + rtCols + `
FROM   refresh_tokens
WHERE  session_id  = $1
  AND  consumed_at IS NULL
  AND  revoked     = FALSE
  AND  expires_at  > NOW()
ORDER  BY issued_at DESC
LIMIT  1`

func (r *refreshTokenRepository) FindActiveBySession(ctx context.Context, sessionID uuid.UUID, opts ...repo.Option) (*domain.RefreshToken, error) {
    o := repo.NewOptions(opts...)
    return scanRefreshToken(r.Exec(o).QueryRow(ctx, sqlRTFindActiveBySession, sessionID))
}

const sqlCountBySession = `SELECT COUNT(*) FROM refresh_tokens WHERE session_id = $1`

const sqlListBySession = `
SELECT ` + rtCols + `
FROM   refresh_tokens
WHERE  session_id = $1
ORDER  BY issued_at DESC
LIMIT  $2 OFFSET $3`

func (r *refreshTokenRepository) ListBySession(ctx context.Context, sessionID uuid.UUID, opts ...repo.Option) ([]*domain.RefreshToken, repo.PageMeta, error) {
    o := repo.NewOptions(opts...)
    db := r.Exec(o)

    p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
    if op := o.Pagination(); op != nil {
        p = *op
        p.Normalize()
    }

    var total int64
    if err := db.QueryRow(ctx, sqlCountBySession, sessionID).Scan(&total); err != nil {
        return nil, repo.PageMeta{}, repo.MapError(err)
    }

    rows, err := db.Query(ctx, sqlListBySession, sessionID, p.Limit(), p.Offset())
    if err != nil {
        return nil, repo.PageMeta{}, repo.MapError(err)
    }
    defer rows.Close()

    tokens := make([]*domain.RefreshToken, 0, p.PageSize)
    for rows.Next() {
        var rt domain.RefreshToken
        if err := rows.Scan(
            &rt.ID, &rt.SessionID, &rt.UserID, &rt.DeviceID, &rt.TokenHash,
            &rt.IssuedAt, &rt.ExpiresAt, &rt.ConsumedAt,
            &rt.Revoked, &rt.RevokedAt, &rt.ReplacedByID,
        ); err != nil {
            return nil, repo.PageMeta{}, repo.MapError(err)
        }
        tokens = append(tokens, &rt)
    }
    if err := rows.Err(); err != nil {
        return nil, repo.PageMeta{}, repo.MapError(err)
    }

    return tokens, repo.NewPageMeta(p, total), nil
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlRTCreate = `
INSERT INTO refresh_tokens (id, session_id, user_id, device_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING ` + rtCols

func (r *refreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken, opts ...repo.Option) (*domain.RefreshToken, error) {
    o := repo.NewOptions(opts...)
    row := r.Exec(o).QueryRow(ctx, sqlRTCreate,
        token.ID,
        token.SessionID,
        token.UserID,
        token.DeviceID,
        token.TokenHash,
        token.ExpiresAt,
    )
    return scanRefreshToken(row)
}

// sqlRTMarkConsumed atomically records consumption and the successor token ID.
// The guard AND consumed_at IS NULL AND revoked = FALSE makes this idempotent-safe:
// a second call for an already-consumed token returns 0 rows → ErrNotFound.
const sqlRTMarkConsumed = `
UPDATE refresh_tokens
SET    consumed_at    = NOW(),
       replaced_by_id = $2
WHERE  id           = $1
  AND  consumed_at  IS NULL
  AND  revoked      = FALSE`

func (r *refreshTokenRepository) MarkConsumed(ctx context.Context, id uuid.UUID, replacedByID uuid.UUID, opts ...repo.Option) error {
    o := repo.NewOptions(opts...)
    db := r.Exec(o)
    tag, err := db.Exec(ctx, sqlRTMarkConsumed, id, replacedByID)
    if err != nil {
        return repo.MapError(err)
    }
    if tag.RowsAffected() == 0 {
        return repo.ErrNotFound
    }
    return nil
}

const sqlRTRevoke = `
UPDATE refresh_tokens
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  id      = $1
  AND  revoked = FALSE`

func (r *refreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
    o := repo.NewOptions(opts...)
    db := r.Exec(o)
    tag, err := db.Exec(ctx, sqlRTRevoke, id)
    if err != nil {
        return repo.MapError(err)
    }
    if tag.RowsAffected() == 0 {
        return repo.ErrNotFound
    }
    return nil
}

// sqlRTRevokeAllForSession uses idx_refresh_tokens_session_id.
const sqlRTRevokeAllForSession = `
UPDATE refresh_tokens
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  session_id = $1
  AND  revoked    = FALSE`

func (r *refreshTokenRepository) RevokeAllForSession(ctx context.Context, sessionID uuid.UUID, opts ...repo.Option) error {
    o := repo.NewOptions(opts...)
    _, err := r.Exec(o).Exec(ctx, sqlRTRevokeAllForSession, sessionID)
    return repo.MapError(err)
}

// sqlRTRevokeAllForUser uses idx_refresh_tokens_user_id — the high-urgency write path.
const sqlRTRevokeAllForUser = `
UPDATE refresh_tokens
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  user_id = $1
  AND  revoked = FALSE`

func (r *refreshTokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error {
    o := repo.NewOptions(opts...)
    _, err := r.Exec(o).Exec(ctx, sqlRTRevokeAllForUser, userID)
    return repo.MapError(err)
}

// sqlRTPurgeStale targets idx_refresh_tokens_expires_consumed
// on (expires_at) WHERE consumed_at IS NOT NULL OR revoked = TRUE.
const sqlRTPurgeStale = `
DELETE FROM refresh_tokens
WHERE  (consumed_at IS NOT NULL OR revoked = TRUE)
  AND  expires_at <= NOW()`

func (r *refreshTokenRepository) PurgeStale(ctx context.Context) (int64, error) {
    tag, err := r.Pool().Exec(ctx, sqlRTPurgeStale)
    if err != nil {
        return 0, repo.MapError(err)
    }
    return tag.RowsAffected(), nil
}