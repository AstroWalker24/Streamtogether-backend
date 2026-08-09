package token_test

// Note: testJWTConfig() and newTestManager() (returns *jwtpkg.Manager) are
// defined in claims_test.go and are visible here because both files share
// package token_test.

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	jwtpkg "github.com/AstroWalker24/Streamtogether-backend/internal/security/jwt"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// ── test helpers ──────────────────────────────────────────────────────────────

// testInput returns a CreateAccessTokenInput with valid, unique identifiers.
func testInput() token.CreateAccessTokenInput {
	return token.CreateAccessTokenInput{
		UserID:    uuid.New(),
		SessionID: uuid.New(),
		Roles:     []string{"user"},
	}
}

// newTestTokenManager creates a *token.Manager using testJWTConfig and a real
// jwt.Manager (both defined in / delegating to claims_test.go helpers).
func newTestTokenManager(t *testing.T) *token.Manager {
	t.Helper()
	mgr, err := token.New(newTestManager(t), testJWTConfig())
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}
	return mgr
}

// mustCreate issues an access token and fails the test on any error.
func mustCreate(t *testing.T, mgr *token.Manager, in token.CreateAccessTokenInput) token.AccessToken {
	t.Helper()
	at, err := mgr.CreateAccessToken(in)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	return at
}

// mustValidate validates a token string and fails the test on any error.
func mustValidate(t *testing.T, mgr *token.Manager, tokenStr string) token.AccessTokenClaims {
	t.Helper()
	claims, err := mgr.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	return claims
}

// ── Construction ──────────────────────────────────────────────────────────────

func TestManagerNew_Valid(t *testing.T) {
	if _, err := token.New(newTestManager(t), testJWTConfig()); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestManagerNew_NilJWTManager(t *testing.T) {
	_, err := token.New(nil, testJWTConfig())
	if err == nil {
		t.Fatal("expected error for nil jwt manager, got nil")
	}
}

func TestManagerNewWithClock_NilClock(t *testing.T) {
	_, err := token.NewWithClock(newTestManager(t), testJWTConfig(), nil)
	if err == nil {
		t.Fatal("expected error for nil clock, got nil")
	}
}

func TestManagerNew_ZeroExpiry(t *testing.T) {
	cfg := testJWTConfig()
	cfg.AccessTokenExpiry = 0
	_, err := token.New(newTestManager(t), cfg)
	if err == nil {
		t.Fatal("expected error for zero access token expiry, got nil")
	}
}

// ── Interface compliance ───────────────────────────────────────────────────────

func TestManager_ImplementsTokenManager(t *testing.T) {
	// Compile-time assertion: *Manager satisfies the TokenManager interface.
	var _ token.TokenManager = (*token.Manager)(nil)
}

// ── CreateAccessToken: valid ───────────────────────────────────────────────────

func TestCreateAccessToken_ValidInput(t *testing.T) {
	at, err := newTestTokenManager(t).CreateAccessToken(testInput())
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	if at.Token == "" {
		t.Error("Token must not be empty")
	}
	if at.ExpiresAt.IsZero() {
		t.Error("ExpiresAt must not be zero")
	}
}

func TestCreateAccessToken_ExpiryMatchesConfig(t *testing.T) {
	cfg := testJWTConfig() // AccessTokenExpiry = 15 minutes
	fixedNow := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

	mgr, err := token.NewWithClock(newTestManager(t), cfg, func() time.Time { return fixedNow })
	if err != nil {
		t.Fatalf("NewWithClock: %v", err)
	}

	at := mustCreate(t, mgr, testInput())

	want := fixedNow.Add(cfg.AccessTokenExpiry)
	if !at.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt: got %v, want %v", at.ExpiresAt, want)
	}
}

func TestCreateAccessToken_NilUserID(t *testing.T) {
	in := testInput()
	in.UserID = uuid.Nil
	_, err := newTestTokenManager(t).CreateAccessToken(in)
	if !errors.Is(err, token.ErrAccessTokenCreation) {
		t.Fatalf("expected ErrAccessTokenCreation, got %v", err)
	}
}

func TestCreateAccessToken_NilSessionID(t *testing.T) {
	in := testInput()
	in.SessionID = uuid.Nil
	_, err := newTestTokenManager(t).CreateAccessToken(in)
	if !errors.Is(err, token.ErrAccessTokenCreation) {
		t.Fatalf("expected ErrAccessTokenCreation, got %v", err)
	}
}

func TestCreateAccessToken_NilRolesAccepted(t *testing.T) {
	in := testInput()
	in.Roles = nil
	if _, err := newTestTokenManager(t).CreateAccessToken(in); err != nil {
		t.Fatalf("CreateAccessToken with nil Roles: %v", err)
	}
}

func TestCreateAccessToken_UniqueJTI(t *testing.T) {
	mgr := newTestTokenManager(t)
	in := testInput()

	claims1 := mustValidate(t, mgr, mustCreate(t, mgr, in).Token)
	claims2 := mustValidate(t, mgr, mustCreate(t, mgr, in).Token)

	if claims1.TokenID == claims2.TokenID {
		t.Errorf("JTI must be unique per token; both have TokenID %v", claims1.TokenID)
	}
}

// ── ValidateAccessToken: valid ─────────────────────────────────────────────────

func TestValidateAccessToken_UserIDRoundTrip(t *testing.T) {
	mgr := newTestTokenManager(t)
	in := testInput()
	claims := mustValidate(t, mgr, mustCreate(t, mgr, in).Token)

	if claims.UserID != in.UserID {
		t.Errorf("UserID: got %v, want %v", claims.UserID, in.UserID)
	}
}

func TestValidateAccessToken_SessionIDRoundTrip(t *testing.T) {
	mgr := newTestTokenManager(t)
	in := testInput()
	claims := mustValidate(t, mgr, mustCreate(t, mgr, in).Token)

	if claims.SessionID != in.SessionID {
		t.Errorf("SessionID: got %v, want %v", claims.SessionID, in.SessionID)
	}
}

func TestValidateAccessToken_TokenIDPresent(t *testing.T) {
	mgr := newTestTokenManager(t)
	claims := mustValidate(t, mgr, mustCreate(t, mgr, testInput()).Token)

	if claims.TokenID == uuid.Nil {
		t.Error("TokenID must not be the nil UUID after round-trip")
	}
}

func TestValidateAccessToken_RolesRoundTrip(t *testing.T) {
	mgr := newTestTokenManager(t)
	in := testInput()
	in.Roles = []string{"user", "moderator"}
	claims := mustValidate(t, mgr, mustCreate(t, mgr, in).Token)

	if len(claims.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d: %v", len(claims.Roles), claims.Roles)
	}
	roleSet := map[string]bool{claims.Roles[0]: true, claims.Roles[1]: true}
	for _, r := range in.Roles {
		if !roleSet[r] {
			t.Errorf("role %q missing after round-trip", r)
		}
	}
}

func TestValidateAccessToken_ExpiresAtRoundTrip(t *testing.T) {
	cfg := testJWTConfig()
	fixedNow := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	want := fixedNow.Add(cfg.AccessTokenExpiry)

	mgr, err := token.NewWithClock(newTestManager(t), cfg, func() time.Time { return fixedNow })
	if err != nil {
		t.Fatalf("NewWithClock: %v", err)
	}

	claims := mustValidate(t, mgr, mustCreate(t, mgr, testInput()).Token)

	// Unix timestamp truncation may cause ±1 s drift.
	diff := claims.ExpiresAt.Sub(want)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("ExpiresAt drift: got %v, want ~%v (diff %v)", claims.ExpiresAt, want, diff)
	}
}

// ── ValidateAccessToken: error cases ──────────────────────────────────────────

func TestValidateAccessToken_ExpiredToken(t *testing.T) {
	cfg := testJWTConfig() // AccessTokenExpiry = 15 minutes
	rng := random.New()

	// jwt.Manager validates using a clock 1 hour ahead of real time.
	// Any token with exp = now+15m appears expired to this manager.
	futureJWT, err := jwtpkg.NewWithClock(cfg, rng, func() time.Time {
		return time.Now().Add(time.Hour)
	})
	if err != nil {
		t.Fatalf("jwtpkg.NewWithClock: %v", err)
	}

	// token.Manager uses the real clock for issuance (exp = now + 15 min).
	mgr, err := token.New(futureJWT, cfg)
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}

	at := mustCreate(t, mgr, testInput())

	_, err = mgr.ValidateAccessToken(at.Token)
	if !errors.Is(err, token.ErrAccessTokenExpired) {
		t.Fatalf("expected ErrAccessTokenExpired, got %v", err)
	}
}

func TestValidateAccessToken_InvalidSignature(t *testing.T) {
	rng := random.New()

	cfgA := testJWTConfig()
	cfgA.Secret = "secret-A-at-least-32-chars-long-aaa"
	jwtA, _ := jwtpkg.New(cfgA, rng)
	mgrA, _ := token.New(jwtA, cfgA)

	cfgB := testJWTConfig()
	cfgB.Secret = "secret-B-at-least-32-chars-long-bbb"
	jwtB, _ := jwtpkg.New(cfgB, rng)
	mgrB, _ := token.New(jwtB, cfgB)

	// Token signed with secret A, validated with secret B.
	at := mustCreate(t, mgrA, testInput())

	_, err := mgrB.ValidateAccessToken(at.Token)
	if !errors.Is(err, token.ErrInvalidAccessTokenSignature) {
		t.Fatalf("expected ErrInvalidAccessTokenSignature, got %v", err)
	}
}

func TestValidateAccessToken_MalformedToken(t *testing.T) {
	_, err := newTestTokenManager(t).ValidateAccessToken("not-a-jwt")
	if !errors.Is(err, token.ErrAccessTokenMalformed) {
		t.Fatalf("expected ErrAccessTokenMalformed, got %v", err)
	}
}

func TestValidateAccessToken_EmptyToken(t *testing.T) {
	_, err := newTestTokenManager(t).ValidateAccessToken("")
	if err == nil {
		t.Fatal("expected error for empty token string, got nil")
	}
}

func TestValidateAccessToken_TamperedSignature(t *testing.T) {
	mgr := newTestTokenManager(t)
	tok := mustCreate(t, mgr, testInput()).Token

	// Flip the last character of the signature segment.
	last := tok[len(tok)-1]
	var tampered string
	if last == 'A' {
		tampered = tok[:len(tok)-1] + "B"
	} else {
		tampered = tok[:len(tok)-1] + "A"
	}

	_, err := mgr.ValidateAccessToken(tampered)
	if !errors.Is(err, token.ErrInvalidAccessTokenSignature) &&
		!errors.Is(err, token.ErrInvalidAccessToken) {
		t.Fatalf("expected signature error for tampered token, got %v", err)
	}
}

func TestValidateAccessToken_InvalidIssuer(t *testing.T) {
	rng := random.New()

	// Issue a token with no issuer using the SAME secret (so signature is valid).
	noIssCfg := testJWTConfig()
	noIssCfg.Issuer = ""
	noIssJWT, _ := jwtpkg.New(noIssCfg, rng)
	noIssMgr, _ := token.New(noIssJWT, noIssCfg)
	at := mustCreate(t, noIssMgr, testInput())

	// Validate with a manager that requires Issuer = "streamtogether-test".
	mainJWT, _ := jwtpkg.New(testJWTConfig(), rng)
	mainMgr, _ := token.New(mainJWT, testJWTConfig())

	_, err := mainMgr.ValidateAccessToken(at.Token)
	if !errors.Is(err, token.ErrInvalidAccessToken) {
		t.Fatalf("expected ErrInvalidAccessToken for wrong issuer, got %v", err)
	}
}

func TestValidateAccessToken_InvalidAudience(t *testing.T) {
	rng := random.New()

	// Issue a token with no audience using the SAME secret.
	noAudCfg := testJWTConfig()
	noAudCfg.Audience = ""
	noAudJWT, _ := jwtpkg.New(noAudCfg, rng)
	noAudMgr, _ := token.New(noAudJWT, noAudCfg)
	at := mustCreate(t, noAudMgr, testInput())

	// Validate with a manager that requires Audience = "st-clients".
	mainJWT, _ := jwtpkg.New(testJWTConfig(), rng)
	mainMgr, _ := token.New(mainJWT, testJWTConfig())

	_, err := mainMgr.ValidateAccessToken(at.Token)
	if !errors.Is(err, token.ErrInvalidAccessToken) {
		t.Fatalf("expected ErrInvalidAccessToken for wrong audience, got %v", err)
	}
}
