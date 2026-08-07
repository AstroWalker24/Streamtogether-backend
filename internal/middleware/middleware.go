// Package middleware provides reusable HTTP middleware for the Fiber HTTP server.
package middleware

import (
	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// Deps carries the shared dependencies injected into middleware constructors.
type Deps struct {
	Config *config.Config
	Logger logger.Logger
}
