// Package config loads the runtime configuration of the RealWorld (Conduit)
// API from environment variables. It replaces the Spring Boot
// application.properties file of the source project.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

// Environment variable names understood by Load.
const (
	EnvDatabaseURL      = "DATABASE_URL"
	EnvPort             = "PORT"
	EnvDefaultImage     = "IMAGE_DEFAULT"
	EnvJWTSecret        = "JWT_SECRET"
	EnvJWTSessionTime   = "JWT_SESSION_TIME"
	EnvStatementTimeout = "DB_STATEMENT_TIMEOUT"
	EnvSQLLogLevel      = "SQL_LOG_LEVEL"
)

// Default values, copied verbatim from the source application.properties
// where the source defined them.
const (
	// DefaultPort is the HTTP listen port (Spring Boot's implicit default).
	DefaultPort = "8080"
	// DefaultImageURL is the avatar assigned to users who register without
	// an image (source: image.default).
	DefaultImageURL = "https://static.productionready.io/images/smiley-cyrus.jpg"
	// DefaultJWTSecret is the token signing secret (source: jwt.secret).
	// MIGRATION_NOTE: committed to source control in the original project;
	// override JWT_SECRET in every non-development environment.
	DefaultJWTSecret = "nRvyYC4soFxBdZ-F-5Nnzz5USXstR1YylsTd-mA0aKtI9HUlriGrtkf-TiuDapkLiUCogO3JOK7kwZisrHp6wA"
	// DefaultJWTSessionSeconds is the token lifetime in seconds
	// (source: jwt.sessionTime).
	DefaultJWTSessionSeconds = 86400
	// DefaultStatementTimeoutSeconds is the per-statement DB timeout in
	// seconds (source: mybatis.configuration.default-statement-timeout).
	DefaultStatementTimeoutSeconds = 3000
	// DefaultSQLLogLevel mirrors the DEBUG level the source set for the
	// MyBatis mappers and ArticleReadService.
	DefaultSQLLogLevel = "debug"
)

// ErrMissingDatabaseURL is returned by Load when DATABASE_URL is not set.
var ErrMissingDatabaseURL = errors.New("config: " + EnvDatabaseURL + " must be set")

// Config is the fully-resolved application configuration.
type Config struct {
	// DatabaseURL is the PostgreSQL DSN.
	// MIGRATION_NOTE: the source used SQLite (jdbc:sqlite:dev.db with empty
	// username/password); the migrated project targets PostgreSQL, so the
	// file path is replaced by a DSN.
	DatabaseURL string
	// Port is the HTTP listen port.
	Port string
	// DefaultImage is the avatar URL given to users registering without one.
	DefaultImage string
	// JWTSecret is the HMAC secret used to sign and verify tokens.
	JWTSecret string
	// JWTSessionTime is how long an issued token stays valid.
	JWTSessionTime time.Duration
	// StatementTimeout bounds each database call; apply it with
	// context.WithTimeout around queries.
	StatementTimeout time.Duration
	// SQLLogLevel is the log level used by the data-access layer.
	SQLLogLevel zerolog.Level
}

// Addr returns the listen address for net/http (":" + Port).
func (c *Config) Addr() string {
	return ":" + c.Port
}

// Load reads configuration from the environment, applying the source
// project's defaults for every unset variable. It returns an error if a
// numeric or level variable cannot be parsed or a required value is missing.
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()

	// SetDefault/BindEnv register each key so lookups see real env vars.
	v.SetDefault(EnvPort, DefaultPort)
	v.SetDefault(EnvDefaultImage, DefaultImageURL)
	v.SetDefault(EnvJWTSecret, DefaultJWTSecret)
	v.SetDefault(EnvJWTSessionTime, strconv.Itoa(DefaultJWTSessionSeconds))
	v.SetDefault(EnvStatementTimeout, strconv.Itoa(DefaultStatementTimeoutSeconds))
	v.SetDefault(EnvSQLLogLevel, DefaultSQLLogLevel)
	if err := v.BindEnv(EnvDatabaseURL); err != nil {
		return nil, fmt.Errorf("config: bind %s: %w", EnvDatabaseURL, err)
	}

	cfg := &Config{
		DatabaseURL:  strings.TrimSpace(v.GetString(EnvDatabaseURL)),
		Port:         strings.TrimSpace(v.GetString(EnvPort)),
		DefaultImage: v.GetString(EnvDefaultImage),
		JWTSecret:    v.GetString(EnvJWTSecret),
	}
	if cfg.DatabaseURL == "" {
		return nil, ErrMissingDatabaseURL
	}
	if cfg.Port == "" {
		cfg.Port = DefaultPort
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("config: %s must not be empty", EnvJWTSecret)
	}

	session, err := positiveSeconds(v, EnvJWTSessionTime)
	if err != nil {
		return nil, err
	}
	cfg.JWTSessionTime = session

	timeout, err := positiveSeconds(v, EnvStatementTimeout)
	if err != nil {
		return nil, err
	}
	cfg.StatementTimeout = timeout

	lvl, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(v.GetString(EnvSQLLogLevel))))
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", EnvSQLLogLevel, err)
	}
	cfg.SQLLogLevel = lvl

	return cfg, nil
}

// positiveSeconds parses key as a strictly positive integer number of seconds.
// viper.GetInt silently yields 0 for garbage, so the raw string is parsed here.
func positiveSeconds(v *viper.Viper, key string) (time.Duration, error) {
	raw := strings.TrimSpace(v.GetString(key))
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not an integer: %w", key, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: %s=%d must be positive", key, n)
	}
	return time.Duration(n) * time.Second, nil
}
