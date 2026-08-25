package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	autherrors "github.com/AstroWalker24/Streamtogether-backend/internal/auth/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/mapper"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/crypto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// RefreshService handles refresh-token rotation and new access-token issuance.
type RefreshService interface {
	// Refresh validates the presented refresh token, rotates it, and returns
	// a new token pair bound to the same session and user.
	Refresh(ctx context.Context, req dto.RefreshRequest) (dto.AuthResponse, error)
}

type refreshService struct {
	tokens   authrepo.RefreshTokenRepository
	sessions authrepo.SessionRepository
	devices  authrepo.DeviceRepository
	users    UserService
	tokenMgr token.TokenManager
	rng      *random.Generator
	db       *database.Database
	jwtCfg   config.JWTConfig
}

// NewRefreshService constructs a RefreshService with explicit dependencies.
func NewRefreshService(
	tokens authrepo.RefreshTokenRepository,
	sessions authrepo.SessionRepository,
	devices authrepo.DeviceRepository,
	users UserService,
	tokenMgr token.TokenManager,
	rng *random.Generator,
	db *database.Database,
	jwtCfg config.JWTConfig,
) RefreshService {
	return &refreshService{
		tokens:   tokens,
		sessions: sessions,
		devices:  devices,
		users:    users,
		tokenMgr: tokenMgr,
		rng:      rng,
		db:       db,
		jwtCfg:   jwtCfg,
	}
}

func (s *refreshService) Refresh(ctx context.Context, req dto.RefreshRequest) (dto.AuthResponse, error) {
	// 1. Hash the raw token to perform the repository lookup.
	tokenHash := crypto.SHA256Hex([]byte(req.RefreshToken))

	// 2. Locate the active token record (unconsumed, unrevoked, unexpired).
	rt, err := s.tokens.FindActiveByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.AuthResponse{}, autherrors.NewTokenInvalid()
		}
		return dto.AuthResponse{}, apperrors.NewDatabase(err)
	}

	// 3. Resolve the associated session.
	sess, err := s.sessions.FindByID(ctx, rt.SessionID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// Session is revoked or expired; the refresh token is no longer usable.
			return dto.AuthResponse{}, autherrors.NewSessionRevoked()
		}
		return dto.AuthResponse{}, apperrors.NewDatabase(err)
	}

	// 4. Resolve the authenticated user from the token's denormalized UserID.
	u, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return dto.AuthResponse{}, err
	}

	// 5. Generate the replacement refresh token before the transaction (no I/O).
	plainRT, err := s.rng.Base64URL(32) // 256 bits of entropy
	if err != nil {
		return dto.AuthResponse{}, apperrors.NewInternal("refresh token generation failed", err)
	}
	newTokenHash := crypto.SHA256Hex([]byte(plainRT))
	newTokenID := uuid.New()

	// New token inherits the session RememberMe policy but never outlives the session.
	rtExpiry := s.jwtCfg.RefreshTokenExpiry
	if sess.RememberMe && s.jwtCfg.RefreshTokenRememberMeExpiry > 0 {
		rtExpiry = s.jwtCfg.RefreshTokenRememberMeExpiry
	}
	candidateExpiry := time.Now().UTC().Add(rtExpiry)
	newExpiry := minTime(candidateExpiry, sess.ExpiresAt)

	// 6. Atomically: consume old token, persist successor, update session activity.
	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)

		if err := s.tokens.MarkConsumed(ctx, rt.ID, newTokenID, opt); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				// Lost a race — another request already consumed this token.
				return autherrors.NewTokenConsumed()
			}
			return apperrors.NewDatabase(err)
		}

		newRT := &domain.RefreshToken{
			ID:        newTokenID,
			SessionID: rt.SessionID,
			UserID:    rt.UserID,
			DeviceID:  rt.DeviceID,
			TokenHash: newTokenHash,
			ExpiresAt: newExpiry,
		}
		if _, err := s.tokens.Create(ctx, newRT, opt); err != nil {
			return apperrors.NewDatabase(err)
		}

		return s.sessions.UpdateLastActive(ctx, rt.SessionID, opt)
	})
	if txErr != nil {
		return dto.AuthResponse{}, txErr
	}

	// 7. Issue the new access token (no DB calls).
	accessToken, err := s.tokenMgr.CreateAccessToken(token.CreateAccessTokenInput{
		UserID:    rt.UserID,
		SessionID: rt.SessionID,
		Roles:     nil,
	})
	if err != nil {
		return dto.AuthResponse{}, apperrors.NewInternal("access token creation failed", err)
	}

	// 8. Fetch the device for the session response.
	device, err := s.resolveDevice(ctx, sess.DeviceID)
	if err != nil {
		return dto.AuthResponse{}, err
	}

	return dto.AuthResponse{
		TokenPair: dto.TokenPair{
			AccessToken:           accessToken.Token,
			AccessTokenExpiresAt:  accessToken.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			RefreshToken:          plainRT,
			RefreshTokenExpiresAt: newExpiry.UTC().Format("2006-01-02T15:04:05Z07:00"),
			TokenType:             "Bearer",
		},
		User:    mapper.UserToResponse(u),
		Session: mapper.SessionToResponse(sess, device, true),
	}, nil
}

// resolveDevice fetches a device by ID, returning a minimal placeholder on ErrNotFound.
func (s *refreshService) resolveDevice(ctx context.Context, deviceID uuid.UUID) (*domain.Device, error) {
	device, err := s.devices.FindByID(ctx, deviceID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return &domain.Device{ID: deviceID}, nil
		}
		return nil, apperrors.NewDatabase(err)
	}
	return device, nil
}

// minTime returns the earlier of two times.
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
