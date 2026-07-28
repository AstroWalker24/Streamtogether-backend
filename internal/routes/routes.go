package routes

import (
	"net/http"

	"github.com/AstroWalker24/Streamtogether-backend/internal/handlers"
)



func Register(health *handlers.HealthHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/ready", health.Ready)
	mux.HandleFunc("GET /health/live", health.Live)
	return mux
}





