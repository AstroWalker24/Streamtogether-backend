package routes

import (
	"github.com/AstroWalker24/Streamtogether-backend/internal/health"
	"github.com/gofiber/fiber/v2"
)

// Register attaches all application routes to the Fiber router.
func Register(r fiber.Router, healthHandler *health.Handler) {
	health.RegisterRoutes(r, healthHandler)
}
