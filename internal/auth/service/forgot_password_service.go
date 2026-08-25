package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/crypto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// passwordResetExpiry is the domain-defined lifetime of a reset token (§2.8).
const passwordResetExpiry = 1 * time.Hour

// ForgotPasswordService initiates the password-reset flow.
type ForgotPasswordService interface {
	// ForgotPassword looks up the account, issues a reset token (INV-16), and
	// returns the raw token for the email delivery layer. Always returns a generic
	// MessageResponse to the caller regardless of whether the email is registered
	// (no enumeration). rawToken is empty when no token was issued.
	ForgotPassword(ctx context.Context, req dto.ForgotPasswordRequest) (rawToken string, msg dto.MessageResponse, err error)
}

type forgotPasswordService struct {
	prtRepo authrepo.PasswordResetTokenRepository
	users   UserService
	rng     *random.Generator
}

// NewForgotPasswordService constructs a ForgotPasswordService with explicit dependencies.
func NewForgotPasswordService(
	prtRepo authrepo.PasswordResetTokenRepository,
	users UserService,
	rng *random.Generator,
) ForgotPasswordService {
	return &forgotPasswordService{
		prtRepo: prtRepo,
		users:   users,
		rng:     rng,
	}
}

func (s *forgotPasswordService) ForgotPassword(ctx context.Context, req dto.ForgotPasswordRequest) (string, dto.MessageResponse, error) {
	// Generic response prevents account enumeration in all non-error paths.
	silentSuccess := dto.MessageResponse{Message: "if your email is registered, you will receive a password reset email"}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.HTTPStatus == http.StatusNotFound {
			return "", silentSuccess, nil
		}
		return "", dto.MessageResponse{}, err
	}

	rawToken, err := s.initiateReset(ctx, u.ID)
	if err != nil {
		return "", dto.MessageResponse{}, err
	}

	return rawToken, silentSuccess, nil
}

// initiateReset generates a new reset token, invalidates previous ones (INV-16),
// and persists the hash. Returns the raw plaintext token for email delivery.
func (s *forgotPasswordService) initiateReset(ctx context.Context, userID uuid.UUID) (string, error) {
	rawToken, err := s.rng.Base64URL(32) // 256-bit entropy
	if err != nil {
		return "", apperrors.NewInternal("token generation failed", err)
	}
	tokenHash := crypto.SHA256Hex([]byte(rawToken))

	// INV-16: at most one valid reset token per user.
	if err := s.prtRepo.InvalidateAllForUser(ctx, userID); err != nil {
		return "", apperrors.NewDatabase(err)
	}

	t := &domain.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(passwordResetExpiry),
	}
	if _, err := s.prtRepo.Create(ctx, t); err != nil {
		return "", apperrors.NewDatabase(err)
	}

	return rawToken, nil
}
