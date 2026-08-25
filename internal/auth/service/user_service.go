// Package service contains the business operation layer for the authentication domain.
package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/password"
)

// UserService defines the business operations for user-domain management.
// It orchestrates the UserRepository and password hashing without exposing
// infrastructure details to callers.
type UserService interface {
	// Create validates uniqueness, hashes the password, and persists a new user.
	// The returned user is in the pending_verification state.
	Create(ctx context.Context, email, username, plainPassword string) (*domain.User, error)

	// GetByID retrieves a non-deleted user by their system UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)

	// GetByEmail retrieves a non-deleted user by canonical email (case-insensitive).
	GetByEmail(ctx context.Context, email string) (*domain.User, error)

	// GetByUsername retrieves a non-deleted user by canonical username (case-insensitive).
	GetByUsername(ctx context.Context, username string) (*domain.User, error)

	// ChangePassword verifies the current password, enforces that the new password
	// differs (BR-12), hashes the new password, and persists the updated hash.
	ChangePassword(ctx context.Context, id uuid.UUID, currentPassword, newPassword string) error

	// UpdateStatus changes the lifecycle status of a non-deleted user.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UserStatus) error

	// SoftDelete marks the user as deleted without removing the record.
	SoftDelete(ctx context.Context, id uuid.UUID) error

	// IsEmailAvailable returns true when no account (including soft-deleted) holds email.
	IsEmailAvailable(ctx context.Context, email string) (bool, error)

	// IsUsernameAvailable returns true when no account (including soft-deleted) holds username.
	IsUsernameAvailable(ctx context.Context, username string) (bool, error)

	// CreateOAuthUser provisions a new user account for an OAuth identity.
	// The account is created with a sentinel password hash ("$oauth$") so it cannot
	// be used for password-based login. Status is always active; EmailVerified is
	// set from the provider's verified claim.
	CreateOAuthUser(ctx context.Context, email, username string, emailVerified bool) (*domain.User, error)

	// UpdateEmailVerified marks the user's email as verified.
	// Only call this after a trusted external verification (e.g., OAuth identity
	// with email_verified=true matching the user's registered email).
	UpdateEmailVerified(ctx context.Context, id uuid.UUID, verified bool) error
}

type userService struct {
	users  authrepo.UserRepository
	hasher *password.Hasher
}

// NewUserService constructs a UserService with explicit dependencies.
// It is safe for concurrent use.
func NewUserService(users authrepo.UserRepository, hasher *password.Hasher) UserService {
	return &userService{users: users, hasher: hasher}
}

func (s *userService) Create(ctx context.Context, email, username, plainPassword string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	username = strings.ToLower(strings.TrimSpace(username))

	emailTaken, err := s.users.ExistsEmail(ctx, email)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	if emailTaken {
		return nil, apperrors.NewConflict("email already registered")
	}

	usernameTaken, err := s.users.ExistsUsername(ctx, username)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	if usernameTaken {
		return nil, apperrors.NewConflict("username already taken")
	}

	hash, err := s.hasher.Hash(plainPassword)
	if err != nil {
		return nil, apperrors.NewInternal("password hashing failed", err)
	}

	u := &domain.User{
		ID:            uuid.New(),
		Email:         email,
		Username:      username,
		PasswordHash:  hash,
		Status:        domain.UserStatusPendingVerification,
		EmailVerified: false,
	}

	created, err := s.users.Create(ctx, u)
	if err != nil {
		if errors.Is(err, repo.ErrDuplicateKey) {
			return nil, apperrors.NewConflict("user already exists")
		}
		return nil, apperrors.NewDatabase(err)
	}
	return created, nil
}

func (s *userService) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, mapUserRepoError(err)
	}
	return u, nil
}

func (s *userService) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil, mapUserRepoError(err)
	}
	return u, nil
}

func (s *userService) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	u, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, mapUserRepoError(err)
	}
	return u, nil
}

func (s *userService) ChangePassword(ctx context.Context, id uuid.UUID, currentPassword, newPassword string) error {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return mapUserRepoError(err)
	}

	ok, err := s.hasher.Verify(currentPassword, u.PasswordHash)
	if err != nil {
		return apperrors.NewInternal("password verification failed", err)
	}
	if !ok {
		return apperrors.NewUnauthorized("current password is incorrect")
	}

	// BR-12: new password must differ from the current one.
	same, err := s.hasher.Verify(newPassword, u.PasswordHash)
	if err != nil {
		return apperrors.NewInternal("password verification failed", err)
	}
	if same {
		return apperrors.NewBadRequest("new password must differ from the current password")
	}

	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return apperrors.NewInternal("password hashing failed", err)
	}

	if err := s.users.UpdatePasswordHash(ctx, id, hash); err != nil {
		return mapUserRepoError(err)
	}
	return nil
}

func (s *userService) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UserStatus) error {
	if err := s.users.UpdateStatus(ctx, id, status); err != nil {
		return mapUserRepoError(err)
	}
	return nil
}

func (s *userService) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if err := s.users.SoftDelete(ctx, id); err != nil {
		return mapUserRepoError(err)
	}
	return nil
}

func (s *userService) IsEmailAvailable(ctx context.Context, email string) (bool, error) {
	exists, err := s.users.ExistsEmail(ctx, email)
	if err != nil {
		return false, apperrors.NewDatabase(err)
	}
	return !exists, nil
}

func (s *userService) IsUsernameAvailable(ctx context.Context, username string) (bool, error) {
	exists, err := s.users.ExistsUsername(ctx, username)
	if err != nil {
		return false, apperrors.NewDatabase(err)
	}
	return !exists, nil
}

// oauthPasswordSentinel is stored as the password_hash for OAuth-only accounts.
// It is deliberately non-empty (satisfying the DB CHECK constraint) and cannot
// be produced by any valid Argon2id operation, preventing password-based login.
const oauthPasswordSentinel = "$oauth$"

func (s *userService) CreateOAuthUser(ctx context.Context, email, username string, emailVerified bool) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	username = strings.ToLower(strings.TrimSpace(username))

	emailTaken, err := s.users.ExistsEmail(ctx, email)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	if emailTaken {
		return nil, apperrors.NewConflict("email already registered")
	}

	usernameTaken, err := s.users.ExistsUsername(ctx, username)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	if usernameTaken {
		return nil, apperrors.NewConflict("username already taken")
	}

	u := &domain.User{
		ID:            uuid.New(),
		Email:         email,
		Username:      username,
		PasswordHash:  oauthPasswordSentinel,
		Status:        domain.UserStatusActive,
		EmailVerified: emailVerified,
	}

	created, err := s.users.Create(ctx, u)
	if err != nil {
		if errors.Is(err, repo.ErrDuplicateKey) {
			return nil, apperrors.NewConflict("user already exists")
		}
		return nil, apperrors.NewDatabase(err)
	}
	return created, nil
}

func (s *userService) UpdateEmailVerified(ctx context.Context, id uuid.UUID, verified bool) error {
	if err := s.users.UpdateEmailVerified(ctx, id, verified); err != nil {
		return mapUserRepoError(err)
	}
	return nil
}

// mapUserRepoError translates repository sentinel errors into application errors.
func mapUserRepoError(err error) error {
	if errors.Is(err, repo.ErrNotFound) {
		return apperrors.NewNotFound("user")
	}
	if errors.Is(err, repo.ErrDuplicateKey) {
		return apperrors.NewConflict("user already exists")
	}
	return apperrors.NewDatabase(err)
}
