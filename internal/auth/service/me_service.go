package service

import (
	"context"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	autherrors "github.com/AstroWalker24/Streamtogether-backend/internal/auth/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/mapper"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
)

// MeService retrieves the currently authenticated user's profile.
type MeService interface {
	// GetCurrentUser returns the authenticated caller's safe public profile.
	// It re-validates the account state against the live record to catch
	// suspensions that occurred after the access token was issued.
	GetCurrentUser(ctx context.Context, claims token.AccessTokenClaims) (dto.UserResponse, error)
}

type meService struct {
	users UserService
}

// NewMeService constructs a MeService backed by the given UserService.
func NewMeService(users UserService) MeService {
	return &meService{users: users}
}

func (s *meService) GetCurrentUser(ctx context.Context, claims token.AccessTokenClaims) (dto.UserResponse, error) {
	u, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		// Covers deleted users (GetByID returns ErrNotFound for soft-deleted rows).
		return dto.UserResponse{}, err
	}

	if !u.IsActive() {
		// Account was suspended (or otherwise deactivated) since the token was issued.
		return dto.UserResponse{}, autherrors.NewAccountSuspended()
	}

	return mapper.UserToResponse(u), nil
}
