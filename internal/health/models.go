// Package health exposes application health and observability endpoints.
package health

import "time"

// Status represents the health state of the application or a single dependency.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// RootResponse is the payload for GET /.
type RootResponse struct {
	Application string `json:"application"`
	Environment string `json:"environment"`
	Version     string `json:"version"`
	Status      string `json:"status"`
}

// HealthResponse is the payload for GET /health.
type HealthResponse struct {
	Status       Status            `json:"status"`
	Uptime       string            `json:"uptime"`
	Version      string            `json:"version"`
	Environment  string            `json:"environment"`
	Timestamp    time.Time         `json:"timestamp"`
	Dependencies map[string]Status `json:"dependencies"`
}

// LiveResponse is the payload for GET /live.
type LiveResponse struct {
	Status string `json:"status"`
}

// ReadyResponse is the payload for GET /ready.
type ReadyResponse struct {
	Status       string            `json:"status"`
	Dependencies map[string]Status `json:"dependencies"`
}

// VersionResponse is the payload for GET /version.
type VersionResponse struct {
	Application string `json:"application"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
	GoVersion   string `json:"go_version"`
	BuildTime   string `json:"build_time,omitempty"`
	GitCommit   string `json:"git_commit,omitempty"`
}
