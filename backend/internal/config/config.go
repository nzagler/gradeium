package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	envDatabaseURL  = "GRADEIUM_DATABASE_URL"
	envListen       = "GRADEIUM_LISTEN_ADDRESS"
	envConfigDir    = "GRADEIUM_CONFIG_DIR"
	envBackupsDir   = "GRADEIUM_BACKUPS_DIR"
	envWebDir       = "GRADEIUM_WEB_DIR"
	envLogLevel     = "GRADEIUM_LOG_LEVEL"
	defaultListen   = ":8080"
	defaultConfig   = "/config"
	defaultBackups  = "/backups"
	defaultWeb      = "frontend"
	defaultLogLevel = "info"
)

// Config contains only infrastructure values required before application settings are available.
type Config struct {
	DatabaseURL   string
	ListenAddress string
	ConfigDir     string
	BackupsDir    string
	WebDir        string
	LogLevel      slog.Level
}

// Load reads and validates Gradeium's bootstrap environment configuration.
func Load() (Config, error) {
	return LoadFrom(os.Getenv)
}

// LoadFrom supports deterministic configuration tests without mutating process state.
func LoadFrom(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:   strings.TrimSpace(getenv(envDatabaseURL)),
		ListenAddress: valueOrDefault(getenv(envListen), defaultListen),
		ConfigDir:     valueOrDefault(getenv(envConfigDir), defaultConfig),
		BackupsDir:    valueOrDefault(getenv(envBackupsDir), defaultBackups),
		WebDir:        valueOrDefault(getenv(envWebDir), defaultWeb),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("%s is required", envDatabaseURL)
	}
	if _, err := pgxpool.ParseConfig(cfg.DatabaseURL); err != nil {
		return Config{}, fmt.Errorf("%s must be a valid PostgreSQL connection string", envDatabaseURL)
	}
	if err := validateListenAddress(cfg.ListenAddress); err != nil {
		return Config{}, fmt.Errorf("%s: %w", envListen, err)
	}
	if cfg.ConfigDir == "" {
		return Config{}, fmt.Errorf("%s must not be empty", envConfigDir)
	}
	if cfg.BackupsDir == "" {
		return Config{}, fmt.Errorf("%s must not be empty", envBackupsDir)
	}
	if cfg.WebDir == "" {
		return Config{}, fmt.Errorf("%s must not be empty", envWebDir)
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(valueOrDefault(getenv(envLogLevel), defaultLogLevel))); err != nil {
		return Config{}, fmt.Errorf("%s must be one of debug, info, warn, or error", envLogLevel)
	}

	return cfg, nil
}

func valueOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func validateListenAddress(address string) error {
	if strings.TrimSpace(address) == "" {
		return errors.New("must not be empty")
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("must be in host:port form")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("must contain a port between 1 and 65535")
	}
	return nil
}
