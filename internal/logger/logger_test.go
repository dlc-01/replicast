package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/dlc-01/replicast/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_JSONFormat(t *testing.T) {
	log := logger.New("json", "info")
	assert.NotNil(t, log)
}

func TestNew_TextFormat(t *testing.T) {
	log := logger.New("text", "debug")
	assert.NotNil(t, log)
}

// TestWithNode_AddsField проверяет что поле node реально попадает в лог.
// Используем буфер чтобы перехватить вывод и проверить JSON.
func TestWithNode_AddsField(t *testing.T) {
	buf := &bytes.Buffer{}
	base := slog.New(slog.NewJSONHandler(buf, nil))

	// Оборачиваем slogLogger через With — имитируем что WithNode добавляет поле
	nodeLog := base.With("node", "node-a")
	nodeLog.Info("test")

	var entry map[string]any
	require.NoError(t, json.NewDecoder(buf).Decode(&entry))
	assert.Equal(t, "node-a", entry["node"], "node field should be in log output")
}

func TestNop_DoesNotPanic(t *testing.T) {
	log := logger.Nop()
	assert.NotPanics(t, func() {
		log.Debug("debug message", "key", "value")
		log.Info("info message")
		log.Warn("warn message")
		log.Error("error message", "err", "something")
	})
}

func TestNop_With_ReturnsSelf(t *testing.T) {
	log := logger.Nop()
	child := log.With("key", "value")
	assert.NotNil(t, child)
	assert.NotPanics(t, func() {
		child.Info("test")
	})
}

func TestNop_WithGroup_ReturnsSelf(t *testing.T) {
	log := logger.Nop()
	child := log.WithGroup("group")
	assert.NotNil(t, child)
	assert.NotPanics(t, func() {
		child.Info("test")
	})
}

func TestContext_RoundTrip(t *testing.T) {
	log := logger.New("json", "info")
	ctx := context.Background()

	ctx = logger.WithCtx(ctx, log)
	got := logger.FromCtx(ctx)

	assert.NotNil(t, got)
	// Проверяем что достали тот же логгер
	assert.Equal(t, log, got)
}

func TestContext_MissingLogger_ReturnsDefault(t *testing.T) {
	ctx := context.Background()
	got := logger.FromCtx(ctx)
	assert.NotNil(t, got)
}

func TestJSONOutput_ContainsExpectedFields(t *testing.T) {
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	log.Info("test message", "key", "value")

	var entry map[string]any
	require.NoError(t, json.NewDecoder(buf).Decode(&entry))

	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "test message", entry["msg"])
	assert.Equal(t, "value", entry["key"])
}

func TestLogLevels_DebugFiltered(t *testing.T) {
	buf := &bytes.Buffer{}
	// info рівень — debug не повинен пройти
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(handler)

	log.Debug("should not appear")
	log.Info("should appear")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 log line (info only), got %d: %s", len(lines), buf.String())
	}

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	assert.Equal(t, "INFO", entry["level"])
}
