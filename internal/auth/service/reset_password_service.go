package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	autherrors "github.com/AstroWalker24/Streamtogether-backend/internal/auth/errors"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/crypto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/password"
)

// ResetPasswordService completes the password-reset flow initiated by ForgotPassword.
type ResetPasswordService interface {
	// ResetPassword validates the token, hashes the new password, and atomically
	// updates the credential, consumes the token, and revokes all sessions.
	ResetPassword(ctx context.Context, req dto.ResetPasswordRequest) (dto.MessageResponse, error)
}

type resetPasswordService struct {
	prtRepo  authrepo.PasswordResetTokenRepository
	userRepo authrepo.UserRepository
	users    UserService
	sessions authrepo.SessionRepository
	tokens   authrepo.RefreshTokenRepository
	hasher   *password.Hasher
	db       *database.Database
}

// NewResetPasswordService constructs a ResetPasswordService with explicit dependencies.
func NewResetPasswordService(
	prtRepo authrepo.PasswordResetTokenRepository,
	userRepo authrepo.UserRepository,
	users UserService,
	sessions authrepo.SessionRepository,
	tokens authrepo.RefreshTokenRepository,
	hasher *password.Hasher,
	db *database.Database,
) ResetPasswordService {
	return &resetPasswordService{
		prtRepo:  prtRepo,
		userRepo: userRepo,
		users:    users,
		sessions: sessions,
		tokens:   tokens,
		hasher:   hasher,
		db:       db,
	}
}

func (s *resetPasswordService) ResetPassword(ctx context.Context, req dto.ResetPasswordRequest) (dto.MessageResponse, error) {
	// Validate password confirmation and policy before any DB interaction.
	if req.NewPassword != req.ConfirmPassword {
		return dto.MessageResponse{}, apperrors.NewBadRequest("passwords do not match")
	}
	if details := passwordPolicyViolation(req.NewPassword); details != "" {
		return dto.MessageResponse{}, autherrors.NewPasswordTooWeak(details)
	}

	tokenHash := crypto.SHA256Hex([]byte(strings.TrimSpace(req.Token)))

	prt, err := s.prtRepo.FindActiveByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.MessageResponse{}, autherrors.NewResetTokenInvalid()
		}
		return dto.MessageResponse{}, apperrors.NewDatabase(err)
	}

	// Resolve the user via UserService; treats deleted users as not-found.
	if _, err := s.users.GetByID(ctx, prt.UserID); err != nil {
		return dto.MessageResponse{}, err
	}

	// Hash the new password before the transaction (CPU-bound, no I/O).
	newHash, err := s.hasher.Hash(req.NewPassword)
	if err != nil {
		return dto.MessageResponse{}, apperrors.NewInternal("password hashing failed", err)
	}

	// Atomically: claim token → update password → revoke all sessions and tokens.
	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)

		// Claim the token first to handle concurrent submissions (concurrency guard).
		if err := s.prtRepo.MarkUsed(ctx, prt.ID, opt); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return autherrors.NewResetTokenConsumed()
			}
			return apperrors.NewDatabase(err)
		}

		if err := s.userRepo.UpdatePasswordHash(ctx, prt.UserID, newHash, opt); err != nil {
			return apperrors.NewDatabase(err)
		}

		if err := s.sessions.RevokeAllForUser(ctx, prt.UserID, opt); err != nil {
			return apperrors.NewDatabase(err)
		}

		return s.tokens.RevokeAllForUser(ctx, prt.UserID, opt)
	})
	if txErr != nil {
		return dto.MessageResponse{}, txErr
	}

	return dto.MessageResponse{Message: "password reset successfully"}, nil
}
