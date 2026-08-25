package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// LogoutService invalidates the current authenticated session.
type LogoutService interface {
	// Logout revokes the session carried by claims and all its associated refresh
	// tokens. If the session is already revoked, the call succeeds idempotently.
	Logout(ctx context.Context, claims token.AccessTokenClaims) (dto.MessageResponse, error)
}

type logoutService struct {
	sessions SessionService
}

// NewLogoutService constructs a LogoutService backed by the given SessionService.
func NewLogoutService(sessions SessionService) LogoutService {
	return &logoutService{sessions: sessions}
}

func (s *logoutService) Logout(ctx context.Context, claims token.AccessTokenClaims) (dto.MessageResponse, error) {
	_, err := s.sessions.RevokeSession(ctx, claims.UserID, claims.SessionID)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.HTTPStatus == http.StatusNotFound {
			// Session already revoked or never existed — idempotent success.
			return dto.MessageResponse{Message: "logged out successfully"}, nil
		}
		return dto.MessageResponse{}, err
	}
	return dto.MessageResponse{Message: "logged out successfully"}, nil
}
