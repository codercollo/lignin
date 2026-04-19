// Package lignin provides configuration primitives for the Lignin platform.
//
// It centralizes all environment-driven configuration required by the
// gateway, scheduler, and supporting internal services. The package
// enforces sane production defaults while allowing full override via
// environment variables for local development and testing.
package lignin

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the root configuration for all lignin subsystems.
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Auth     AuthConfig
	Callback CallbackConfig
	OTel     OTelConfig
}

// AppConfig defines global application-level settings
type AppConfig struct {
	Env         string `env:"APP_ENV"  envDefault:"production"`
	LogLevel    string `env:"LOG_LEVEL"  envDefault:"info"`
	ServiceName string `env:"SERVICE_NAME"  envDefault:"lignin-gateway"`
}

// ServerConfig defines HTTP server behavior
type ServerConfig struct {
	Host            string        `env:"SERVER_HOST"  envDefault:"0.0.0.0"`
	Port            int           `env:"SERVER_PORT"  envDefault:"8080"`
	ReadTimeout     time.Duration `env:"SERVER_READ_TIMEOUT"  envDefault:"10s"`
	WriteTimeout    time.Duration `env:"SERVER_WRITE_TIMEOUT"     envDefault:"30s"`
	IdleTimeout     time.Duration `env:"SERVER_IDLE_TIMEOUT"      envDefault:"120s"`
	ShutdownTimeout time.Duration `env:"SERVER_SHUTDOWN_TIMEOUT"  envDefault:"15s"`
	RateLimit       float64       `env:"SERVER_RATE_LIMIT" envDefault:"100"`
	RateBurst       int           `env:"SERVER_RATE_BURST" envDefault:"20"`
}

// Addr returns the network address
func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// DatabaseConfig defines Postgres connection settings
type DatabaseConfig struct {
	DSN             string        `env:"DATABASE_DSN,required"`
	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS"    envDefault:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS"    envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"5m"`
	ConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"1m"`

	MigrationsDir string `env:"DB_MIGRATIONS_DIR" envDefault:"migrations"`
}

// RedisConfig defines catching and ephemeral state storage config
type RedisConfig struct {
	Addr     string `env:"REDIS_ADDR"     envDefault:"localhost:6379"`
	Password string `env:"REDIS_PASSWORD" envDefault:""`
	DB       int    `env:"REDIS_DB"       envDefault:"0"`
	PoolSize int    `env:"REDIS_POOL_SIZE" envDefault:"10"`

	TTL time.Duration `env:"REDIS_TTL" envDefault:"15m"`
}

// AuthConfig defines credentials and behavior for external OAuth2/Mpesa auth
// and internal JWT session signing
type AuthConfig struct {
	ConsumerKey    string `env:"MPESA_CONSUMER_KEY,required"`
	ConsumerSecret string `env:"MPESA_CONSUMER_SECRET,required"`
	TokenURL       string `env:"MPESA_TOKEN_URL,required"`

	TokenBufferDuration time.Duration `env:"AUTH_TOKEN_BUFFER" envDefault:"60s"`
	JWTSecret           string        `env:"JWT_SECRET,required"`
	JWTExpiry           time.Duration `env:"JWT_EXPIRY" envDefault:"24h"`
}

// CallbackConfig defines security and processing constraints for inbound
// webhook callbacks
type CallbackConfig struct {
	SignatureHeader string        `env:"CALLBACK_SIG_HEADER" envDefault:"X-Mpesa-Signature"`
	SignatureSecret string        `env:"CALLBACK_SIG_SECRET,required"`
	DedupTTL        time.Duration `env:"CALLBACK_DEDUP_TTL" envDefault:"24h"`
	MaxBodyBytes    int64         `env:"CALLBACK_MAX_BODY_BYTES" envDefault:"1048576"`
}

// OTelConfig defines OpenTelemetry tracing and metrics export config
type OTelConfig struct {
	Endpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:""`
	Enabled  bool   `env:"OTEL_ENABLED" envDefault:"false"`
}

// Load parses configuration from environment variables and validates it.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("lignin: parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("lignin: validate config: %w", err)
	}

	return cfg, nil
}

// validate performs internal consistency checks on config values
func (c *Config) validate() error {
	if c.App.Env != "production" && c.App.Env != "staging" &&
		c.App.Env != "development" && c.App.Env != "test" {
		return fmt.Errorf("APP_ENV must be production|staging|development|test, got %q", c.App.Env)
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("SERVER_PORT must be 1-65535, got %d", c.Server.Port)
	}

	if c.Database.MaxOpenConns < c.Database.MaxIdleConns {
		return fmt.Errorf(
			"DB_MAX_OPEN_CONNS (%d) must be >= DB_MAX_IDLE_CONNS (%d)",
			c.Database.MaxOpenConns,
			c.Database.MaxIdleConns,
		)
	}

	if c.Auth.TokenBufferDuration <= 0 {
		return fmt.Errorf("AUTH_TOKEN_BUFFER must be positive")
	}

	return nil

}

// IsDevelopment returns true if the application is running in dev mode
func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

// IsTest returns true if the application is running in test mode
func (c *Config) IsTest() bool {
	return c.App.Env == "test"
}
