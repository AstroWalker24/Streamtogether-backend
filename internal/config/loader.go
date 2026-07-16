package config

import (
	"fmt"
	"os"
	"strings"
	"time"
	"github.com/spf13/viper"
)

func Load() (*Config, error) {
	v := viper.New() 

	setDefaults(v) 
	env:= resolveEnvironment()

	err := loadEnvFile(v, env)
	if err != nil {
		return nil, fmt.Errorf("config: loading env file: %w", err)
	}
	v.AutomaticEnv()

	cfg, err := populate(v, env)
	if err != nil {
		return nil, fmt.Errorf("config: populating struct: %w", err)
	}
	validationError := Validate(cfg)
	if validationError != nil {
		return nil, fmt.Errorf("config: validation failed: %w", validationError)
	}
	return cfg, nil 
}

func resolveEnvironment() Environment {
	switch Environment(strings.ToLower(os.Getenv("APP_ENV"))) {
	case EnvTest:
		return EnvTest 
	case EnvProduction:
		return EnvProduction
	default:
		return EnvDevelopment 
	}
}

func loadEnvFile(v *viper.Viper, env Environment) error {
	v.SetConfigType("env")
	v.SetConfigFile(fmt.Sprintf("configs/.env.%s", env))

	err := v.ReadInConfig()
	if err != nil {
		_, ok := err.(viper.ConfigFileNotFoundError)
		if ok {
			return nil
		}
		if (os.IsNotExist(err)) {
			return nil
		}
		return err
	}
	return nil
}


func setDefaults(v *viper.Viper) {
	v.SetDefault("APP_NAME", "Streamtogether Backend")
	v.SetDefault("APP_ENV", string(EnvDevelopment))
	v.SetDefault("APP_VERSION", "0.0.1")
	v.SetDefault("APP_HOST", "0.0.0.0")
	v.SetDefault("APP_PORT", 8080)

	v.SetDefault("READ_TIMEOUT", "15s")
	v.SetDefault("WRITE_TIMEOUT", "15s")
	v.SetDefault("IDLE_TIMEOUT", "60s")
	v.SetDefault("SHUTDOWN_TIMEOUT", "30s")

	v.SetDefault("POSTGRES_HOST", "localhost")
	v.SetDefault("POSTGRES_PORT", 5432)
	v.SetDefault("POSTGRES_SSLMODE", "disable")
	v.SetDefault("POSTGRES_MAX_OPEN_CONNS", 25)
	v.SetDefault("POSTGRES_MAX_IDLE_CONNS", 10)
	v.SetDefault("POSTGRES_CONN_MAX_LIFETIME", "30m")

	v.SetDefault("REDIS_HOST", "localhost")
	v.SetDefault("REDIS_PORT", 6379)
	v.SetDefault("REDIS_DB", 0)

	v.SetDefault("JWT_ACCESS_TOKEN_EXPIRY", "15m")
	v.SetDefault("JWT_REFRESH_TOKEN_EXPIRY", "168h")

	v.SetDefault("LOG_LEVEL", "debug")
	v.SetDefault("LOG_FORMAT", "json")

	v.SetDefault("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
	v.SetDefault("CORS_ALLOWED_HEADERS", "*")

	v.SetDefault("RATE_LIMIT_ENABLED", true)
	v.SetDefault("RATE_LIMIT_REQUESTS", 100)
	v.SetDefault("RATE_LIMIT_DURATION", "1m")

	v.SetDefault("SWAGGER_ENABLED", true)
	v.SetDefault("PROMETHEUS_ENABLED", true)

	v.SetDefault("FEATURE_CHAT", true)
	v.SetDefault("FEATURE_VOICE",false)
	v.SetDefault("FEATURE_AI", false)


}

func populate(v *viper.Viper, env Environment) (*Config, error) {
	readTimeout, err := parseDuration(v, "READ_TIMEOUT")
	if err != nil {
		return nil, err
	}
	writeTimeout, err := parseDuration(v, "WRITE_TIMEOUT")
	if err != nil {
		return nil, err
	}
	idleTimeout, err := parseDuration(v, "IDLE_TIMEOUT")
	if err != nil {
		return nil, err
	}
	shutdownTimeout, err := parseDuration(v, "SHUTDOWN_TIMEOUT")
	if err != nil {
		return nil, err
	}
	connMaxLifetime, err := parseDuration(v, "POSTGRES_CONN_MAX_LIFETIME")
	if err != nil {
		return nil, err
	}
	accessExpiry, err := parseDuration(v, "JWT_ACCESS_TOKEN_EXPIRY")
	if err != nil {
		return nil, err
	}
	refreshExpiry, err := parseDuration(v, "JWT_REFRESH_TOKEN_EXPIRY")
	if err != nil {
		return nil, err
	}
	rateLimitDuration, err := parseDuration(v, "RATE_LIMIT_DURATION")
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		App: AppConfig{
			Name: v.GetString("APP_NAME"),
			Environment: env,
			Version: v.GetString("APP_VERSION"),
			Host: v.GetString("APP_HOST"),
			Port: v.GetInt("APP_PORT"),
		},
		Server: ServerConfig{
			ReadTimeout:     readTimeout,
            WriteTimeout:    writeTimeout,
            IdleTimeout:     idleTimeout,
            ShutdownTimeout: shutdownTimeout,
		},
		Database: DatabaseConfig{
            Host:            v.GetString("POSTGRES_HOST"),
            Port:            v.GetInt("POSTGRES_PORT"),
            User:            v.GetString("POSTGRES_USER"),
            Password:        v.GetString("POSTGRES_PASSWORD"),
            Database:        v.GetString("POSTGRES_DB"),
            SSLMode:         v.GetString("POSTGRES_SSLMODE"),
            MaxOpenConns:    v.GetInt("POSTGRES_MAX_OPEN_CONNS"),
            MaxIdleConns:    v.GetInt("POSTGRES_MAX_IDLE_CONNS"),
            ConnMaxLifetime: connMaxLifetime,
        },
		Redis: RedisConfig{
            Host:     v.GetString("REDIS_HOST"),
            Port:     v.GetInt("REDIS_PORT"),
            Password: v.GetString("REDIS_PASSWORD"),
            DB:       v.GetInt("REDIS_DB"),
        },
		JWT: JWTConfig{
            Secret:             v.GetString("JWT_SECRET"),
            AccessTokenExpiry:  accessExpiry,
            RefreshTokenExpiry: refreshExpiry,
        },
		Logging: LoggingConfig{
            Level:  v.GetString("LOG_LEVEL"),
            Format: v.GetString("LOG_FORMAT"),
        },
        CORS: CORSConfig{
            AllowedOrigins: splitCSV(v.GetString("CORS_ALLOWED_ORIGINS")),
            AllowedMethods: splitCSV(v.GetString("CORS_ALLOWED_METHODS")),
            AllowedHeaders: splitCSV(v.GetString("CORS_ALLOWED_HEADERS")),
        },
		RateLimit: RateLimitConfig{
            Enabled:  v.GetBool("RATE_LIMIT_ENABLED"),
            Requests: v.GetInt("RATE_LIMIT_REQUESTS"),
            Duration: rateLimitDuration,
        },
        Swagger: SwaggerConfig{
            Enabled: v.GetBool("SWAGGER_ENABLED"),
        },
        Monitoring: MonitoringConfig{
            PrometheusEnabled: v.GetBool("PROMETHEUS_ENABLED"),
        },
        Features: FeaturesConfig{
            Chat:  v.GetBool("FEATURE_CHAT"),
            Voice: v.GetBool("FEATURE_VOICE"),
            AI:    v.GetBool("FEATURE_AI"),
        },
	}
	return cfg, nil
}

func parseDuration(v *viper.Viper, key string) (time.Duration, error) {
	raw := v.GetString(key)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s=%q: %w", key, raw, err)
	}
	return d, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))

	for _, p := range parts {
		trimmed := strings.TrimSpace(p) 
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}