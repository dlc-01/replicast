package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func validEnv(t *testing.T) {
	t.Helper()
	setEnv(t, "NODE_NAME", "node-test")
	setEnv(t, "BASE_URL", "http://localhost:8080")
	setEnv(t, "DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	setEnv(t, "SHARED_SECRET", "secret-at-least-16-chars")
	setEnv(t, "JWT_SECRET", "jwt-secret-long-enough-32-chars!!")
}

func TestLoad_ValidConfig(t *testing.T) {
	validEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "node-test", cfg.NodeName)
	assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, 5*time.Second, cfg.OutboxInterval)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_InternalBaseURL_DefaultsToBaseURL(t *testing.T) {
	validEnv(t)
	os.Unsetenv("INTERNAL_BASE_URL")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, cfg.BaseURL, cfg.InternalBaseURL)
}

func TestLoad_InternalBaseURL_Override(t *testing.T) {
	validEnv(t)
	setEnv(t, "INTERNAL_BASE_URL", "http://node-a:8080")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
	assert.Equal(t, "http://node-a:8080", cfg.InternalBaseURL)
}

func TestLoad_MissingRequired(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "JWT_SECRET", "SHARED_SECRET"} {
		os.Unsetenv(key)
	}

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_JWTSecretTooShort(t *testing.T) {
	validEnv(t)
	setEnv(t, "JWT_SECRET", "short")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_SECRET must be at least 32")
}

func TestLoad_CustomOutboxInterval(t *testing.T) {
	validEnv(t)
	setEnv(t, "OUTBOX_INTERVAL_MS", "2000")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, cfg.OutboxInterval)
}

func TestLoad_InvalidOutboxInterval_UsesDefault(t *testing.T) {
	validEnv(t)
	setEnv(t, "OUTBOX_INTERVAL_MS", "not-a-number")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, cfg.OutboxInterval)
}

func TestLoad_NodeKeyPath_Default_Empty(t *testing.T) {
	validEnv(t)
	os.Unsetenv("NODE_KEY_PATH")

	cfg, err := config.Load()
	require.NoError(t, err)

	// По умолчанию пусто — ключ генерируется эфемерно при старте
	assert.Equal(t, "", cfg.NodeKeyPath)
}

func TestLoad_NodeKeyPath_Override(t *testing.T) {
	validEnv(t)
	setEnv(t, "NODE_KEY_PATH", "/app/keys/node-a.pem")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "/app/keys/node-a.pem", cfg.NodeKeyPath)
}
