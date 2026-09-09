package handler

import "github.com/gofiber/fiber/v2"

// RegisterRoutes wires authenticated Friend Requests endpoints onto r.
func RegisterRoutes(r fiber.Router, h *Handler, requireAuth fiber.Handler) {
	me := r.Group("/me", requireAuth)
	me.Post("/friend-requests", h.SendFriendRequest)
	me.Get("/friend-requests/incoming", h.GetIncomingRequests)
	me.Get("/friend-requests/outgoing", h.GetOutgoingRequests)
	me.Get("/friend-requests/:requestId", h.GetFriendRequestByID)
	me.Post("/friend-requests/:requestId/accept", h.AcceptFriendRequest)
	me.Post("/friend-requests/:requestId/reject", h.RejectFriendRequest)
	me.Delete("/friend-requests/:requestId", h.CancelFriendRequest)
}
