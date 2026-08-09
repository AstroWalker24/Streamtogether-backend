package jwt

import (
	"fmt"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Parser decodes a JWT token string without verifying the signature.
// It is safe for concurrent use.
//
// Warning: claims returned by ParseUnverified are NOT authenticated.
// They MUST NOT be used to make access-control decisions without subsequent
// signature verification via Verifier.Verify.
type Parser struct{}

// newParser returns a ready-to-use Parser.
func newParser() *Parser {
	return &Parser{}
}

// ParseUnverified decodes the token and extracts its claims without verifying
// the signature or validating any registered claims.
//
// Its primary intended use is to read the "jti", "iss", or "sub" before
// performing a key-material lookup in order to drive a subsequent verified
// parse. Do NOT use this output for authorization.
func (p *Parser) ParseUnverified(tokenString string) (Claims, error) {
	parser := jwtlib.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwtlib.MapClaims{})
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrMalformedToken, err)
	}

	mc, ok := token.Claims.(jwtlib.MapClaims)
	if !ok {
		return Claims{}, ErrMalformedToken
	}

	return fromLibClaims(mc), nil
}
