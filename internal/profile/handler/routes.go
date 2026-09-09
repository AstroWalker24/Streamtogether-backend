package handler

import "github.com/gofiber/fiber/v2"

// RegisterRoutes wires the profile endpoints onto r.
//
//	GET   /me/profile              — retrieve the authenticated user's own profile
//	PATCH /me/profile              — update the authenticated user's own profile
//	GET   /users/:userId/profile   — retrieve a profile by user ID (authenticated)
func RegisterRoutes(r fiber.Router, h *Handler, requireAuth fiber.Handler) {
	me := r.Group("/me", requireAuth)
	me.Get("/profile", h.GetMyProfile)
	me.Patch("/profile", h.UpdateMyProfile)

	r.Get("/users/:userId/profile", requireAuth, h.GetUserProfile)
}
