package health

import (
	"context"
	"runtime"
	"time"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// Build-time variables; override via -ldflags at build time.
var (
	BuildTime = ""
	GitCommit = ""
)

// Service runs dependency checks and assembles health endpoint responses.
type Service struct {
	cfg       *config.Config
	log       logger.Logger
	checkers  []Checker
	startedAt time.Time
}

// NewService constructs a Service with the provided checkers.
func NewService(cfg *config.Config, log logger.Logger, checkers ...Checker) *Service {
	return &Service{
		cfg:       cfg,
		log:       log,
		checkers:  checkers,
		startedAt: time.Now(),
	}
}

// Root returns the basic application info response.
func (s *Service) Root() RootResponse {
	return RootResponse{
		Application: s.cfg.App.Name,
		Environment: string(s.cfg.App.Environment),
		Version:     s.cfg.App.Version,
		Status:      "running",
	}
}

// Health runs all checks and returns the aggregated health response.
func (s *Service) Health(ctx context.Context) HealthResponse {
	deps := s.runChecks(ctx)
	return HealthResponse{
		Status:       aggregateStatus(deps, s.checkers),
		Uptime:       time.Since(s.startedAt).Round(time.Second).String(),
		Version:      s.cfg.App.Version,
		Environment:  string(s.cfg.App.Environment),
		Timestamp:    time.Now().UTC(),
		Dependencies: deps,
	}
}

// Live signals that the process is alive.
func (s *Service) Live() LiveResponse {
	return LiveResponse{Status: "ok"}
}

// Ready runs all checks and reports whether the app can serve requests.
// The second return value is false when any required dependency is unhealthy.
func (s *Service) Ready(ctx context.Context) (ReadyResponse, bool) {
	deps := s.runChecks(ctx)
	status := aggregateStatus(deps, s.checkers)
	ready := status != StatusUnhealthy

	if !ready {
		s.log.Warn("readiness check failed", logger.Any("dependencies", deps))
	}

	return ReadyResponse{
		Status:       string(status),
		Dependencies: deps,
	}, ready
}

// Version returns build and runtime version information.
func (s *Service) Version() VersionResponse {
	return VersionResponse{
		Application: s.cfg.App.Name,
		Version:     s.cfg.App.Version,
		Environment: string(s.cfg.App.Environment),
		GoVersion:   runtime.Version(),
		BuildTime:   BuildTime,
		GitCommit:   GitCommit,
	}
}

// runChecks executes every registered checker and returns a per-name status map.
func (s *Service) runChecks(ctx context.Context) map[string]Status {
	deps := make(map[string]Status, len(s.checkers))
	for _, c := range s.checkers {
		if err := c.Check(ctx); err != nil {
			s.log.Error("dependency check failed",
				logger.String("dependency", c.Name()),
				logger.Err(err),
			)
			deps[c.Name()] = StatusUnhealthy
		} else {
			deps[c.Name()] = StatusHealthy
		}
	}
	return deps
}

// aggregateStatus derives the overall status from individual dependency results.
// Any required unhealthy dependency → Unhealthy.
// Any optional unhealthy dependency → Degraded.
// All healthy → Healthy.
func aggregateStatus(deps map[string]Status, checkers []Checker) Status {
	overall := StatusHealthy
	for _, c := range checkers {
		if deps[c.Name()] == StatusUnhealthy {
			if c.Required() {
				return StatusUnhealthy
			}
			overall = StatusDegraded
		}
	}
	return overall
}
