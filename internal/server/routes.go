package server

import "github.com/gofiber/fiber/v2"

// RegisterRoutes calls fn with the root Fiber router.
// Modules use this to attach their routes without coupling to the server internals.
func (s *Server) RegisterRoutes(fn func(router fiber.Router)) {
    fn(s.app)
}