package jwt_test

import (
	"errors"
	"testing"
	"time"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/jwt"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
)

// testConfig returns a minimal valid JWTConfig for tests.
func testConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:             "test-secret-must-be-long-enough-32chars",
		Algorithm:          "HS256",
		Issuer:             "streamtogether",
		Audience:           "streamtogether-clients",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		ClockSkew:          0,
	}
}

// fixedClock returns a Clock fixed at the given time.
func fixedClock(t time.Time) jwt.Clock {
	return func() time.Time { return t }
}

// newTestManager builds a Manager with a fixed clock pinned to now.
func newTestManager(t *testing.T, cfg config.JWTConfig, now time.Time) *jwt.Manager {
	t.Helper()
	rng := random.New()
	m, err := jwt.NewWithClock(cfg, rng, fixedClock(now))
	if err != nil {
		t.Fatalf("NewWithClock: %v", err)
	}
	return m
}

// validClaims builds a Claims value that should pass all validation checks
// given a manager configured with testConfig.
func validClaims(now time.Time) jwt.Claims {
	return jwt.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "streamtogether",
			Subject:   "user-uuid-1234",
			Audience:  []string{"streamtogether-clients"},
			IssuedAt:  now,
			ExpiresAt: now.Add(15 * time.Minute),
			JWTID:     "test-jti",
		},
	}
}

// ── Construction ─────────────────────────────────────────────────────────────

func TestNew_RejectsEmptySecret(t *testing.T) {
	cfg := testConfig()
	cfg.Secret = ""
	_, err := jwt.New(cfg, random.New())
	if err == nil {
		t.Fatal("expected error for empty secret, got nil")
	}
}

func TestNew_RejectsUnsupportedAlgorithm(t *testing.T) {
	cfg := testConfig()
	cfg.Algorithm = "RS256"
	_, err := jwt.New(cfg, random.New())
	if !errors.Is(err, jwt.ErrUnsupportedAlgorithm) {
		t.Fatalf("expected ErrUnsupportedAlgorithm, got %v", err)
	}
}

func TestNew_RejectsNilRNG(t *testing.T) {
	_, err := jwt.NewWithClock(testConfig(), nil, time.Now)
	if err == nil {
		t.Fatal("expected error for nil rng, got nil")
	}
}

// ── Issue & VerifyAndValidate (happy path) ────────────────────────────────────

func TestIssueAndVerify_ValidToken(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t, testConfig(), now)
	claims := validClaims(now)

	token, err := m.Issue(claims)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := m.VerifyAndValidate(token)
	if err != nil {
		t.Fatalf("VerifyAndValidate: %v", err)
	}

	if got.Subject != claims.Subject {
		t.Errorf("Subject: got %q, want %q", got.Subject, claims.Subject)
	}
	if got.Issuer != claims.Issuer {
		t.Errorf("Issuer: got %q, want %q", got.Issuer, claims.Issuer)
	}
	if got.JWTID != claims.JWTID {
		t.Errorf("JWTID: got %q, want %q", got.JWTID, claims.JWTID)
	}
}

// ── Signature verification ────────────────────────────────────────────────────

func TestVerify_InvalidSignature(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t, testConfig(), now)
	claims := validClaims(now)

	token, _ := m.Issue(claims)

	// Tamper with the last character of the signature segment.
	tampered := token[:len(token)-1]
	if token[len(token)-1] == 'A' {
		tampered += "B"
	} else {
		tampered += "A"
	}

	_, err := m.Verify(tampered)
	if !errors.Is(err, jwt.ErrInvalidSignature) && !errors.Is(err, jwt.ErrMalformedToken) {
		t.Fatalf("expected ErrInvalidSignature or ErrMalformedToken, got %v", err)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	now := time.Now().UTC()

	cfgA := testConfig()
	cfgA.Secret = "secret-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mA := newTestManager(t, cfgA, now)

	cfgB := testConfig()
	cfgB.Secret = "secret-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mB := newTestManager(t, cfgB, now)

	token, _ := mA.Issue(validClaims(now))
	_, err := mB.Verify(token)
	if !errors.Is(err, jwt.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerify_MalformedToken(t *testing.T) {
	m := newTestManager(t, testConfig(), time.Now())
	_, err := m.Verify("not.a.jwt")
	if !errors.Is(err, jwt.ErrMalformedToken) && !errors.Is(err, jwt.ErrInvalidSignature) {
		t.Fatalf("expected ErrMalformedToken or ErrInvalidSignature, got %v", err)
	}
}

func TestVerify_EmptyToken(t *testing.T) {
	m := newTestManager(t, testConfig(), time.Now())
	_, err := m.Verify("")
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
}

// ── Unsupported algorithm ─────────────────────────────────────────────────────

func TestVerify_RejectsTokenWithDifferentAlgorithm(t *testing.T) {
	// Build a manager with HS256 but manually craft a token that claims "none".
	// In practice, the library will reject "none" outright, but the important
	// invariant is that the keyFunc enforces the configured algorithm.
	now := time.Now().UTC()
	m := newTestManager(t, testConfig(), now)

	// A real "none" algorithm token (unsigned): header.payload.
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiJ1c2VyLTEiLCJleHAiOjk5OTk5OTk5OTl9."

	_, err := m.Verify(noneToken)
	if !errors.Is(err, jwt.ErrUnsupportedAlgorithm) &&
		!errors.Is(err, jwt.ErrMalformedToken) &&
		!errors.Is(err, jwt.ErrInvalidSignature) {
		t.Fatalf("expected algorithm or signature rejection, got %v", err)
	}
}

// ── Claim validation ──────────────────────────────────────────────────────────

func TestValidateClaims_ExpiredToken(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	m := newTestManager(t, testConfig(), time.Now().UTC()) // clock is NOW

	// Issue a token that was already expired 1 hour ago.
	claims := validClaims(past.Add(-15 * time.Minute))
	claims.ExpiresAt = past // already expired

	token, _ := m.Issue(claims)

	// Verify passes (signature is valid); ValidateClaims should catch expiry.
	parsed, err := m.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := m.ValidateClaims(parsed); !errors.Is(err, jwt.ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestValidateClaims_TokenNotYetValid(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t, testConfig(), now)

	claims := validClaims(now)
	claims.NotBefore = now.Add(10 * time.Minute) // valid only in the future

	token, _ := m.Issue(claims)
	parsed, _ := m.Verify(token)
	if err := m.ValidateClaims(parsed); !errors.Is(err, jwt.ErrTokenNotYetValid) {
		t.Fatalf("expected ErrTokenNotYetValid, got %v", err)
	}
}

func TestValidateClaims_InvalidIssuer(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t, testConfig(), now)

	claims := validClaims(now)
	claims.Issuer = "evil-issuer"

	token, _ := m.Issue(claims)
	parsed, _ := m.Verify(token)
	if err := m.ValidateClaims(parsed); !errors.Is(err, jwt.ErrInvalidIssuer) {
		t.Fatalf("expected ErrInvalidIssuer, got %v", err)
	}
}

func TestValidateClaims_InvalidAudience(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t, testConfig(), now)

	claims := validClaims(now)
	claims.Audience = []string{"wrong-audience"}

	token, _ := m.Issue(claims)
	parsed, _ := m.Verify(token)
	if err := m.ValidateClaims(parsed); !errors.Is(err, jwt.ErrInvalidAudience) {
		t.Fatalf("expected ErrInvalidAudience, got %v", err)
	}
}

func TestValidateClaims_SkipsIssuerWhenNotConfigured(t *testing.T) {
	cfg := testConfig()
	cfg.Issuer = ""   // no issuer configured
	cfg.Audience = "" // no audience configured

	now := time.Now().UTC()
	m := newTestManager(t, cfg, now)

	claims := validClaims(now)
	claims.Issuer = "" // no issuer in token either
	claims.Audience = nil

	token, _ := m.Issue(claims)
	parsed, _ := m.Verify(token)
	if err := m.ValidateClaims(parsed); err != nil {
		t.Fatalf("expected no error when issuer/audience unconfigured, got %v", err)
	}
}

// ── Clock skew ────────────────────────────────────────────────────────────────

func TestValidateClaims_ClockSkewAcceptsSlightlyExpired(t *testing.T) {
	cfg := testConfig()
	cfg.ClockSkew = 30 * time.Second

	now := time.Now().UTC()
	// The manager's clock is 20 seconds ahead of when the token expires.
	// With 30 s of skew tolerance, the token should still be accepted.
	managerNow := now.Add(20 * time.Second)
	m := newTestManager(t, cfg, managerNow)

	claims := validClaims(now)
	claims.ExpiresAt = now // expired 20 s ago from manager's perspective

	token, _ := m.Issue(claims)
	parsed, _ := m.Verify(token)
	if err := m.ValidateClaims(parsed); err != nil {
		t.Fatalf("expected skew to absorb 20s expiry lag, got %v", err)
	}
}

func TestValidateClaims_ClockSkewRejectsWhenExceeded(t *testing.T) {
	cfg := testConfig()
	cfg.ClockSkew = 10 * time.Second

	now := time.Now().UTC()
	// The manager's clock is 30 seconds ahead: exceeds the 10 s skew budget.
	managerNow := now.Add(30 * time.Second)
	m := newTestManager(t, cfg, managerNow)

	claims := validClaims(now)
	claims.ExpiresAt = now

	token, _ := m.Issue(claims)
	parsed, _ := m.Verify(token)
	if err := m.ValidateClaims(parsed); !errors.Is(err, jwt.ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken when skew is exceeded, got %v", err)
	}
}

// ── JTI generation ────────────────────────────────────────────────────────────

func TestGenerateJTI_UniqueAndNonEmpty(t *testing.T) {
	m := newTestManager(t, testConfig(), time.Now())

	seen := make(map[string]struct{}, 100)
	for i := range 100 {
		jti, err := m.GenerateJTI()
		if err != nil {
			t.Fatalf("GenerateJTI iteration %d: %v", i, err)
		}
		if jti == "" {
			t.Fatalf("GenerateJTI returned empty string at iteration %d", i)
		}
		if _, dup := seen[jti]; dup {
			t.Fatalf("GenerateJTI produced duplicate JTI at iteration %d: %q", i, jti)
		}
		seen[jti] = struct{}{}
	}
}

// ── ParseUnverified ───────────────────────────────────────────────────────────

func TestParseUnverified_ExtractsClaimsWithoutVerification(t *testing.T) {
	now := time.Now().UTC()

	// Issue a token with manager A.
	mA := newTestManager(t, testConfig(), now)
	claims := validClaims(now)
	token, _ := mA.Issue(claims)

	// Parse with a manager B that has a DIFFERENT secret (can't verify).
	cfgB := testConfig()
	cfgB.Secret = "completely-different-secret-bbbbb"
	mB := newTestManager(t, cfgB, now)

	// ParseUnverified should still extract the Subject without error.
	parsed, err := mB.ParseUnverified(token)
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	if parsed.Subject != claims.Subject {
		t.Errorf("Subject: got %q, want %q", parsed.Subject, claims.Subject)
	}
}

// ── Custom claims round-trip ──────────────────────────────────────────────────

func TestIssueAndVerify_CustomClaimsRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	m := newTestManager(t, testConfig(), now)

	claims := validClaims(now)
	claims.Custom = map[string]any{
		"session_id": "sess-uuid-5678",
		"roles":      []string{"user"},
	}

	token, err := m.Issue(claims)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := m.VerifyAndValidate(token)
	if err != nil {
		t.Fatalf("VerifyAndValidate: %v", err)
	}

	if got.Custom["session_id"] != "sess-uuid-5678" {
		t.Errorf("custom session_id: got %v", got.Custom["session_id"])
	}
}
