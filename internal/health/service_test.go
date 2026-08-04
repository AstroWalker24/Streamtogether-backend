package health_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/health"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

func testConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			Name:        "Test App",
			Environment: config.EnvTest,
			Version:     "0.0.1",
		},
	}
}

func TestService_Root(t *testing.T) {
	svc := health.NewService(testConfig(), logger.Nop())
	resp := svc.Root()

	if resp.Application != "Test App" {
		t.Errorf("expected application %q, got %q", "Test App", resp.Application)
	}
	if resp.Status != "running" {
		t.Errorf("expected status %q, got %q", "running", resp.Status)
	}
}

func TestService_Live(t *testing.T) {
	svc := health.NewService(testConfig(), logger.Nop())
	resp := svc.Live()

	if resp.Status != "ok" {
		t.Errorf("expected %q, got %q", "ok", resp.Status)
	}
}

func TestService_Health_AllHealthy(t *testing.T) {
	passing := health.NewChecker("db", true, func(_ context.Context) error { return nil })
	svc := health.NewService(testConfig(), logger.Nop(), passing)

	resp := svc.Health(context.Background())

	if resp.Status != health.StatusHealthy {
		t.Errorf("expected %q, got %q", health.StatusHealthy, resp.Status)
	}
	if resp.Dependencies["db"] != health.StatusHealthy {
		t.Errorf("expected db healthy")
	}
}

func TestService_Health_RequiredDown_Unhealthy(t *testing.T) {
	failing := health.NewChecker("db", true, func(_ context.Context) error {
		return errors.New("connection refused")
	})
	svc := health.NewService(testConfig(), logger.Nop(), failing)

	resp := svc.Health(context.Background())

	if resp.Status != health.StatusUnhealthy {
		t.Errorf("expected %q, got %q", health.StatusUnhealthy, resp.Status)
	}
}

func TestService_Health_OptionalDown_Degraded(t *testing.T) {
	optional := health.NewChecker("cache", false, func(_ context.Context) error {
		return errors.New("timeout")
	})
	svc := health.NewService(testConfig(), logger.Nop(), optional)

	resp := svc.Health(context.Background())

	if resp.Status != health.StatusDegraded {
		t.Errorf("expected %q, got %q", health.StatusDegraded, resp.Status)
	}
}

func TestService_Ready_AllHealthy(t *testing.T) {
	passing := health.NewChecker("db", true, func(_ context.Context) error { return nil })
	svc := health.NewService(testConfig(), logger.Nop(), passing)

	_, ok := svc.Ready(context.Background())
	if !ok {
		t.Error("expected ready=true when all checkers pass")
	}
}

func TestService_Ready_RequiredDown_NotReady(t *testing.T) {
	failing := health.NewChecker("db", true, func(_ context.Context) error {
		return errors.New("down")
	})
	svc := health.NewService(testConfig(), logger.Nop(), failing)

	_, ok := svc.Ready(context.Background())
	if ok {
		t.Error("expected ready=false when required checker fails")
	}
}

func TestService_Version(t *testing.T) {
	svc := health.NewService(testConfig(), logger.Nop())
	resp := svc.Version()

	if resp.GoVersion == "" {
		t.Error("expected non-empty go version")
	}
	if resp.Version != "0.0.1" {
		t.Errorf("expected version %q, got %q", "0.0.1", resp.Version)
	}
}
