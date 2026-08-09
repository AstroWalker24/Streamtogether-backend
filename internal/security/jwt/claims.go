package jwt

import (
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// RegisteredClaims mirrors the standard JWT registered claim set (RFC 7519 §4.1).
// All time fields are represented as time.Time for type safety; zero values
// indicate the claim was absent in the token.
type RegisteredClaims struct {
	// Issuer identifies the principal that issued the token ("iss").
	Issuer string

	// Subject identifies the principal that is the subject of the JWT ("sub").
	Subject string

	// Audience identifies the recipients the JWT is intended for ("aud").
	// Multiple values are supported per the JWT specification.
	Audience []string

	// ExpiresAt is the time after which the token MUST NOT be accepted ("exp").
	ExpiresAt time.Time

	// NotBefore is the time before which the token MUST NOT be accepted ("nbf").
	NotBefore time.Time

	// IssuedAt is the time at which the JWT was issued ("iat").
	IssuedAt time.Time

	// JWTID is a unique identifier for the JWT ("jti").
	JWTID string
}

// Claims is the canonical payload representation used throughout this package.
// It combines the standard registered claims with an optional map for
// controlled custom claims.
//
// Do not embed jwtlib types here; keep the third-party library internal.
type Claims struct {
	RegisteredClaims

	// Custom holds application-defined claims extending the standard set.
	// Values must be JSON-serializable. Nil entries are omitted when signing.
	Custom map[string]any
}

// toLibClaims converts Claims to the library's MapClaims for signing.
// This conversion keeps the third-party type inside this package.
func (c Claims) toLibClaims() jwtlib.MapClaims {
	mc := make(jwtlib.MapClaims, 7+len(c.Custom))

	if c.Issuer != "" {
		mc["iss"] = c.Issuer
	}
	if c.Subject != "" {
		mc["sub"] = c.Subject
	}
	switch len(c.Audience) {
	case 0:
		// omit
	case 1:
		// RFC 7519 §4.1.3: single audience is commonly serialized as a plain string
		mc["aud"] = c.Audience[0]
	default:
		mc["aud"] = c.Audience
	}
	if !c.ExpiresAt.IsZero() {
		mc["exp"] = c.ExpiresAt.Unix()
	}
	if !c.NotBefore.IsZero() {
		mc["nbf"] = c.NotBefore.Unix()
	}
	if !c.IssuedAt.IsZero() {
		mc["iat"] = c.IssuedAt.Unix()
	}
	if c.JWTID != "" {
		mc["jti"] = c.JWTID
	}

	for k, v := range c.Custom {
		if v != nil {
			mc[k] = v
		}
	}

	return mc
}

// fromLibClaims converts the library's MapClaims into our Claims type.
// Unknown keys are collected into Claims.Custom.
func fromLibClaims(mc jwtlib.MapClaims) Claims {
	var c Claims

	if v, ok := mc["iss"].(string); ok {
		c.Issuer = v
	}
	if v, ok := mc["sub"].(string); ok {
		c.Subject = v
	}

	// "aud" may be a bare string or a JSON array of strings.
	switch v := mc["aud"].(type) {
	case string:
		c.Audience = []string{v}
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok {
				c.Audience = append(c.Audience, s)
			}
		}
	}

	// Numeric date claims are decoded as float64 by the standard JSON decoder.
	if v, ok := mc["exp"].(float64); ok {
		c.ExpiresAt = time.Unix(int64(v), 0).UTC()
	}
	if v, ok := mc["nbf"].(float64); ok {
		c.NotBefore = time.Unix(int64(v), 0).UTC()
	}
	if v, ok := mc["iat"].(float64); ok {
		c.IssuedAt = time.Unix(int64(v), 0).UTC()
	}
	if v, ok := mc["jti"].(string); ok {
		c.JWTID = v
	}

	// Collect non-standard claims into Custom.
	standard := map[string]struct{}{
		"iss": {}, "sub": {}, "aud": {},
		"exp": {}, "nbf": {}, "iat": {}, "jti": {},
	}
	for k, v := range mc {
		if _, isStd := standard[k]; !isStd {
			if c.Custom == nil {
				c.Custom = make(map[string]any)
			}
			c.Custom[k] = v
		}
	}

	return c
}
