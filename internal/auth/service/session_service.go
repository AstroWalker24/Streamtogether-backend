package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/mapper"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// SessionService manages the lifecycle of authenticated sessions after login.
type SessionService interface {
	// GetSession returns session metadata, verifying it belongs to userID.
	GetSession(ctx context.Context, userID, sessionID uuid.UUID) (dto.SessionResponse, error)

	// ListSessions returns all active sessions for userID.
	// currentSessionID identifies the caller's own session for IsCurrent annotation.
	ListSessions(ctx context.Context, userID, currentSessionID uuid.UUID) ([]dto.SessionResponse, error)

	// RevokeSession revokes a specific session owned by userID and its refresh tokens.
	RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) (dto.MessageResponse, error)

	// RevokeAllSessions revokes all sessions for userID and all their refresh tokens.
	RevokeAllSessions(ctx context.Context, userID uuid.UUID) (dto.MessageResponse, error)
}

type sessionService struct {
	sessions authrepo.SessionRepository
	tokens   authrepo.RefreshTokenRepository
	devices  authrepo.DeviceRepository
	db       *database.Database
}

// NewSessionService constructs a SessionService with explicit dependencies.
func NewSessionService(
	sessions authrepo.SessionRepository,
	tokens authrepo.RefreshTokenRepository,
	devices authrepo.DeviceRepository,
	db *database.Database,
) SessionService {
	return &sessionService{
		sessions: sessions,
		tokens:   tokens,
		devices:  devices,
		db:       db,
	}
}

func (s *sessionService) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (dto.SessionResponse, error) {
	sess, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.SessionResponse{}, apperrors.NewNotFound("session")
		}
		return dto.SessionResponse{}, apperrors.NewDatabase(err)
	}

	if sess.UserID != userID {
		return dto.SessionResponse{}, apperrors.NewForbidden("not authorized to access this session")
	}

	device, err := s.resolveDevice(ctx, sess.DeviceID)
	if err != nil {
		return dto.SessionResponse{}, err
	}

	return mapper.SessionToResponse(sess, device, false), nil
}

func (s *sessionService) ListSessions(ctx context.Context, userID, currentSessionID uuid.UUID) ([]dto.SessionResponse, error) {
	sessions, _, err := s.sessions.ListActiveByUser(ctx, userID)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}

	out := make([]dto.SessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		device, err := s.resolveDevice(ctx, sess.DeviceID)
		if err != nil {
			return nil, err
		}
		out = append(out, mapper.SessionToResponse(sess, device, sess.ID == currentSessionID))
	}
	return out, nil
}

func (s *sessionService) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) (dto.MessageResponse, error) {
	sess, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.MessageResponse{}, apperrors.NewNotFound("session")
		}
		return dto.MessageResponse{}, apperrors.NewDatabase(err)
	}

	if sess.UserID != userID {
		return dto.MessageResponse{}, apperrors.NewForbidden("not authorized to revoke this session")
	}

	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)
		if err := s.sessions.Revoke(ctx, sessionID, opt); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return apperrors.NewNotFound("session")
			}
			return apperrors.NewDatabase(err)
		}
		return s.tokens.RevokeAllForSession(ctx, sessionID, opt)
	})
	if txErr != nil {
		return dto.MessageResponse{}, txErr
	}

	return dto.MessageResponse{Message: "session revoked"}, nil
}

func (s *sessionService) RevokeAllSessions(ctx context.Context, userID uuid.UUID) (dto.MessageResponse, error) {
	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)
		if err := s.sessions.RevokeAllForUser(ctx, userID, opt); err != nil {
			return apperrors.NewDatabase(err)
		}
		return s.tokens.RevokeAllForUser(ctx, userID, opt)
	})
	if txErr != nil {
		return dto.MessageResponse{}, txErr
	}

	return dto.MessageResponse{Message: "all sessions revoked"}, nil
}

// resolveDevice fetches device info for a session. Returns a minimal placeholder
// on ErrNotFound to handle the edge case of a device being revoked while its session
// is still active.
func (s *sessionService) resolveDevice(ctx context.Context, deviceID uuid.UUID) (*domain.Device, error) {
	device, err := s.devices.FindByID(ctx, deviceID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return &domain.Device{ID: deviceID}, nil
		}
		return nil, apperrors.NewDatabase(err)
	}
	return device, nil
}
