// Package repository provides the PostgreSQL-backed persistence implementation
// for the Friends domain. All SQL is encapsulated here; callers interact only
// with the FriendshipRepository interface.
package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// FriendshipRepository defines the persistence contract for established,
// canonical Friendship records.
type FriendshipRepository interface {
	// Create inserts a canonical friendship and returns the record with
	// DB-populated fields set. The caller must provide canonical user IDs.
	Create(ctx context.Context, friendship *domain.Friendship, opts ...repo.Option) (*domain.Friendship, error)

	// FindByUserPair retrieves the friendship between two users regardless of
	// argument order. Returns repo.ErrNotFound when none exists.
	FindByUserPair(ctx context.Context, firstUserID, secondUserID uuid.UUID, opts ...repo.Option) (*domain.Friendship, error)

	// ExistsByUserPair reports whether an established friendship exists between
	// two users regardless of argument order.
	ExistsByUserPair(ctx context.Context, firstUserID, secondUserID uuid.UUID, opts ...repo.Option) (bool, error)

	// GetFriends returns paginated friendships involving userID, ordered by
	// creation time descending and then ID descending.
	GetFriends(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Friendship, repo.PageMeta, error)

	// DeleteByUserPair hard-deletes the friendship between two users regardless
	// of argument order. Returns repo.ErrNotFound when none exists.
	DeleteByUserPair(ctx context.Context, firstUserID, secondUserID uuid.UUID, opts ...repo.Option) error
}

// ─── implementation ───────────────────────────────────────────────────────────

type friendshipRepository struct {
	repo.Base
}

// NewFriendshipRepository constructs a FriendshipRepository backed by db.
func NewFriendshipRepository(db *database.Database, log logger.Logger) FriendshipRepository {
	return &friendshipRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ──────────────────────────────────────────────────────────────

const friendshipCols = `id, first_user_id, second_user_id, created_at`

// ─── scan helper ──────────────────────────────────────────────────────────────

func scanFriendship(row pgx.Row) (*domain.Friendship, error) {
	var friendship domain.Friendship
	err := row.Scan(
		&friendship.ID,
		&friendship.FirstUserID,
		&friendship.SecondUserID,
		&friendship.CreatedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &friendship, nil
}

// ─── read operations ──────────────────────────────────────────────────────────

const sqlFriendshipFindByUserPair = `
SELECT ` + friendshipCols + `
FROM   friendships
WHERE  (first_user_id = $1 AND second_user_id = $2)
    OR (first_user_id = $2 AND second_user_id = $1)`

func (r *friendshipRepository) FindByUserPair(ctx context.Context, firstUserID, secondUserID uuid.UUID, opts ...repo.Option) (*domain.Friendship, error) {
	o := repo.NewOptions(opts...)
	return scanFriendship(r.Exec(o).QueryRow(ctx, sqlFriendshipFindByUserPair, firstUserID, secondUserID))
}

const sqlFriendshipExistsByUserPair = `
SELECT EXISTS (
    SELECT 1
    FROM   friendships
    WHERE  (first_user_id = $1 AND second_user_id = $2)
        OR (first_user_id = $2 AND second_user_id = $1)
)`

func (r *friendshipRepository) ExistsByUserPair(ctx context.Context, firstUserID, secondUserID uuid.UUID, opts ...repo.Option) (bool, error) {
	var exists bool
	o := repo.NewOptions(opts...)
	if err := r.Exec(o).QueryRow(ctx, sqlFriendshipExistsByUserPair, firstUserID, secondUserID).Scan(&exists); err != nil {
		return false, repo.MapError(err)
	}
	return exists, nil
}

const sqlCountFriends = `
SELECT COUNT(*)
FROM   friendships
WHERE  first_user_id = $1
    OR second_user_id = $1`

const sqlGetFriends = `
SELECT ` + friendshipCols + `
FROM   friendships
WHERE  first_user_id = $1
    OR second_user_id = $1
ORDER  BY created_at DESC, id DESC
LIMIT  $2 OFFSET $3`

func (r *friendshipRepository) GetFriends(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Friendship, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, sqlCountFriends, userID).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlGetFriends, userID, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	friendships := make([]*domain.Friendship, 0, p.PageSize)
	for rows.Next() {
		var friendship domain.Friendship
		if err := rows.Scan(
			&friendship.ID,
			&friendship.FirstUserID,
			&friendship.SecondUserID,
			&friendship.CreatedAt,
		); err != nil {
			return nil, repo.PageMeta{}, repo.MapError(err)
		}
		friendships = append(friendships, &friendship)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	return friendships, repo.NewPageMeta(p, total), nil
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlFriendshipCreate = `
INSERT INTO friendships (id, first_user_id, second_user_id)
VALUES ($1, $2, $3)
RETURNING ` + friendshipCols

func (r *friendshipRepository) Create(ctx context.Context, friendship *domain.Friendship, opts ...repo.Option) (*domain.Friendship, error) {
	if friendship.ID == uuid.Nil {
		friendship.ID = uuid.New()
	}
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlFriendshipCreate,
		friendship.ID,
		friendship.FirstUserID,
		friendship.SecondUserID,
	)
	return scanFriendship(row)
}

const sqlFriendshipDeleteByUserPair = `
DELETE FROM friendships
WHERE  (first_user_id = $1 AND second_user_id = $2)
    OR (first_user_id = $2 AND second_user_id = $1)`

func (r *friendshipRepository) DeleteByUserPair(ctx context.Context, firstUserID, secondUserID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	tag, err := r.Exec(o).Exec(ctx, sqlFriendshipDeleteByUserPair, firstUserID, secondUserID)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}
