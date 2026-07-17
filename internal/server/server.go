package server

import (
	"net/http"
	"context"
	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
)


type Server struct {
	http *http.Server
}

func New(cfg config.ServerConfig, appCfg config.AppConfig, handler http.Handler) *Server {
	return &Server{
		http: &http.Server{
			Addr: appCfg.Address(),
			Handler: handler,
			ReadTimeout: cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout: cfg.IdleTimeout,
		},
	}
}

func (s *Server) Start() error {
	err := s.http.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
} 


