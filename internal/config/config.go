package config

import (
	"fmt"
	"time"
)



type Environment string 

const (
	EnvDevelopment Environment = "development"
	EnvProduction Environment = "production"
	EnvTest Environment = "test"
)

type Config struct {
	App AppConfig 
	Server ServerConfig 
	Database DatabaseConfig
	Redis RedisConfig 
	JWT JWTConfig 
	Logging LoggingConfig 
	CORS CORSConfig
	RateLimit RateLimitConfig
	Swagger SwaggerConfig 
	Monitoring MonitoringConfig 
	Features FeaturesConfig
}

type AppConfig struct {
	Name string 
	Environment Environment 
	Version string 
	Host string 
	Port int 
}

func (a AppConfig) Address() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

type ServerConfig struct {
	ReadTimeout time.Duration
	WriteTimeout time.Duration
	IdleTimeout time.Duration
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	Host string 
	Port int 
	User string 
	Password string 
	Database string 
	SSLMode string 
	MaxOpenConns int 
	MaxIdleConns int 
	ConnMaxLifetime time.Duration
}

func (db DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s", db.Host, db.Port, db.User, db.Password, db.Database, db.SSLMode)
}

type RedisConfig struct {
	Host string 
	Port int 
	Password string 
	DB int
}

func (r RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

type JWTConfig struct {
	Secret string 
	AccessTokenExpiry time.Duration
	RefreshTokenExpiry time.Duration
}

type LoggingConfig struct {
	Level string 
	Format string
}

type CORSConfig struct {
	AllowedOrigins []string 
	AllowedMethods []string 
	AllowedHeaders []string
}

type RateLimitConfig struct {
	Enabled bool 
	Requests int 
	Duration time.Duration
}

type SwaggerConfig struct {
	Enabled bool
}

type MonitoringConfig struct {
	PrometheusEnabled bool
}

type FeaturesConfig struct {
	Chat bool
	Voice bool
	AI bool
}






