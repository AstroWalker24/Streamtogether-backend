package routes

import (
	"github.com/gofiber/fiber/v2"

	friendshandler "github.com/AstroWalker24/Streamtogether-backend/internal/friends/handler"
	friendrequesthandler "github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/handler"
	"github.com/AstroWalker24/Streamtogether-backend/internal/health"
	profilehandler "github.com/AstroWalker24/Streamtogether-backend/internal/profile/handler"
)

// Register attaches all application routes to the Fiber router.
func Register(r fiber.Router, healthHandler *health.Handler, profileHandler *profilehandler.Handler, friendsHandler *friendshandler.Handler, friendRequestHandler *friendrequesthandler.Handler, requireAuth fiber.Handler) {
	health.RegisterRoutes(r, healthHandler)
	profilehandler.RegisterRoutes(r, profileHandler, requireAuth)
	friendshandler.RegisterRoutes(r, friendsHandler, requireAuth)
	friendrequesthandler.RegisterRoutes(r, friendRequestHandler, requireAuth)
}
