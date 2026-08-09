package token_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	jwtpkg "github.com/AstroWalker24/Streamtogether-backend/internal/security/jwt"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func testJWTConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:             "test-secret-must-be-at-least-32-chars",
		Algorithm:          "HS256",
		Issuer:             "streamtogether-test",
		Audience:           "st-clients",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
}

func newTestManager(t *testing.T) *jwtpkg.Manager {
	t.Helper()
	m, err := jwtpkg.New(testJWTConfig(), random.New())
	if err != nil {
		t.Fatalf("jwtpkg.New: %v", err)
	}
	return m
}

// validInput returns a BuildInput with all required fields populated.
func validInput(now time.Time) token.BuildInput {
	return token.BuildInput{
		UserID:    uuid.New(),
		SessionID: uuid.New(),
		Roles:     []string{"user"},
		TokenID:   uuid.New(),
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
		Issuer:    "streamtogether-test",
		Audience:  []string{"st-clients"},
	}
}

// roundTrip signs and verifies a token using the JWT Foundation, then returns
// the extracted AccessTokenClaims.
func roundTrip(t *testing.T, m *jwtpkg.Manager, in token.BuildInput) token.AccessTokenClaims {
	t.Helper()

	jwtClaims, err := token.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	tokenStr, err := m.Issue(jwtClaims)
	if err != nil {
		t.Fatalf("Manager.Issue: %v", err)
	}
	verified, err := m.VerifyAndValidate(tokenStr)
	if err != nil {
		t.Fatalf("Manager.VerifyAndValidate: %v", err)
	}
	out, err := token.FromJWTClaims(verified)
	if err != nil {
		t.Fatalf("FromJWTClaims: %v", err)
	}
	return out
}

// directExtract calls FromJWTClaims without going through JWT serialisation.
// Useful for testing structural extraction logic in isolation.
func directExtract(t *testing.T, in token.BuildInput) token.AccessTokenClaims {
	t.Helper()
	jwtClaims, err := token.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out, err := token.FromJWTClaims(jwtClaims)
	if err != nil {
		t.Fatalf("FromJWTClaims: %v", err)
	}
	return out
}

// ── Build: valid construction ─────────────────────────────────────────────────

func TestBuild_ValidInput(t *testing.T) {
	now := time.Now().UTC()
	in := validInput(now)

	claims, err := token.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if claims.Subject != in.UserID.String() {
		t.Errorf("Subject: got %q, want %q", claims.Subject, in.UserID.String())
	}
	if claims.JWTID != in.TokenID.String() {
		t.Errorf("JWTID: got %q, want %q", claims.JWTID, in.TokenID.String())
	}
	if claims.Issuer != in.Issuer {
		t.Errorf("Issuer: got %q, want %q", claims.Issuer, in.Issuer)
	}
}

func TestBuild_SubjectIsUserID(t *testing.T) {
	now := time.Now().UTC()
	in := validInput(now)

	claims, err := token.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	parsed, err := uuid.Parse(claims.Subject)
	if err != nil {
		t.Fatalf("sub is not a valid UUID: %v", err)
	}
	if parsed != in.UserID {
		t.Errorf("sub round-trip: got %v, want %v", parsed, in.UserID)
	}
}

func TestBuild_NilRolesProducesEmptyArray(t *testing.T) {
	now := time.Now().UTC()
	in := validInput(now)
	in.Roles = nil

	claims, err := token.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if claims.Custom["roles"] == nil {
		t.Error("nil Roles input must produce a non-nil empty array in Custom, not nil")
	}
}

func TestBuild_CorrectCustomClaimKeys(t *testing.T) {
	now := time.Now().UTC()
	in := validInput(now)

	claims, err := token.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := claims.Custom["sid"]; !ok {
		t.Error("expected Custom[\"sid\"] to be present")
	}
	if _, ok := claims.Custom["roles"]; !ok {
		t.Error("expected Custom[\"roles\"] to be present")
	}
}

// ── Build: validation errors ──────────────────────────────────────────────────

func TestBuild_ZeroUserID(t *testing.T) {
	in := validInput(time.Now().UTC())
	in.UserID = uuid.Nil
	_, err := token.Build(in)
	if !errors.Is(err, token.ErrInvalidAuthClaims) {
		t.Fatalf("expected ErrInvalidAuthClaims, got %v", err)
	}
}

func TestBuild_ZeroSessionID(t *testing.T) {
	in := validInput(time.Now().UTC())
	in.SessionID = uuid.Nil
	_, err := token.Build(in)
	if !errors.Is(err, token.ErrInvalidAuthClaims) {
		t.Fatalf("expected ErrInvalidAuthClaims, got %v", err)
	}
}

func TestBuild_ZeroTokenID(t *testing.T) {
	in := validInput(time.Now().UTC())
	in.TokenID = uuid.Nil
	_, err := token.Build(in)
	if !errors.Is(err, token.ErrInvalidAuthClaims) {
		t.Fatalf("expected ErrInvalidAuthClaims, got %v", err)
	}
}

func TestBuild_ZeroIssuedAt(t *testing.T) {
	in := validInput(time.Now().UTC())
	in.IssuedAt = time.Time{}
	_, err := token.Build(in)
	if !errors.Is(err, token.ErrInvalidAuthClaims) {
		t.Fatalf("expected ErrInvalidAuthClaims, got %v", err)
	}
}

func TestBuild_ZeroExpiresAt(t *testing.T) {
	in := validInput(time.Now().UTC())
	in.ExpiresAt = time.Time{}
	_, err := token.Build(in)
	if !errors.Is(err, token.ErrInvalidAuthClaims) {
		t.Fatalf("expected ErrInvalidAuthClaims, got %v", err)
	}
}

func TestBuild_ExpiresAtEqualToIssuedAt(t *testing.T) {
	now := time.Now().UTC()
	in := validInput(now)
	in.ExpiresAt = now // equal is not strictly after
	_, err := token.Build(in)
	if !errors.Is(err, token.ErrInvalidAuthClaims) {
		t.Fatalf("expected ErrInvalidAuthClaims when ExpiresAt == IssuedAt, got %v", err)
	}
}

func TestBuild_ExpiresAtBeforeIssuedAt(t *testing.T) {
	now := time.Now().UTC()
	in := validInput(now)
	in.ExpiresAt = now.Add(-time.Minute)
	_, err := token.Build(in)
	if !errors.Is(err, token.ErrInvalidAuthClaims) {
		t.Fatalf("expected ErrInvalidAuthClaims when ExpiresAt < IssuedAt, got %v", err)
	}
}

// ── FromJWTClaims: direct extraction (no JWT round-trip) ─────────────────────

func TestFromJWTClaims_ValidClaims(t *testing.T) {
	now := time.Now().UTC()
	in := validInput(now)
	got := directExtract(t, in)

	if got.UserID != in.UserID {
		t.Errorf("UserID: got %v, want %v", got.UserID, in.UserID)
	}
	if got.SessionID != in.SessionID {
		t.Errorf("SessionID: got %v, want %v", got.SessionID, in.SessionID)
	}
	if got.TokenID != in.TokenID {
		t.Errorf("TokenID: got %v, want %v", got.TokenID, in.TokenID)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "user" {
		t.Errorf("Roles: got %v, want [user]", got.Roles)
	}
}

func TestFromJWTClaims_MissingSubject(t *testing.T) {
	_, err := token.FromJWTClaims(jwtpkg.Claims{})
	if !errors.Is(err, token.ErrMissingSubject) {
		t.Fatalf("expected ErrMissingSubject, got %v", err)
	}
}

func TestFromJWTClaims_InvalidSubjectUUID(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: "not-a-uuid",
			JWTID:   uuid.New().String(),
		},
		Custom: map[string]any{
			"sid":   uuid.New().String(),
			"roles": []string{},
		},
	}
	_, err := token.FromJWTClaims(claims)
	if !errors.Is(err, token.ErrInvalidUserID) {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
}

func TestFromJWTClaims_MissingTokenID(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: uuid.New().String(),
			// JWTID intentionally absent
		},
		Custom: map[string]any{
			"sid":   uuid.New().String(),
			"roles": []string{},
		},
	}
	_, err := token.FromJWTClaims(claims)
	if !errors.Is(err, token.ErrMissingTokenID) {
		t.Fatalf("expected ErrMissingTokenID, got %v", err)
	}
}

func TestFromJWTClaims_InvalidTokenIDUUID(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: uuid.New().String(),
			JWTID:   "not-a-uuid",
		},
		Custom: map[string]any{
			"sid":   uuid.New().String(),
			"roles": []string{},
		},
	}
	_, err := token.FromJWTClaims(claims)
	if !errors.Is(err, token.ErrInvalidTokenID) {
		t.Fatalf("expected ErrInvalidTokenID, got %v", err)
	}
}

func TestFromJWTClaims_MissingSessionID(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: uuid.New().String(),
			JWTID:   uuid.New().String(),
		},
		Custom: map[string]any{
			"roles": []string{},
			// "sid" intentionally absent
		},
	}
	_, err := token.FromJWTClaims(claims)
	if !errors.Is(err, token.ErrMissingSessionID) {
		t.Fatalf("expected ErrMissingSessionID, got %v", err)
	}
}

func TestFromJWTClaims_InvalidSessionIDType(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: uuid.New().String(),
			JWTID:   uuid.New().String(),
		},
		Custom: map[string]any{
			"sid":   42, // wrong type
			"roles": []string{},
		},
	}
	_, err := token.FromJWTClaims(claims)
	if !errors.Is(err, token.ErrInvalidClaimType) {
		t.Fatalf("expected ErrInvalidClaimType, got %v", err)
	}
}

func TestFromJWTClaims_InvalidSessionIDUUID(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: uuid.New().String(),
			JWTID:   uuid.New().String(),
		},
		Custom: map[string]any{
			"sid":   "not-a-uuid",
			"roles": []string{},
		},
	}
	_, err := token.FromJWTClaims(claims)
	if !errors.Is(err, token.ErrInvalidSessionID) {
		t.Fatalf("expected ErrInvalidSessionID, got %v", err)
	}
}

func TestFromJWTClaims_InvalidRolesType(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: uuid.New().String(),
			JWTID:   uuid.New().String(),
		},
		Custom: map[string]any{
			"sid":   uuid.New().String(),
			"roles": "user", // bare string, not an array
		},
	}
	_, err := token.FromJWTClaims(claims)
	if !errors.Is(err, token.ErrInvalidClaimType) {
		t.Fatalf("expected ErrInvalidClaimType, got %v", err)
	}
}

func TestFromJWTClaims_MissingRolesIsAccepted(t *testing.T) {
	claims := jwtpkg.Claims{
		RegisteredClaims: jwtpkg.RegisteredClaims{
			Subject: uuid.New().String(),
			JWTID:   uuid.New().String(),
		},
		Custom: map[string]any{
			"sid": uuid.New().String(),
			// "roles" intentionally absent — treated as empty
		},
	}
	got, err := token.FromJWTClaims(claims)
	if err != nil {
		t.Fatalf("expected no error for absent roles, got %v", err)
	}
	if got.Roles != nil {
		t.Errorf("expected nil Roles for absent key, got %v", got.Roles)
	}
}

// ── Round-trip: Build → Sign → Verify → Extract ───────────────────────────────

func TestRoundTrip_FullCycle(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t)
	in := validInput(now)
	got := roundTrip(t, m, in)

	if got.UserID != in.UserID {
		t.Errorf("UserID: got %v, want %v", got.UserID, in.UserID)
	}
	if got.SessionID != in.SessionID {
		t.Errorf("SessionID: got %v, want %v", got.SessionID, in.SessionID)
	}
	if got.TokenID != in.TokenID {
		t.Errorf("TokenID: got %v, want %v", got.TokenID, in.TokenID)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "user" {
		t.Errorf("Roles: got %v, want [user]", got.Roles)
	}
}

func TestRoundTrip_SubjectIsUserID(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t)
	in := validInput(now)
	got := roundTrip(t, m, in)

	if got.UserID != in.UserID {
		t.Errorf("sub round-trip failed: got %v, want %v", got.UserID, in.UserID)
	}
}

func TestRoundTrip_SessionIDPreserved(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t)
	in := validInput(now)
	got := roundTrip(t, m, in)

	if got.SessionID != in.SessionID {
		t.Errorf("SessionID round-trip: got %v, want %v", got.SessionID, in.SessionID)
	}
}

func TestRoundTrip_JTIPreserved(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t)
	in := validInput(now)
	got := roundTrip(t, m, in)

	if got.TokenID == uuid.Nil {
		t.Error("TokenID must not be nil UUID after round-trip")
	}
	if got.TokenID != in.TokenID {
		t.Errorf("TokenID round-trip: got %v, want %v", got.TokenID, in.TokenID)
	}
}

func TestRoundTrip_ExpiryPreserved(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(15 * time.Minute)
	m := newTestManager(t)
	in := validInput(now)
	in.ExpiresAt = expiry

	got := roundTrip(t, m, in)

	// Unix timestamp truncation may cause ±1s difference.
	diff := got.ExpiresAt.Sub(expiry)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("ExpiresAt drift too large: got %v, want ~%v (diff %v)", got.ExpiresAt, expiry, diff)
	}
}

func TestRoundTrip_MultipleRoles(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t)
	in := validInput(now)
	in.Roles = []string{"user", "moderator"}

	got := roundTrip(t, m, in)

	if len(got.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d: %v", len(got.Roles), got.Roles)
	}
	roleSet := map[string]bool{got.Roles[0]: true, got.Roles[1]: true}
	for _, want := range in.Roles {
		if !roleSet[want] {
			t.Errorf("role %q not found after round-trip", want)
		}
	}
}

func TestRoundTrip_MinimalPayload(t *testing.T) {
	// Confirm that no claims beyond the defined set appear in the signed token.
	now := time.Now().UTC()
	m := newTestManager(t)
	in := validInput(now)

	jwtClaims, err := token.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	tokenStr, err := m.Issue(jwtClaims)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	verified, err := m.VerifyAndValidate(tokenStr)
	if err != nil {
		t.Fatalf("VerifyAndValidate: %v", err)
	}

	for k := range verified.Custom {
		if k != "sid" && k != "roles" {
			t.Errorf("unexpected custom claim %q in token payload", k)
		}
	}
}

func TestRoundTrip_IssuedAtPreserved(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second) // truncate to avoid sub-second drift
	m := newTestManager(t)
	in := validInput(now)
	in.IssuedAt = now

	got := roundTrip(t, m, in)

	if got.IssuedAt.IsZero() {
		t.Error("IssuedAt must not be zero after round-trip")
	}
	diff := got.IssuedAt.Sub(now)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("IssuedAt drift too large: got %v, want ~%v (diff %v)", got.IssuedAt, now, diff)
	}
}
