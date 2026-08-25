package service

import (
	"context"
	"errors"
	"strings"
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
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/password"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// LoginService handles the user authentication flow.
type LoginService interface {
	// Login authenticates a user and returns a full authentication response.
	// ipAddress and userAgent are sourced from the HTTP request by the handler.
	Login(ctx context.Context, req dto.LoginRequest, ipAddress, userAgent string) (dto.AuthResponse, error)
}

type loginService struct {
	users      UserService
	devices    authrepo.DeviceRepository
	sessions   authrepo.SessionRepository
	tokens     authrepo.RefreshTokenRepository
	tokenMgr   token.TokenManager
	hasher     *password.Hasher
	rng        *random.Generator
	db         *database.Database
	jwtCfg     config.JWTConfig
	maxDevices int // 0 = no limit
}

// NewLoginService constructs a LoginService with all required dependencies.
func NewLoginService(
	users UserService,
	devices authrepo.DeviceRepository,
	sessions authrepo.SessionRepository,
	tokens authrepo.RefreshTokenRepository,
	tokenMgr token.TokenManager,
	hasher *password.Hasher,
	rng *random.Generator,
	db *database.Database,
	jwtCfg config.JWTConfig,
	maxDevices int,
) LoginService {
	return &loginService{
		users:      users,
		devices:    devices,
		sessions:   sessions,
		tokens:     tokens,
		tokenMgr:   tokenMgr,
		hasher:     hasher,
		rng:        rng,
		db:         db,
		jwtCfg:     jwtCfg,
		maxDevices: maxDevices,
	}
}

func (s *loginService) Login(ctx context.Context, req dto.LoginRequest, ipAddress, userAgent string) (dto.AuthResponse, error) {
	req.Normalize()

	// 1. Locate user by email or username.
	var u *domain.User
	var err error
	if strings.ContainsRune(req.Identifier, '@') {
		u, err = s.users.GetByEmail(ctx, req.Identifier)
	} else {
		u, err = s.users.GetByUsername(ctx, req.Identifier)
	}
	if err != nil {
		// Always return AUTH_INVALID_CREDENTIALS to prevent user enumeration.
		return dto.AuthResponse{}, autherrors.NewInvalidCredentials()
	}

	// 2. Verify password. Never log the plaintext or the hash.
	ok, err := s.hasher.Verify(req.Password, u.PasswordHash)
	if err != nil {
		return dto.AuthResponse{}, apperrors.NewInternal("password verification failed", err)
	}
	if !ok {
		return dto.AuthResponse{}, autherrors.NewInvalidCredentials()
	}

	// 3. Enforce account status (INV-09, INV-18).
	switch u.Status {
	case domain.UserStatusActive:
		// proceed
	case domain.UserStatusPendingVerification:
		return dto.AuthResponse{}, autherrors.NewEmailNotVerified()
	case domain.UserStatusSuspended:
		return dto.AuthResponse{}, autherrors.NewAccountSuspended()
	case domain.UserStatusDeleted:
		return dto.AuthResponse{}, autherrors.NewAccountDeleted()
	default:
		return dto.AuthResponse{}, apperrors.NewInternal("unknown account status")
	}

	// 4. Resolve device.
	fingerprintHash := crypto.SHA256Hex([]byte(req.DeviceFingerprint))

	existingDevice, lookupErr := s.devices.FindActiveByFingerprint(ctx, u.ID, fingerprintHash)
	if lookupErr != nil && !errors.Is(lookupErr, repo.ErrNotFound) {
		return dto.AuthResponse{}, apperrors.NewDatabase(lookupErr)
	}
	isNewDevice := errors.Is(lookupErr, repo.ErrNotFound)

	if isNewDevice {
		count, err := s.devices.CountActiveByUser(ctx, u.ID)
		if err != nil {
			return dto.AuthResponse{}, apperrors.NewDatabase(err)
		}
		if s.maxDevices > 0 && count >= s.maxDevices {
			return dto.AuthResponse{}, autherrors.NewDeviceLimitReached()
		}
	}

	// 5. Generate refresh token plaintext before the transaction (pure, no I/O).
	plainRT, err := s.rng.Base64URL(32) // 256 bits of entropy
	if err != nil {
		return dto.AuthResponse{}, apperrors.NewInternal("refresh token generation failed", err)
	}
	rtHash := crypto.SHA256Hex([]byte(plainRT))

	// 6. Calculate expiry.
	now := time.Now().UTC()
	rtExpiry := s.jwtCfg.RefreshTokenExpiry
	if req.RememberMe && s.jwtCfg.RefreshTokenRememberMeExpiry > 0 {
		rtExpiry = s.jwtCfg.RefreshTokenRememberMeExpiry
	}
	expiresAt := now.Add(rtExpiry)

	// 7. Atomically persist device, session, and refresh token.
	var finalDevice *domain.Device
	var createdSession *domain.Session

	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)

		if isNewDevice {
			platform := domain.DevicePlatformWeb
			if req.Platform != "" {
				platform = domain.DevicePlatform(req.Platform)
			}
			friendlyName := req.DeviceName
			if friendlyName == "" {
				friendlyName = "Unknown device"
			}
			d := &domain.Device{
				ID:              uuid.New(),
				UserID:          u.ID,
				FingerprintHash: fingerprintHash,
				FriendlyName:    friendlyName,
				Platform:        platform,
				Browser:         req.Browser,
				OS:              req.OS,
				LastIPAddress:   ipAddress,
			}
			registered, err := s.devices.Register(ctx, d, opt)
			if err != nil {
				return apperrors.NewDatabase(err)
			}
			finalDevice = registered
		} else {
			// Update mutable metadata on the returning device.
			existingDevice.Browser = req.Browser
			existingDevice.OS = req.OS
			existingDevice.LastIPAddress = ipAddress
			if req.DeviceName != "" {
				existingDevice.FriendlyName = req.DeviceName
			}
			updated, err := s.devices.UpdateMetadata(ctx, existingDevice, opt)
			if err != nil {
				return apperrors.NewDatabase(err)
			}
			if err := s.devices.UpdateLastActive(ctx, updated.ID, ipAddress, opt); err != nil {
				return apperrors.NewDatabase(err)
			}
			finalDevice = updated
		}

		sess := &domain.Session{
			ID:         uuid.New(),
			UserID:     u.ID,
			DeviceID:   finalDevice.ID,
			IPAddress:  ipAddress,
			UserAgent:  userAgent,
			ExpiresAt:  expiresAt,
			RememberMe: req.RememberMe,
		}
		created, err := s.sessions.Create(ctx, sess, opt)
		if err != nil {
			return apperrors.NewDatabase(err)
		}
		createdSession = created

		rt := &domain.RefreshToken{
			ID:        uuid.New(),
			SessionID: createdSession.ID,
			UserID:    u.ID,
			DeviceID:  finalDevice.ID,
			TokenHash: rtHash,
			ExpiresAt: expiresAt,
		}
		if _, err := s.tokens.Create(ctx, rt, opt); err != nil {
			return apperrors.NewDatabase(err)
		}
		return nil
	})
	if txErr != nil {
		return dto.AuthResponse{}, txErr
	}

	// 8. Issue access token (no DB calls; pure JWT signing).
	accessToken, err := s.tokenMgr.CreateAccessToken(token.CreateAccessTokenInput{
		UserID:    u.ID,
		SessionID: createdSession.ID,
		Roles:     nil, // role resolution belongs to a later step
	})
	if err != nil {
		return dto.AuthResponse{}, apperrors.NewInternal("access token creation failed", err)
	}

	return dto.AuthResponse{
		TokenPair: dto.TokenPair{
			AccessToken:           accessToken.Token,
			AccessTokenExpiresAt:  accessToken.ExpiresAt.UTC().Format(time.RFC3339),
			RefreshToken:          plainRT,
			RefreshTokenExpiresAt: expiresAt.UTC().Format(time.RFC3339),
			TokenType:             "Bearer",
		},
		User:    mapper.UserToResponse(u),
		Session: mapper.SessionToResponse(createdSession, finalDevice, true),
	}, nil
}
