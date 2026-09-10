package handler

import "github.com/gofiber/fiber/v2"

// RegisterRoutes wires authenticated Search endpoints onto r.
//
//	GET /me/search/users — search for discoverable users by username or display name
func RegisterRoutes(r fiber.Router, h *Handler, requireAuth fiber.Handler) {
	me := r.Group("/me", requireAuth)
	me.Get("/search/users", h.Search)
}
