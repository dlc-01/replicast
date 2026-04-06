package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	NodeName        string
	BaseURL         string
	InternalBaseURL string
	Port            string
	DatabaseURL     string
	JWTSecret       string
	SharedSecret    string
	OutboxInterval  time.Duration
	LogFormat       string
	LogLevel        string
	NodeKeyPath     string // путь к файлу с RSA приватным ключом узла (опционально)
}

func Load() (*Config, error) {
	cfg := &Config{
		NodeName:        getEnv("NODE_NAME", "node-a"),
		BaseURL:         getEnv("BASE_URL", "http://localhost:8080"),
		InternalBaseURL: getEnv("INTERNAL_BASE_URL", ""),
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     getEnv("DATABASE_URL", ""),
		JWTSecret:       getEnv("JWT_SECRET", ""),
		SharedSecret:    getEnv("SHARED_SECRET", ""),
		LogFormat:       getEnv("LOG_FORMAT", "json"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		OutboxInterval:  parseMillis(getEnv("OUTBOX_INTERVAL_MS", "5000")),
		NodeKeyPath:     getEnv("NODE_KEY_PATH", ""),
	}
	// Если INTERNAL_BASE_URL не задан — используем BASE_URL
	if cfg.InternalBaseURL == "" {
		cfg.InternalBaseURL = cfg.BaseURL
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// WellKnownBaseURL возвращает URL для discovery через /.well-known.
// Внутри Docker используем InternalBaseURL, снаружи — BASE_URL.
func (c *Config) WellKnownBaseURL() string {
	return c.InternalBaseURL
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	if c.SharedSecret == "" {
		return fmt.Errorf("SHARED_SECRET is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseMillis(s string) time.Duration {
	ms, err := strconv.Atoi(s)
	if err != nil {
		return 5 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}
