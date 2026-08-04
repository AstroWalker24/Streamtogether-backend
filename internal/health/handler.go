package health

import "github.com/gofiber/fiber/v2"

// Handler holds the service and delegates all business logic to it.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler backed by svc.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Root handles GET /.
func (h *Handler) Root(c *fiber.Ctx) error {
	return c.JSON(h.svc.Root())
}

// Health handles GET /health.
func (h *Handler) Health(c *fiber.Ctx) error {
	return c.JSON(h.svc.Health(c.Context()))
}

// Live handles GET /live. Always returns 200 if the process is running.
func (h *Handler) Live(c *fiber.Ctx) error {
	return c.JSON(h.svc.Live())
}

// Ready handles GET /ready. Returns 503 when any required dependency is unhealthy.
func (h *Handler) Ready(c *fiber.Ctx) error {
	resp, ok := h.svc.Ready(c.Context())
	if !ok {
		return c.Status(fiber.StatusServiceUnavailable).JSON(resp)
	}
	return c.JSON(resp)
}

// Version handles GET /version.
func (h *Handler) Version(c *fiber.Ctx) error {
	return c.JSON(h.svc.Version())
}
