// Package repository provides the PostgreSQL-backed persistence implementation
// for the Friend Requests domain. All SQL is encapsulated here; callers interact
// only with the FriendRequestRepository interface.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// FriendRequestRepository defines the persistence contract for directed
// FriendRequest records. It does not make eligibility, authorization, or
// lifecycle-transition decisions.
type FriendRequestRepository interface {
	// Create inserts a pending Friend Request and returns the record with
	// DB-populated fields set.
	Create(ctx context.Context, request *domain.FriendRequest, opts ...repo.Option) (*domain.FriendRequest, error)

	// FindByID retrieves a Friend Request by its surrogate UUID. WithLock may be
	// supplied inside a transaction before a status transition.
	FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.FriendRequest, error)

	// FindPendingByRequesterRecipient retrieves the pending request in the exact
	// requester -> recipient direction. It does not reverse or canonicalize IDs.
	FindPendingByRequesterRecipient(ctx context.Context, requesterUserID, recipientUserID uuid.UUID, opts ...repo.Option) (*domain.FriendRequest, error)

	// FindPendingByEitherDirection retrieves pending requests between two users
	// while preserving their stored direction. It supports service-level reverse
	// request policy without imposing an unordered-pair rule in persistence.
	FindPendingByEitherDirection(ctx context.Context, userA, userB uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, error)

	// LockUserPair serializes Friend Request decisions for an unordered pair
	// within the caller's transaction while preserving directed request storage.
	LockUserPair(ctx context.Context, userA, userB uuid.UUID, opts ...repo.Option) error

	// ListIncoming returns paginated pending requests received by userID, ordered
	// by creation time descending and then ID descending.
	ListIncoming(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error)

	// ListOutgoing returns paginated pending requests sent by userID, ordered by
	// creation time descending and then ID descending.
	ListOutgoing(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error)

	// UpdateStatus persists a status and responded_at value for an existing
	// request. The service is responsible for validating the transition.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.FriendRequestStatus, respondedAt *time.Time, opts ...repo.Option) (*domain.FriendRequest, error)
}

// --- implementation ----------------------------------------------------------

type friendRequestRepository struct {
	repo.Base
}

// NewFriendRequestRepository constructs a FriendRequestRepository backed by db.
func NewFriendRequestRepository(db *database.Database, log logger.Logger) FriendRequestRepository {
	return &friendRequestRepository{Base: repo.NewBase(db, log)}
}

// --- column list --------------------------------------------------------------

const friendRequestCols = `id, requester_user_id, recipient_user_id, status,
       created_at, updated_at, responded_at`

// --- scan helpers -------------------------------------------------------------

func scanFriendRequest(row pgx.Row) (*domain.FriendRequest, error) {
	var request domain.FriendRequest
	err := row.Scan(
		&request.ID,
		&request.RequesterUserID,
		&request.RecipientUserID,
		(*string)(&request.Status),
		&request.CreatedAt,
		&request.UpdatedAt,
		&request.RespondedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &request, nil
}

func scanFriendRequestRows(rows pgx.Rows) ([]*domain.FriendRequest, error) {
	requests := make([]*domain.FriendRequest, 0)
	for rows.Next() {
		var request domain.FriendRequest
		if err := rows.Scan(
			&request.ID,
			&request.RequesterUserID,
			&request.RecipientUserID,
			(*string)(&request.Status),
			&request.CreatedAt,
			&request.UpdatedAt,
			&request.RespondedAt,
		); err != nil {
			return nil, repo.MapError(err)
		}
		requests = append(requests, &request)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.MapError(err)
	}
	return requests, nil
}

// --- read operations ----------------------------------------------------------

const sqlFriendRequestFindByID = `
SELECT ` + friendRequestCols + `
FROM   friend_requests
WHERE  id = $1`

const sqlFriendRequestFindByIDForUpdate = `
SELECT ` + friendRequestCols + `
FROM   friend_requests
WHERE  id = $1
FOR UPDATE`

func (r *friendRequestRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.FriendRequest, error) {
	o := repo.NewOptions(opts...)
	query := sqlFriendRequestFindByID
	if o.LockForUpdate() {
		query = sqlFriendRequestFindByIDForUpdate
	}
	return scanFriendRequest(r.Exec(o).QueryRow(ctx, query, id))
}

const sqlFriendRequestFindPendingByRequesterRecipient = `
SELECT ` + friendRequestCols + `
FROM   friend_requests
WHERE  requester_user_id = $1
  AND  recipient_user_id = $2
  AND  status = 'pending'`

func (r *friendRequestRepository) FindPendingByRequesterRecipient(ctx context.Context, requesterUserID, recipientUserID uuid.UUID, opts ...repo.Option) (*domain.FriendRequest, error) {
	o := repo.NewOptions(opts...)
	return scanFriendRequest(r.Exec(o).QueryRow(ctx, sqlFriendRequestFindPendingByRequesterRecipient, requesterUserID, recipientUserID))
}

const sqlFriendRequestFindPendingByEitherDirection = `
SELECT ` + friendRequestCols + `
FROM   friend_requests
WHERE  ((requester_user_id = $1 AND recipient_user_id = $2)
    OR  (requester_user_id = $2 AND recipient_user_id = $1))
  AND  status = 'pending'
ORDER  BY created_at DESC, id DESC`

func (r *friendRequestRepository) FindPendingByEitherDirection(ctx context.Context, userA, userB uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, error) {
	o := repo.NewOptions(opts...)
	rows, err := r.Exec(o).Query(ctx, sqlFriendRequestFindPendingByEitherDirection, userA, userB)
	if err != nil {
		return nil, repo.MapError(err)
	}
	defer rows.Close()

	return scanFriendRequestRows(rows)
}

const sqlFriendRequestLockUserPair = `
SELECT pg_advisory_xact_lock(
    hashtextextended(
        LEAST($1::TEXT, $2::TEXT) || ':' || GREATEST($1::TEXT, $2::TEXT),
        0
    )
)`

func (r *friendRequestRepository) LockUserPair(ctx context.Context, userA, userB uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlFriendRequestLockUserPair, userA, userB)
	return repo.MapError(err)
}

const sqlCountIncomingFriendRequests = `
SELECT COUNT(*)
FROM   friend_requests
WHERE  recipient_user_id = $1
  AND  status = 'pending'`

const sqlListIncomingFriendRequests = `
SELECT ` + friendRequestCols + `
FROM   friend_requests
WHERE  recipient_user_id = $1
  AND  status = 'pending'
ORDER  BY created_at DESC, id DESC
LIMIT  $2 OFFSET $3`

func (r *friendRequestRepository) ListIncoming(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error) {
	return r.listPending(ctx, userID, sqlCountIncomingFriendRequests, sqlListIncomingFriendRequests, opts...)
}

const sqlCountOutgoingFriendRequests = `
SELECT COUNT(*)
FROM   friend_requests
WHERE  requester_user_id = $1
  AND  status = 'pending'`

const sqlListOutgoingFriendRequests = `
SELECT ` + friendRequestCols + `
FROM   friend_requests
WHERE  requester_user_id = $1
  AND  status = 'pending'
ORDER  BY created_at DESC, id DESC
LIMIT  $2 OFFSET $3`

func (r *friendRequestRepository) ListOutgoing(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error) {
	return r.listPending(ctx, userID, sqlCountOutgoingFriendRequests, sqlListOutgoingFriendRequests, opts...)
}

func (r *friendRequestRepository) listPending(ctx context.Context, userID uuid.UUID, countQuery, listQuery string, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, listQuery, userID, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	requests, err := scanFriendRequestRows(rows)
	if err != nil {
		return nil, repo.PageMeta{}, err
	}
	return requests, repo.NewPageMeta(p, total), nil
}

// --- write operations ---------------------------------------------------------

const sqlFriendRequestCreate = `
INSERT INTO friend_requests (id, requester_user_id, recipient_user_id, status)
VALUES ($1, $2, $3, $4)
RETURNING ` + friendRequestCols

func (r *friendRequestRepository) Create(ctx context.Context, request *domain.FriendRequest, opts ...repo.Option) (*domain.FriendRequest, error) {
	if request.ID == uuid.Nil {
		request.ID = uuid.New()
	}
	if request.Status == "" {
		request.Status = domain.FriendRequestStatusPending
	}
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlFriendRequestCreate,
		request.ID,
		request.RequesterUserID,
		request.RecipientUserID,
		string(request.Status),
	)
	return scanFriendRequest(row)
}

// sqlFriendRequestUpdateStatus writes only transition-managed fields.
// Endpoints and created_at are immutable, and updated_at is trigger-managed.
const sqlFriendRequestUpdateStatus = `
UPDATE friend_requests
SET    status       = $2,
       responded_at = $3
WHERE  id = $1
RETURNING ` + friendRequestCols

func (r *friendRequestRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.FriendRequestStatus, respondedAt *time.Time, opts ...repo.Option) (*domain.FriendRequest, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlFriendRequestUpdateStatus, id, string(status), respondedAt)
	return scanFriendRequest(row)
}
