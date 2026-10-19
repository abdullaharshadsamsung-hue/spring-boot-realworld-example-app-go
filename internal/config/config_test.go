package config

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	all := []string{EnvDatabaseURL, EnvPort, EnvDefaultImage, EnvJWTSecret, EnvJWTSessionTime, EnvStatementTimeout, EnvSQLLogLevel}
	for _, k := range all {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{EnvDatabaseURL: "postgres://localhost/db"})
	// Empty-string env vars: viper treats set-but-empty as unset only if AllowEmptyEnv false (default).
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/db" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %q", cfg.Port)
	}
	if cfg.Addr() != ":8080" {
		t.Errorf("Addr = %q", cfg.Addr())
	}
	if cfg.DefaultImage != "https://static.productionready.io/images/smiley-cyrus.jpg" {
		t.Errorf("DefaultImage = %q", cfg.DefaultImage)
	}
	if cfg.JWTSecret != DefaultJWTSecret {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.JWTSessionTime != 86400*time.Second {
		t.Errorf("JWTSessionTime = %v", cfg.JWTSessionTime)
	}
	if cfg.StatementTimeout != 3000*time.Second {
		t.Errorf("StatementTimeout = %v", cfg.StatementTimeout)
	}
	if cfg.SQLLogLevel != zerolog.DebugLevel {
		t.Errorf("SQLLogLevel = %v", cfg.SQLLogLevel)
	}
}

func TestLoadOverrides(t *testing.T) {
	setEnv(t, map[string]string{
		EnvDatabaseURL:      "  postgres://x/y  ",
		EnvPort:             " 9090 ",
		EnvDefaultImage:     "http://img/a.png",
		EnvJWTSecret:        "other-secret",
		EnvJWTSessionTime:   " 60 ",
		EnvStatementTimeout: "5",
		EnvSQLLogLevel:      " WARN ",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://x/y" || cfg.Port != "9090" || cfg.Addr() != ":9090" {
		t.Errorf("bad url/port: %+v", cfg)
	}
	if cfg.DefaultImage != "http://img/a.png" || cfg.JWTSecret != "other-secret" {
		t.Errorf("bad image/secret: %+v", cfg)
	}
	if cfg.JWTSessionTime != time.Minute || cfg.StatementTimeout != 5*time.Second {
		t.Errorf("bad durations: %+v", cfg)
	}
	if cfg.SQLLogLevel != zerolog.WarnLevel {
		t.Errorf("SQLLogLevel = %v", cfg.SQLLogLevel)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		is      error
		contain string
	}{
		{"missing db url", map[string]string{}, ErrMissingDatabaseURL, ""},
		{"blank db url", map[string]string{EnvDatabaseURL: "   "}, ErrMissingDatabaseURL, ""},
		{"session not int", map[string]string{EnvDatabaseURL: "d", EnvJWTSessionTime: "abc"}, nil, EnvJWTSessionTime},
		{"session zero", map[string]string{EnvDatabaseURL: "d", EnvJWTSessionTime: "0"}, nil, "must be positive"},
		{"session negative", map[string]string{EnvDatabaseURL: "d", EnvJWTSessionTime: "-5"}, nil, "must be positive"},
		{"timeout not int", map[string]string{EnvDatabaseURL: "d", EnvStatementTimeout: "1.5"}, nil, EnvStatementTimeout},
		{"timeout zero", map[string]string{EnvDatabaseURL: "d", EnvStatementTimeout: "0"}, nil, EnvStatementTimeout},
		{"bad log level", map[string]string{EnvDatabaseURL: "d", EnvSQLLogLevel: "verbose"}, nil, EnvSQLLogLevel},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)
			cfg, err := Load()
			if err == nil {
				t.Fatalf("expected error, got cfg %+v", cfg)
			}
			if cfg != nil {
				t.Errorf("expected nil cfg")
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("err = %v, want %v", err, tc.is)
			}
			if tc.contain != "" && !strings.Contains(err.Error(), tc.contain) {
				t.Errorf("err %q missing %q", err, tc.contain)
			}
		})
	}
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		in   string
		want zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"INFO", zerolog.InfoLevel},
		{"error", zerolog.ErrorLevel},
		{"trace", zerolog.TraceLevel},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			setEnv(t, map[string]string{EnvDatabaseURL: "d", EnvSQLLogLevel: tc.in})
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SQLLogLevel != tc.want {
				t.Errorf("got %v want %v", cfg.SQLLogLevel, tc.want)
			}
		})
	}
}

func TestAddr(t *testing.T) {
	tests := []struct{ port, want string }{{"8080", ":8080"}, {"1", ":1"}, {"", ":"}}
	for _, tc := range tests {
		c := &Config{Port: tc.port}
		if got := c.Addr(); got != tc.want {
			t.Errorf("Addr(%q) = %q, want %q", tc.port, got, tc.want)
		}
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultImageURL != "https://static.productionready.io/images/smiley-cyrus.jpg" {
		t.Error("default image changed")
	}
	if DefaultJWTSessionSeconds != 86400 {
		t.Error("session lifetime changed")
	}
	if DefaultStatementTimeoutSeconds != 3000 {
		t.Error("statement timeout changed")
	}
	if DefaultSQLLogLevel != "debug" {
		t.Error("sql log level changed")
	}
}