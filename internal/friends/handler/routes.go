package handler

import "github.com/gofiber/fiber/v2"

// RegisterRoutes wires authenticated, actor-owned Friends endpoints onto r.
//
//	POST   /me/friends          — establish a friendship with a target user
//	GET    /me/friends          — list the authenticated user's friendships
//	GET    /me/friends/:userId  — check friendship with a target user
//	DELETE /me/friends/:userId  — remove friendship with a target user
func RegisterRoutes(r fiber.Router, h *Handler, requireAuth fiber.Handler) {
	me := r.Group("/me", requireAuth)
	me.Post("/friends", h.EstablishFriendship)
	me.Get("/friends", h.GetFriends)
	me.Get("/friends/:userId", h.AreFriends)
	me.Delete("/friends/:userId", h.RemoveFriendship)
}
