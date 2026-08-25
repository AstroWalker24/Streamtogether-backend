package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	autherrors "github.com/AstroWalker24/Streamtogether-backend/internal/auth/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/mapper"
	oauthpkg "github.com/AstroWalker24/Streamtogether-backend/internal/auth/oauth"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	redispkg "github.com/AstroWalker24/Streamtogether-backend/internal/redis"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/crypto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

const (
	oauthStateKeyPrefix = "oauth:state:"
	oauthLinkKeyPrefix  = "oauth:link:"
	oauthStateTTL       = 10 * time.Minute
	// maxUsernameLen is the maximum number of characters in a username (matches BR-05).
	maxUsernameLen = 30
)

// linkStatePayload is stored in Redis to bind an OAuth linking state to a specific user.
type linkStatePayload struct {
	UserID   string `json:"user_id"`
	Provider string `json:"provider"`
}

// GoogleOAuthService manages the Google OAuth 2.0 / OIDC login and account-linking flows.
type GoogleOAuthService interface {
	// InitiateLogin generates a secure state token, stores it in Redis, and
	// returns the Google authorization URL to which the client should redirect.
	InitiateLogin(ctx context.Context) (authURL string, err error)

	// HandleCallback validates the CSRF state, exchanges the authorization code,
	// resolves or provisions the application user, and returns a full AuthResponse.
	HandleCallback(ctx context.Context, code, state, ipAddress, userAgent string, deviceInput dto.OAuthCallbackInput) (dto.AuthResponse, error)

	// InitiateAccountLinking generates an OAuth state bound to the authenticated user
	// and returns the Google authorization URL. The user must be redirected there.
	// The state is stored in Redis with a 10-minute TTL and is single-use.
	InitiateAccountLinking(ctx context.Context, userID uuid.UUID) (authURL string, err error)

	// HandleLinkCallback validates the linking state, exchanges the authorization code,
	// verifies the Google identity, and attaches it to the authenticated user.
	// It is idempotent: linking the same identity twice returns success.
	HandleLinkCallback(ctx context.Context, code, state string, authenticatedUserID uuid.UUID) (dto.MessageResponse, error)
}

type googleOAuthService struct {
	provider   oauthpkg.Provider
	oauthIDs   authrepo.OAuthIdentityRepository
	users      UserService
	devices    authrepo.DeviceRepository
	sessions   authrepo.SessionRepository
	tokens     authrepo.RefreshTokenRepository
	tokenMgr   token.TokenManager
	rng        *random.Generator
	rdb        *redispkg.Redis
	db         *database.Database
	jwtCfg     config.JWTConfig
	maxDevices int // 0 = no limit
}

// NewGoogleOAuthService constructs a GoogleOAuthService with all required dependencies.
func NewGoogleOAuthService(
	provider oauthpkg.Provider,
	oauthIDs authrepo.OAuthIdentityRepository,
	users UserService,
	devices authrepo.DeviceRepository,
	sessions authrepo.SessionRepository,
	tokens authrepo.RefreshTokenRepository,
	tokenMgr token.TokenManager,
	rng *random.Generator,
	rdb *redispkg.Redis,
	db *database.Database,
	jwtCfg config.JWTConfig,
	maxDevices int,
) GoogleOAuthService {
	return &googleOAuthService{
		provider:   provider,
		oauthIDs:   oauthIDs,
		users:      users,
		devices:    devices,
		sessions:   sessions,
		tokens:     tokens,
		tokenMgr:   tokenMgr,
		rng:        rng,
		rdb:        rdb,
		db:         db,
		jwtCfg:     jwtCfg,
		maxDevices: maxDevices,
	}
}

// ─── InitiateLogin ────────────────────────────────────────────────────────────

func (s *googleOAuthService) InitiateLogin(ctx context.Context) (string, error) {
	state, err := oauthpkg.GenerateState(s.rng)
	if err != nil {
		return "", apperrors.NewInternal("OAuth state generation failed", err)
	}

	key := oauthStateKeyPrefix + state
	if err := s.rdb.Set(ctx, key, "1", oauthStateTTL); err != nil {
		return "", apperrors.NewRedis(err)
	}

	return s.provider.BuildAuthorizationURL(state), nil
}

// ─── HandleCallback ───────────────────────────────────────────────────────────

func (s *googleOAuthService) HandleCallback(ctx context.Context, code, state, ipAddress, userAgent string, deviceInput dto.OAuthCallbackInput) (dto.AuthResponse, error) {
	// 1. Validate CSRF state (Redis lookup + single-use consumption).
	if err := s.validateAndConsumeState(ctx, state); err != nil {
		return dto.AuthResponse{}, err
	}

	// 2. Exchange authorization code for a verified provider identity.
	identity, err := s.provider.ExchangeCode(ctx, code, "")
	if err != nil {
		return dto.AuthResponse{}, err
	}

	// 3. Resolve or provision the application user.
	u, err := s.resolveUser(ctx, identity)
	if err != nil {
		return dto.AuthResponse{}, err
	}

	// 4. Enforce account status.
	switch u.Status {
	case domain.UserStatusActive:
		// proceed
	case domain.UserStatusSuspended:
		return dto.AuthResponse{}, autherrors.NewAccountSuspended()
	case domain.UserStatusDeleted:
		return dto.AuthResponse{}, autherrors.NewAccountDeleted()
	default:
		// pending_verification is not possible for OAuth users (set to active at creation).
		return dto.AuthResponse{}, apperrors.NewInternal("unknown account status")
	}

	// 5. Create device, session, and token (mirrors loginService).
	return s.createAuthSession(ctx, u, ipAddress, userAgent, identity, deviceInput)
}

// ─── CSRF state validation ────────────────────────────────────────────────────

func (s *googleOAuthService) validateAndConsumeState(ctx context.Context, state string) error {
	if state == "" {
		return autherrors.NewOAuthStateInvalid()
	}
	key := oauthStateKeyPrefix + state
	n, err := s.rdb.Exists(ctx, key)
	if err != nil {
		return apperrors.NewRedis(err)
	}
	if n == 0 {
		return autherrors.NewOAuthStateInvalid()
	}
	// Consume the state so it cannot be replayed.
	if _, err := s.rdb.Delete(ctx, key); err != nil {
		return apperrors.NewRedis(err)
	}
	return nil
}

// ─── User resolution / provisioning ─────────────────────────────────────────

func (s *googleOAuthService) resolveUser(ctx context.Context, identity oauthpkg.ProviderIdentity) (*domain.User, error) {
	// Try to find an existing OAuth identity link.
	oauthID, err := s.oauthIDs.FindByProvider(ctx, identity.Provider, identity.ProviderUserID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return nil, apperrors.NewDatabase(err)
	}
	if err == nil {
		// Known identity — return the linked user.
		return s.users.GetByID(ctx, oauthID.UserID)
	}

	// Unknown provider identity — check if the email is already registered.
	// Google verifies email ownership before setting email_verified=true, but we
	// deliberately do NOT auto-link by email (security boundary). Instead we surface
	// a conflict error so the user can link manually via the account settings flow.
	if identity.Email != "" {
		available, err := s.users.IsEmailAvailable(ctx, identity.Email)
		if err != nil {
			return nil, err
		}
		if !available {
			return nil, autherrors.NewOAuthEmailConflict()
		}
	}

	// Provision a new user + identity link.
	return s.provisionUser(ctx, identity)
}

func (s *googleOAuthService) provisionUser(ctx context.Context, identity oauthpkg.ProviderIdentity) (*domain.User, error) {
	username, err := s.generateUsername(ctx, identity)
	if err != nil {
		return nil, err
	}

	newUser, err := s.users.CreateOAuthUser(ctx, identity.Email, username, identity.EmailVerified)
	if err != nil {
		return nil, err
	}

	link := &domain.OAuthIdentity{
		ID:             uuid.New(),
		UserID:         newUser.ID,
		Provider:       identity.Provider,
		ProviderUserID: identity.ProviderUserID,
	}
	if _, err := s.oauthIDs.Create(ctx, link); err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	return newUser, nil
}

// generateUsername derives a unique username from the provider identity.
// Preference order: DisplayName → email local-part → random fallback.
// If the candidate is already taken a 6-character hex suffix is appended.
func (s *googleOAuthService) generateUsername(ctx context.Context, identity oauthpkg.ProviderIdentity) (string, error) {
	base := cleanUsername(identity.DisplayName)
	if base == "" {
		// Fall back to the local-part of the email address.
		if idx := strings.IndexByte(identity.Email, '@'); idx > 0 {
			base = cleanUsername(identity.Email[:idx])
		}
	}
	if base == "" {
		base = "user"
	}

	available, err := s.users.IsUsernameAvailable(ctx, base)
	if err != nil {
		return "", err
	}
	if available {
		return base, nil
	}

	// Append a random hex suffix to resolve the collision.
	suffix, err := s.rng.Hex(3) // 6 hex chars
	if err != nil {
		return "", apperrors.NewInternal("username suffix generation failed", err)
	}
	candidate := base + "_" + suffix
	// Trim to maxUsernameLen in case base is near the limit.
	if len(candidate) > maxUsernameLen {
		candidate = base[:maxUsernameLen-8] + "_" + suffix
	}
	return candidate, nil
}

// cleanUsername converts a display name to a safe, lowercase username:
//   - lowercases all characters
//   - replaces spaces and hyphens with underscores
//   - removes characters that are not alphanumeric or underscore
//   - trims leading/trailing underscores
//   - truncates to maxUsernameLen
func cleanUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteByte('_')
		}
	}
	result := strings.Trim(b.String(), "_")
	if len(result) > maxUsernameLen {
		result = result[:maxUsernameLen]
	}
	return result
}

// ─── Device / session / token creation (mirrors loginService) ────────────────

func (s *googleOAuthService) createAuthSession(
	ctx context.Context,
	u *domain.User,
	ipAddress, userAgent string,
	identity oauthpkg.ProviderIdentity,
	d dto.OAuthCallbackInput,
) (dto.AuthResponse, error) {

	// Derive device fingerprint: use caller-supplied value or derive from identity+UA.
	fingerprint := d.DeviceFingerprint
	if fingerprint == "" {
		fingerprint = identity.Provider + ":" + identity.ProviderUserID + ":" + userAgent
	}
	fingerprintHash := crypto.SHA256Hex([]byte(fingerprint))

	// Find existing device or prepare to register a new one.
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

	// Generate refresh token before the transaction (pure, no I/O).
	plainRT, err := s.rng.Base64URL(32)
	if err != nil {
		return dto.AuthResponse{}, apperrors.NewInternal("refresh token generation failed", err)
	}
	rtHash := crypto.SHA256Hex([]byte(plainRT))

	// Calculate expiry.
	now := time.Now().UTC()
	rtExpiry := s.jwtCfg.RefreshTokenExpiry
	if d.RememberMe && s.jwtCfg.RefreshTokenRememberMeExpiry > 0 {
		rtExpiry = s.jwtCfg.RefreshTokenRememberMeExpiry
	}
	expiresAt := now.Add(rtExpiry)

	// Atomically persist device, session, and refresh token.
	var finalDevice *domain.Device
	var createdSession *domain.Session

	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)

		if isNewDevice {
			platform := domain.DevicePlatformWeb
			if d.Platform != "" {
				platform = domain.DevicePlatform(d.Platform)
			}
			friendlyName := d.FriendlyName
			if friendlyName == "" {
				friendlyName = "Unknown device"
			}
			newDevice := &domain.Device{
				ID:              uuid.New(),
				UserID:          u.ID,
				FingerprintHash: fingerprintHash,
				FriendlyName:    friendlyName,
				Platform:        platform,
				Browser:         d.Browser,
				OS:              d.OS,
				LastIPAddress:   ipAddress,
			}
			registered, err := s.devices.Register(ctx, newDevice, opt)
			if err != nil {
				return apperrors.NewDatabase(err)
			}
			finalDevice = registered
		} else {
			existingDevice.Browser = d.Browser
			existingDevice.OS = d.OS
			existingDevice.LastIPAddress = ipAddress
			if d.FriendlyName != "" {
				existingDevice.FriendlyName = d.FriendlyName
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
			RememberMe: d.RememberMe,
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

	// Issue access token (no DB calls; pure JWT signing).
	accessToken, err := s.tokenMgr.CreateAccessToken(token.CreateAccessTokenInput{
		UserID:    u.ID,
		SessionID: createdSession.ID,
		Roles:     nil,
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

// ─── Account Linking ──────────────────────────────────────────────────────────

func (s *googleOAuthService) InitiateAccountLinking(ctx context.Context, userID uuid.UUID) (string, error) {
	state, err := oauthpkg.GenerateState(s.rng)
	if err != nil {
		return "", apperrors.NewInternal("OAuth state generation failed", err)
	}

	payload := linkStatePayload{UserID: userID.String(), Provider: s.provider.Name()}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", apperrors.NewInternal("OAuth link state serialization failed", err)
	}

	key := oauthLinkKeyPrefix + state
	if err := s.rdb.Set(ctx, key, string(raw), oauthStateTTL); err != nil {
		return "", apperrors.NewRedis(err)
	}

	return s.provider.BuildAuthorizationURL(state), nil
}

func (s *googleOAuthService) HandleLinkCallback(ctx context.Context, code, state string, authenticatedUserID uuid.UUID) (dto.MessageResponse, error) {
	// 1. Validate and consume the linking state (CSRF + ownership check).
	if err := s.validateAndConsumeLinkState(ctx, state, authenticatedUserID); err != nil {
		return dto.MessageResponse{}, err
	}

	// 2. Exchange authorization code for a verified Google identity.
	identity, err := s.provider.ExchangeCode(ctx, code, "")
	if err != nil {
		return dto.MessageResponse{}, err
	}

	// 3. Check whether this provider identity is already linked.
	existing, findErr := s.oauthIDs.FindByProvider(ctx, identity.Provider, identity.ProviderUserID)
	if findErr != nil && !errors.Is(findErr, repo.ErrNotFound) {
		return dto.MessageResponse{}, apperrors.NewDatabase(findErr)
	}

	if findErr == nil {
		// Identity record found.
		if existing.UserID == authenticatedUserID {
			// Idempotent: already linked to this user.
			return dto.MessageResponse{Message: "account already linked"}, nil
		}
		// Security: identity belongs to a different user — reject unconditionally.
		return dto.MessageResponse{}, autherrors.NewOAuthIdentityOwnedByOther()
	}

	// 4. Persist the new identity link.
	link := &domain.OAuthIdentity{
		ID:             uuid.New(),
		UserID:         authenticatedUserID,
		Provider:       identity.Provider,
		ProviderUserID: identity.ProviderUserID,
	}
	if _, err := s.oauthIDs.Create(ctx, link); err != nil {
		return dto.MessageResponse{}, apperrors.NewDatabase(err)
	}

	// 5. Promote email verification when Google confirms the matching email.
	// Only update — never downgrade an already-verified email.
	if identity.EmailVerified && identity.Email != "" {
		u, err := s.users.GetByID(ctx, authenticatedUserID)
		if err == nil && !u.EmailVerified && strings.EqualFold(u.Email, identity.Email) {
			_ = s.users.UpdateEmailVerified(ctx, authenticatedUserID, true)
		}
	}

	return dto.MessageResponse{Message: "account linked successfully"}, nil
}

// validateAndConsumeLinkState validates the linking state, verifies it was
// initiated by authenticatedUserID, and atomically consumes it (single-use).
func (s *googleOAuthService) validateAndConsumeLinkState(ctx context.Context, state string, authenticatedUserID uuid.UUID) error {
	if state == "" {
		return autherrors.NewOAuthStateInvalid()
	}

	key := oauthLinkKeyPrefix + state

	n, err := s.rdb.Exists(ctx, key)
	if err != nil {
		return apperrors.NewRedis(err)
	}
	if n == 0 {
		return autherrors.NewOAuthStateInvalid()
	}

	raw, err := s.rdb.Get(ctx, key)
	if err != nil {
		// Key expired between Exists and Get — treat as invalid state.
		return autherrors.NewOAuthStateInvalid()
	}

	var payload linkStatePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return autherrors.NewOAuthStateInvalid()
	}

	// Verify the state was created by the currently authenticated user.
	if payload.UserID != authenticatedUserID.String() {
		// Consume the state to prevent the legitimate owner from being confused.
		_, _ = s.rdb.Delete(ctx, key)
		return apperrors.NewForbidden("OAuth state was not initiated by the current user")
	}

	// Consume the state — check deletion count to handle concurrent callbacks.
	deleted, err := s.rdb.Delete(ctx, key)
	if err != nil {
		return apperrors.NewRedis(err)
	}
	if deleted == 0 {
		// Race: another concurrent request consumed the state first.
		return autherrors.NewOAuthStateInvalid()
	}

	return nil
}
