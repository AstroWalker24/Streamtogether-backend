package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	autherrors "github.com/AstroWalker24/Streamtogether-backend/internal/auth/errors"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/crypto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// emailVerificationExpiry is the domain-defined lifetime of a verification token (§2.7).
const emailVerificationExpiry = 24 * time.Hour

// EmailVerificationService manages the email verification lifecycle.
type EmailVerificationService interface {
	// InitiateVerification generates a new verification token for userID,
	// invalidating any previous token (INV-15). Returns the raw plaintext token
	// for the email delivery layer; the raw value is never persisted.
	InitiateVerification(ctx context.Context, userID uuid.UUID) (rawToken string, err error)

	// VerifyEmail consumes the submitted token, marks the user's email as verified,
	// and transitions a pending_verification account to active.
	VerifyEmail(ctx context.Context, req dto.VerifyEmailRequest) (dto.MessageResponse, error)

	// ResendVerification invalidates previous tokens, issues a new one, and returns
	// the raw token for the delivery layer. Returns an empty rawToken and a generic
	// success MessageResponse when the email is not registered (no enumeration).
	ResendVerification(ctx context.Context, req dto.ResendVerificationRequest) (rawToken string, msg dto.MessageResponse, err error)
}

type emailVerificationService struct {
	evTokens authrepo.EmailVerificationTokenRepository
	userRepo authrepo.UserRepository
	users    UserService
	rng      *random.Generator
	db       *database.Database
}

// NewEmailVerificationService constructs an EmailVerificationService with explicit dependencies.
func NewEmailVerificationService(
	evTokens authrepo.EmailVerificationTokenRepository,
	userRepo authrepo.UserRepository,
	users UserService,
	rng *random.Generator,
	db *database.Database,
) EmailVerificationService {
	return &emailVerificationService{
		evTokens: evTokens,
		userRepo: userRepo,
		users:    users,
		rng:      rng,
		db:       db,
	}
}

func (s *emailVerificationService) InitiateVerification(ctx context.Context, userID uuid.UUID) (string, error) {
	rawToken, err := s.rng.Base64URL(32) // 256-bit entropy
	if err != nil {
		return "", apperrors.NewInternal("token generation failed", err)
	}
	tokenHash := crypto.SHA256Hex([]byte(rawToken))

	// INV-15: only one valid token may exist per user.
	if err := s.evTokens.InvalidateAllForUser(ctx, userID); err != nil {
		return "", apperrors.NewDatabase(err)
	}

	t := &domain.EmailVerificationToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(emailVerificationExpiry),
	}
	if _, err := s.evTokens.Create(ctx, t); err != nil {
		return "", apperrors.NewDatabase(err)
	}

	return rawToken, nil
}

func (s *emailVerificationService) VerifyEmail(ctx context.Context, req dto.VerifyEmailRequest) (dto.MessageResponse, error) {
	tokenHash := crypto.SHA256Hex([]byte(strings.TrimSpace(req.Token)))

	evToken, err := s.evTokens.FindActiveByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.MessageResponse{}, autherrors.NewVerificationTokenInvalid()
		}
		return dto.MessageResponse{}, apperrors.NewDatabase(err)
	}

	u, err := s.users.GetByID(ctx, evToken.UserID)
	if err != nil {
		return dto.MessageResponse{}, err
	}

	if u.EmailVerified {
		return dto.MessageResponse{}, autherrors.NewEmailAlreadyVerified()
	}

	capturedStatus := u.Status

	// Atomically: claim the token first (concurrency guard), then update the user.
	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)

		if err := s.evTokens.MarkUsed(ctx, evToken.ID, opt); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				// A concurrent request already consumed this token.
				return autherrors.NewVerificationTokenConsumed()
			}
			return apperrors.NewDatabase(err)
		}

		if err := s.userRepo.UpdateEmailVerified(ctx, evToken.UserID, true, opt); err != nil {
			return apperrors.NewDatabase(err)
		}

		if capturedStatus == domain.UserStatusPendingVerification {
			return s.userRepo.UpdateStatus(ctx, evToken.UserID, domain.UserStatusActive, opt)
		}

		return nil
	})
	if txErr != nil {
		return dto.MessageResponse{}, txErr
	}

	return dto.MessageResponse{Message: "email verified successfully"}, nil
}

func (s *emailVerificationService) ResendVerification(ctx context.Context, req dto.ResendVerificationRequest) (string, dto.MessageResponse, error) {
	// Generic success prevents email enumeration in all non-error paths.
	silentSuccess := dto.MessageResponse{Message: "if your email is registered and unverified, you will receive a new verification email"}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.HTTPStatus == http.StatusNotFound {
			return "", silentSuccess, nil
		}
		return "", dto.MessageResponse{}, err
	}

	if u.EmailVerified {
		// Already verified; don't reveal this state to the caller.
		return "", silentSuccess, nil
	}

	if u.Status == domain.UserStatusSuspended {
		// Don't issue verification tokens to suspended accounts.
		return "", silentSuccess, nil
	}

	rawToken, err := s.InitiateVerification(ctx, u.ID)
	if err != nil {
		return "", dto.MessageResponse{}, err
	}

	return rawToken, silentSuccess, nil
}
