// Package service contains the business operation layer for the Friends domain.
package service

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"

	authservice "github.com/AstroWalker24/Streamtogether-backend/internal/auth/service"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/domain"
	friendserrors "github.com/AstroWalker24/Streamtogether-backend/internal/friends/errors"
	friendsrepo "github.com/AstroWalker24/Streamtogether-backend/internal/friends/repository"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// FriendshipService defines the business operations for established, mutual
// friendships. Friend requests, profile data, privacy, and blocking are outside
// this service's responsibility.
type FriendshipService interface {
	// EstablishFriendship validates both users, canonicalizes the pair, and
	// creates one established Friendship. Optional repository options allow a
	// caller to coordinate establishment within an existing transaction.
	EstablishFriendship(ctx context.Context, userA, userB uuid.UUID, opts ...repo.Option) (*domain.Friendship, error)

	// AreFriends reports whether an established friendship exists between two users.
	AreFriends(ctx context.Context, userA, userB uuid.UUID) (bool, error)

	// GetFriends returns paginated friendships involving userID, excluding friends
	// whose accounts are no longer active.
	GetFriends(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Friendship, repo.PageMeta, error)

	// RemoveFriendship removes the established friendship between two users.
	RemoveFriendship(ctx context.Context, userA, userB uuid.UUID) error
}

type friendshipService struct {
	friends friendsrepo.FriendshipRepository
	users   authservice.UserService
}

// NewFriendshipService constructs a FriendshipService with its relationship and
// account-eligibility dependencies.
func NewFriendshipService(friends friendsrepo.FriendshipRepository, users authservice.UserService) FriendshipService {
	return &friendshipService{friends: friends, users: users}
}

func (s *friendshipService) EstablishFriendship(ctx context.Context, userA, userB uuid.UUID, opts ...repo.Option) (*domain.Friendship, error) {
	if err := validateDistinctUserIDs(userA, userB); err != nil {
		return nil, err
	}
	if err := s.requireActiveUser(ctx, userA); err != nil {
		return nil, err
	}
	if err := s.requireActiveUser(ctx, userB); err != nil {
		return nil, err
	}

	firstUserID, secondUserID := canonicalPair(userA, userB)
	friendship := &domain.Friendship{
		ID:           uuid.New(),
		FirstUserID:  firstUserID,
		SecondUserID: secondUserID,
	}

	created, err := s.friends.Create(ctx, friendship, opts...)
	if err != nil {
		if errors.Is(err, repo.ErrDuplicateKey) {
			return nil, friendserrors.NewAlreadyFriends()
		}
		return nil, mapFriendshipRepoError(err)
	}
	return created, nil
}

func (s *friendshipService) AreFriends(ctx context.Context, userA, userB uuid.UUID) (bool, error) {
	if userA == uuid.Nil || userB == uuid.Nil {
		return false, friendserrors.NewInvalidUserID()
	}
	if userA == userB {
		return false, nil
	}

	firstUserID, secondUserID := canonicalPair(userA, userB)
	exists, err := s.friends.ExistsByUserPair(ctx, firstUserID, secondUserID)
	if err != nil {
		return false, mapFriendshipRepoError(err)
	}
	return exists, nil
}

func (s *friendshipService) GetFriends(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Friendship, repo.PageMeta, error) {
	if userID == uuid.Nil {
		return nil, repo.PageMeta{}, friendserrors.NewInvalidUserID()
	}

	friendships, pageMeta, err := s.friends.GetFriends(ctx, userID, opts...)
	if err != nil {
		return nil, repo.PageMeta{}, mapFriendshipRepoError(err)
	}

	visible := make([]*domain.Friendship, 0, len(friendships))
	for _, friendship := range friendships {
		friendUserID := otherUserID(friendship, userID)
		user, err := s.users.GetByID(ctx, friendUserID)
		if err != nil {
			if isNotFoundAppError(err) {
				continue
			}
			return nil, repo.PageMeta{}, err
		}
		if !user.IsActive() || user.IsDeleted() {
			continue
		}
		visible = append(visible, friendship)
	}

	return visible, pageMeta, nil
}

func (s *friendshipService) RemoveFriendship(ctx context.Context, userA, userB uuid.UUID) error {
	if err := validateDistinctUserIDs(userA, userB); err != nil {
		return err
	}

	firstUserID, secondUserID := canonicalPair(userA, userB)
	if err := s.friends.DeleteByUserPair(ctx, firstUserID, secondUserID); err != nil {
		return mapFriendshipRepoError(err)
	}
	return nil
}

func (s *friendshipService) requireActiveUser(ctx context.Context, userID uuid.UUID) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.IsActive() || user.IsDeleted() {
		return friendserrors.NewUserNotEligible()
	}
	return nil
}

func validateDistinctUserIDs(userA, userB uuid.UUID) error {
	if userA == uuid.Nil || userB == uuid.Nil {
		return friendserrors.NewInvalidUserID()
	}
	if userA == userB {
		return friendserrors.NewCannotFriendSelf()
	}
	return nil
}

func canonicalPair(userA, userB uuid.UUID) (uuid.UUID, uuid.UUID) {
	if bytes.Compare(userA[:], userB[:]) < 0 {
		return userA, userB
	}
	return userB, userA
}

func otherUserID(friendship *domain.Friendship, userID uuid.UUID) uuid.UUID {
	if friendship.FirstUserID == userID {
		return friendship.SecondUserID
	}
	return friendship.FirstUserID
}

func mapFriendshipRepoError(err error) error {
	if errors.Is(err, repo.ErrNotFound) {
		return friendserrors.NewFriendshipNotFound()
	}
	return apperrors.NewDatabase(err)
}

func isNotFoundAppError(err error) bool {
	var appErr *apperrors.AppError
	return errors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound
}
