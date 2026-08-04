package app

import (
	redisx "github.com/AstroWalker24/Streamtogether-backend/internal/redis"

	"github.com/AstroWalker24/Streamtogether-backend/internal/config"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/AstroWalker24/Streamtogether-backend/internal/server"
)

type App struct {
	cfg    *config.Config
	log    logger.Logger
	db     *database.Database
	redis  *redisx.Redis
	server *server.Server
}
