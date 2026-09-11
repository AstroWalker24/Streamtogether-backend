package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/AstroWalker24/Streamtogether-backend/internal/presence/domain"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

type postgresPresenceRepository struct {
	repo.Base
}

// NewDurablePresenceRepository constructs a DurablePresenceRepository backed by PostgreSQL.
func NewDurablePresenceRepository(db *database.Database, log logger.Logger) DurablePresenceRepository {
	return &postgresPresenceRepository{Base: repo.NewBase(db, log)}
}

const userPresenceCols = `user_id, last_seen_at, updated_at`

func scanDurablePresence(row pgx.Row) (*domain.DurablePresence, error) {
	var p domain.DurablePresence
	err := row.Scan(
		&p.UserID,
		&p.LastSeenAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &p, nil
}

const sqlPresenceFindByUserID = `
SELECT ` + userPresenceCols + `
FROM   user_presences
WHERE  user_id = $1`

func (r *postgresPresenceRepository) FindByUserID(ctx context.Context, userID uuid.UUID, opts ...repo.Option) (*domain.DurablePresence, error) {
	o := repo.NewOptions(opts...)
	return scanDurablePresence(r.Exec(o).QueryRow(ctx, sqlPresenceFindByUserID, userID))
}

const sqlPresenceUpsertLastSeen = `
INSERT INTO user_presences (user_id, last_seen_at, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (user_id)
DO UPDATE SET
    last_seen_at = EXCLUDED.last_seen_at,
    updated_at   = NOW()`

func (r *postgresPresenceRepository) UpsertLastSeen(ctx context.Context, userID uuid.UUID, lastSeenAt time.Time, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlPresenceUpsertLastSeen, userID, lastSeenAt)
	if err != nil {
		return repo.MapError(err)
	}
	return nil
}

const sqlPresenceFindByUserIDs = `
SELECT ` + userPresenceCols + `
FROM   user_presences
WHERE  user_id = ANY($1)`

func (r *postgresPresenceRepository) FindByUserIDs(ctx context.Context, userIDs []uuid.UUID, opts ...repo.Option) (map[uuid.UUID]*domain.DurablePresence, error) {
	if len(userIDs) == 0 {
		return make(map[uuid.UUID]*domain.DurablePresence), nil
	}

	o := repo.NewOptions(opts...)
	rows, err := r.Exec(o).Query(ctx, sqlPresenceFindByUserIDs, userIDs)
	if err != nil {
		return nil, repo.MapError(err)
	}
	defer rows.Close()

	results := make(map[uuid.UUID]*domain.DurablePresence, len(userIDs))
	for rows.Next() {
		p, scanErr := scanDurablePresence(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		results[p.UserID] = p
	}

	if err := rows.Err(); err != nil {
		return nil, repo.MapError(err)
	}

	return results, nil
}
