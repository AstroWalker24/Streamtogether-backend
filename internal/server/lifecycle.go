package server

import (
    "context"
    "fmt"

    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// Start begins listening for HTTP connections. It blocks until the server
// stops or returns a listen error. Call Shutdown to stop it gracefully.
func (s *Server) Start(_ context.Context) error {
    addr := s.cfg.App.Address()

    s.log.Info("server starting", logger.String("address", addr))

    if err := s.app.Listen(addr); err != nil {
        return fmt.Errorf("server listen: %w", err)
    }

    return nil
}

// Shutdown gracefully stops the server, waiting for in-flight requests to complete.
func (s *Server) Shutdown(ctx context.Context) error {
    s.log.Info("server shutdown initiated")

    if err := s.app.ShutdownWithContext(ctx); err != nil {
        return fmt.Errorf("server shutdown: %w", err)
    }

    s.log.Info("server shutdown complete")
    return nil
}