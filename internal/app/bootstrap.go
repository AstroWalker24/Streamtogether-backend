package app

import (
	"context"
	"fmt"

	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	authsvc "github.com/AstroWalker24/Streamtogether-backend/internal/auth/service"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	friendshandler "github.com/AstroWalker24/Streamtogether-backend/internal/friends/handler"
	friendsrepo "github.com/AstroWalker24/Streamtogether-backend/internal/friends/repository"
	friendsvc "github.com/AstroWalker24/Streamtogether-backend/internal/friends/service"
	"github.com/AstroWalker24/Streamtogether-backend/internal/health"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/AstroWalker24/Streamtogether-backend/internal/middleware"
	profilehandler "github.com/AstroWalker24/Streamtogether-backend/internal/profile/handler"
	profilerepo "github.com/AstroWalker24/Streamtogether-backend/internal/profile/repository"
	profilesvc "github.com/AstroWalker24/Streamtogether-backend/internal/profile/service"
	redisx "github.com/AstroWalker24/Streamtogether-backend/internal/redis"
	"github.com/AstroWalker24/Streamtogether-backend/internal/routes"
	jwtpkg "github.com/AstroWalker24/Streamtogether-backend/internal/security/jwt"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/password"
	"github.com/AstroWalker24/Streamtogether-backend/internal/security/random"
	"github.com/AstroWalker24/Streamtogether-backend/internal/server"
)

func New() (*App, error) {
	// 1. Configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
	}

	// 2. Logger
	log := logger.New(
		logger.WithLevel(cfg.Logging.Level),
		logger.WithPretty(cfg.App.Environment == config.EnvDevelopment),
	)
	log.Info("configuration loaded",
		logger.String("env", string(cfg.App.Environment)),
		logger.String("version", cfg.App.Version),
	)

	ctx := context.Background()

	// 3. PostgreSQL
	db, err := database.New(ctx, cfg.Database, log)
	if err != nil {
		return nil, fmt.Errorf("app: init postgres: %w", err)
	}

	// 4. Redis
	redisInstance, err := redisx.New(ctx, cfg, log)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("app: init redis: %w", err)
	}

	// 5. HTTP Server
	srv, err := server.New(cfg, log)
	if err != nil {
		_ = redisInstance.Close()
		db.Close()
		return nil, fmt.Errorf("app: init server: %w", err)
	}

	mwRegistry := middleware.NewRegistry(middleware.Deps{
		Config: cfg,
		Logger: log,
	})
	mwRegistry.Register(srv.App())

	// 6. Health checkers — required dependencies that must be up for /ready
	checkers := []health.Checker{
		health.NewChecker("postgres", true, db.Health),
		health.NewChecker("redis", true, redisInstance.Health),
	}

	// 7. Health package wiring
	healthSvc := health.NewService(cfg, log, checkers...)
	healthHandler := health.NewHandler(healthSvc)

	// 8. Token manager — required by the auth middleware
	rng := random.New()
	jwtMgr, err := jwtpkg.New(cfg.JWT, rng)
	if err != nil {
		_ = redisInstance.Close()
		db.Close()
		return nil, fmt.Errorf("app: init jwt manager: %w", err)
	}
	tokenMgr, err := token.New(jwtMgr, cfg.JWT)
	if err != nil {
		_ = redisInstance.Close()
		db.Close()
		return nil, fmt.Errorf("app: init token manager: %w", err)
	}
	requireAuth := middleware.NewAuthRequired(tokenMgr)

	// 9. Profile module
	profileRepo := profilerepo.NewProfileRepository(db, log)
	profileSvc := profilesvc.NewProfileService(profileRepo)
	profileHdlr := profilehandler.NewHandler(profileSvc)

	// 10. Friends module
	userRepo := authrepo.NewUserRepository(db, log)
	userSvc := authsvc.NewUserService(userRepo, password.New(cfg.Password))
	friendsRepo := friendsrepo.NewFriendshipRepository(db, log)
	friendsSvc := friendsvc.NewFriendshipService(friendsRepo, userSvc)
	friendsHdlr := friendshandler.NewHandler(friendsSvc)

	// 11. Route registration
	routes.Register(srv.App(), healthHandler, profileHdlr, friendsHdlr, requireAuth)

	return &App{
		cfg:    cfg,
		log:    log,
		db:     db,
		redis:  redisInstance,
		server: srv,
	}, nil
}
