package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/AstroWalker24/Streamtogether-backend/internal/api"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
)

// localsClaimsKey is a private type used as the Fiber Locals key for validated
// JWT claims. A private type prevents accidental key collisions with other
// packages.
type localsClaimsKey struct{}

var claimsKey = localsClaimsKey{}

// NewAuthRequired returns a Fiber middleware that enforces a valid
// "Authorization: Bearer <token>" header on every request.
//
// On success the validated AccessTokenClaims are stored in Fiber Locals and
// the next handler is called. Retrieve them in downstream handlers with
// ClaimsFrom. On failure a 401 JSON response is sent and the chain is halted.
func NewAuthRequired(tm token.TokenManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return api.Unauthorized(c, "authentication required")
		}
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			return api.Unauthorized(c, "authentication required")
		}
		tokenStr := authHeader[len(prefix):]
		claims, err := tm.ValidateAccessToken(tokenStr)
		if err != nil {
			return api.Unauthorized(c, "invalid or expired token")
		}
		c.Locals(claimsKey, claims)
		return c.Next()
	}
}

// ClaimsFrom retrieves the AccessTokenClaims stored by NewAuthRequired.
// ok is false only when the handler is not guarded by NewAuthRequired, which
// is a programming error that must not occur on protected routes.
func ClaimsFrom(c *fiber.Ctx) (token.AccessTokenClaims, bool) {
	claims, ok := c.Locals(claimsKey).(token.AccessTokenClaims)
	return claims, ok
}
