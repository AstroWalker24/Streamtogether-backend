// Package service contains the business operation layer for Friend Requests.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authservice "github.com/AstroWalker24/Streamtogether-backend/internal/auth/service"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	friendserrors "github.com/AstroWalker24/Streamtogether-backend/internal/friends/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/domain"
	requesterrors "github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/errors"
	requestrepo "github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/repository"
	friendsvc "github.com/AstroWalker24/Streamtogether-backend/internal/friends/service"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// FriendRequestService defines business operations for directed Friend Requests.
type FriendRequestService interface {
	SendFriendRequest(ctx context.Context, requesterUserID, recipientUserID uuid.UUID) (*domain.FriendRequest, error)
	AcceptFriendRequest(ctx context.Context, actorUserID, requestID uuid.UUID) (*domain.FriendRequest, error)
	RejectFriendRequest(ctx context.Context, actorUserID, requestID uuid.UUID) (*domain.FriendRequest, error)
	CancelFriendRequest(ctx context.Context, actorUserID, requestID uuid.UUID) (*domain.FriendRequest, error)
	GetIncomingRequests(ctx context.Context, actorUserID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error)
	GetOutgoingRequests(ctx context.Context, actorUserID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error)
}

type friendRequestService struct {
	requests    requestrepo.FriendRequestRepository
	users       authservice.UserService
	friendships friendsvc.FriendshipService
	db          *database.Database
}

// NewFriendRequestService constructs a FriendRequestService with explicit
// persistence, identity, and Friendship dependencies.
func NewFriendRequestService(requests requestrepo.FriendRequestRepository, users authservice.UserService, friendships friendsvc.FriendshipService, db *database.Database) FriendRequestService {
	return &friendRequestService{requests: requests, users: users, friendships: friendships, db: db}
}

func (s *friendRequestService) SendFriendRequest(ctx context.Context, requesterUserID, recipientUserID uuid.UUID) (*domain.FriendRequest, error) {
	if err := validateDistinctUserIDs(requesterUserID, recipientUserID); err != nil {
		return nil, err
	}
	if err := s.requireActiveUser(ctx, requesterUserID); err != nil {
		return nil, err
	}
	if err := s.requireActiveUser(ctx, recipientUserID); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txOpt := repo.WithTransaction(tx)
	if err := s.requests.LockUserPair(ctx, requesterUserID, recipientUserID, txOpt); err != nil {
		return nil, mapRequestRepoError(err)
	}

	alreadyFriends, err := s.friendships.AreFriends(ctx, requesterUserID, recipientUserID)
	if err != nil {
		return nil, err
	}
	if alreadyFriends {
		return nil, apperrors.NewConflict("users are already friends")
	}

	if _, err := s.requests.FindPendingByRequesterRecipient(ctx, requesterUserID, recipientUserID, txOpt); err == nil {
		return nil, requesterrors.NewDuplicatePending()
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, mapRequestRepoError(err)
	}

	reverse, err := s.requests.FindPendingByRequesterRecipient(ctx, recipientUserID, requesterUserID, txOpt)
	if err == nil {
		accepted, err := s.acceptLocked(ctx, tx, requesterUserID, reverse)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, apperrors.NewDatabase(err)
		}
		return accepted, nil
	}
	if !errors.Is(err, repo.ErrNotFound) {
		return nil, mapRequestRepoError(err)
	}

	created, err := s.requests.Create(ctx, &domain.FriendRequest{
		ID:              uuid.New(),
		RequesterUserID: requesterUserID,
		RecipientUserID: recipientUserID,
		Status:          domain.FriendRequestStatusPending,
	}, txOpt)
	if err != nil {
		return nil, mapRequestRepoError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	return created, nil
}

func (s *friendRequestService) AcceptFriendRequest(ctx context.Context, actorUserID, requestID uuid.UUID) (*domain.FriendRequest, error) {
	if err := validateOperationIDs(actorUserID, requestID); err != nil {
		return nil, err
	}
	if err := s.requireActiveUser(ctx, actorUserID); err != nil {
		return nil, err
	}
	return s.withLockedRequest(ctx, requestID, func(tx pgx.Tx, request *domain.FriendRequest) (*domain.FriendRequest, error) {
		if request.RecipientUserID != actorUserID {
			return nil, requesterrors.NewUnauthorizedTransition()
		}
		if err := s.requireActiveUser(ctx, request.RequesterUserID); err != nil {
			return nil, err
		}
		return s.acceptLocked(ctx, tx, actorUserID, request)
	})
}

func (s *friendRequestService) RejectFriendRequest(ctx context.Context, actorUserID, requestID uuid.UUID) (*domain.FriendRequest, error) {
	if err := validateOperationIDs(actorUserID, requestID); err != nil {
		return nil, err
	}
	if err := s.requireActiveUser(ctx, actorUserID); err != nil {
		return nil, err
	}
	return s.withLockedRequest(ctx, requestID, func(tx pgx.Tx, request *domain.FriendRequest) (*domain.FriendRequest, error) {
		if request.RecipientUserID != actorUserID {
			return nil, requesterrors.NewUnauthorizedTransition()
		}
		if request.Status != domain.FriendRequestStatusPending {
			return nil, requesterrors.NewRequestNotPending()
		}
		now := time.Now().UTC()
		updated, err := s.requests.UpdateStatus(ctx, request.ID, domain.FriendRequestStatusRejected, &now, repo.WithTransaction(tx))
		if err != nil {
			return nil, mapRequestRepoError(err)
		}
		return updated, nil
	})
}

func (s *friendRequestService) CancelFriendRequest(ctx context.Context, actorUserID, requestID uuid.UUID) (*domain.FriendRequest, error) {
	if err := validateOperationIDs(actorUserID, requestID); err != nil {
		return nil, err
	}
	if err := s.requireActiveUser(ctx, actorUserID); err != nil {
		return nil, err
	}
	return s.withLockedRequest(ctx, requestID, func(tx pgx.Tx, request *domain.FriendRequest) (*domain.FriendRequest, error) {
		if request.RequesterUserID != actorUserID {
			return nil, requesterrors.NewUnauthorizedTransition()
		}
		if request.Status != domain.FriendRequestStatusPending {
			return nil, requesterrors.NewRequestNotPending()
		}
		updated, err := s.requests.UpdateStatus(ctx, request.ID, domain.FriendRequestStatusCancelled, nil, repo.WithTransaction(tx))
		if err != nil {
			return nil, mapRequestRepoError(err)
		}
		return updated, nil
	})
}

func (s *friendRequestService) GetIncomingRequests(ctx context.Context, actorUserID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error) {
	if err := s.requireActiveUser(ctx, actorUserID); err != nil {
		return nil, repo.PageMeta{}, err
	}
	requests, pageMeta, err := s.requests.ListIncoming(ctx, actorUserID, opts...)
	if err != nil {
		return nil, repo.PageMeta{}, mapRequestRepoError(err)
	}
	return requests, pageMeta, nil
}

func (s *friendRequestService) GetOutgoingRequests(ctx context.Context, actorUserID uuid.UUID, opts ...repo.Option) ([]*domain.FriendRequest, repo.PageMeta, error) {
	if err := s.requireActiveUser(ctx, actorUserID); err != nil {
		return nil, repo.PageMeta{}, err
	}
	requests, pageMeta, err := s.requests.ListOutgoing(ctx, actorUserID, opts...)
	if err != nil {
		return nil, repo.PageMeta{}, mapRequestRepoError(err)
	}
	return requests, pageMeta, nil
}

func (s *friendRequestService) acceptLocked(ctx context.Context, tx pgx.Tx, actorUserID uuid.UUID, request *domain.FriendRequest) (*domain.FriendRequest, error) {
	if request.RecipientUserID != actorUserID {
		return nil, requesterrors.NewUnauthorizedTransition()
	}
	if request.Status != domain.FriendRequestStatusPending {
		return nil, requesterrors.NewRequestNotPending()
	}
	if _, err := s.friendships.EstablishFriendship(ctx, request.RequesterUserID, request.RecipientUserID, repo.WithTransaction(tx)); err != nil && !isAlreadyFriendsError(err) {
		return nil, err
	}
	now := time.Now().UTC()
	updated, err := s.requests.UpdateStatus(ctx, request.ID, domain.FriendRequestStatusAccepted, &now, repo.WithTransaction(tx))
	if err != nil {
		return nil, mapRequestRepoError(err)
	}
	return updated, nil
}

func (s *friendRequestService) withLockedRequest(ctx context.Context, requestID uuid.UUID, operation func(pgx.Tx, *domain.FriendRequest) (*domain.FriendRequest, error)) (*domain.FriendRequest, error) {
	tx, err := s.db.BeginTx(ctx)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	request, err := s.requests.FindByID(ctx, requestID, repo.WithTransaction(tx), repo.WithLock())
	if err != nil {
		return nil, mapRequestRepoError(err)
	}
	updated, err := operation(tx, request)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	return updated, nil
}

func (s *friendRequestService) requireActiveUser(ctx context.Context, userID uuid.UUID) error {
	if userID == uuid.Nil {
		return requesterrors.NewInvalidUserID()
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.IsActive() || user.IsDeleted() {
		return requesterrors.NewUserNotEligible()
	}
	return nil
}

func validateDistinctUserIDs(requesterUserID, recipientUserID uuid.UUID) error {
	if requesterUserID == uuid.Nil || recipientUserID == uuid.Nil {
		return requesterrors.NewInvalidUserID()
	}
	if requesterUserID == recipientUserID {
		return requesterrors.NewCannotRequestSelf()
	}
	return nil
}

func validateOperationIDs(actorUserID, requestID uuid.UUID) error {
	if actorUserID == uuid.Nil || requestID == uuid.Nil {
		return requesterrors.NewInvalidUserID()
	}
	return nil
}

func mapRequestRepoError(err error) error {
	if errors.Is(err, repo.ErrNotFound) {
		return requesterrors.NewRequestNotFound()
	}
	if errors.Is(err, repo.ErrDuplicateKey) {
		return requesterrors.NewDuplicatePending()
	}
	return apperrors.NewDatabase(err)
}

func isAlreadyFriendsError(err error) bool {
	var appErr *apperrors.AppError
	return errors.As(err, &appErr) && appErr.Code == friendserrors.CodeAlreadyFriends
}
