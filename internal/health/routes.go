package health

import "github.com/gofiber/fiber/v2"

func RegisterRoutes(r fiber.Router, h *Handler) {
	r.Get("/", h.Root)
	r.Get("/health", h.Health)
	r.Get("/live", h.Live)
	r.Get("/ready", h.Ready)
	r.Get("/version", h.Version)
}


