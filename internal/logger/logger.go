package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Logger — интерфейс логгера.
// Позволяет подменять реализацию в тестах без slog зависимости.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
	WithGroup(name string) Logger
}

// — Реализация поверх slog ─────────────────────────────────────────

type slogLogger struct {
	log *slog.Logger
}

func (l *slogLogger) Debug(msg string, args ...any) { l.log.Debug(msg, args...) }
func (l *slogLogger) Info(msg string, args ...any)  { l.log.Info(msg, args...) }
func (l *slogLogger) Warn(msg string, args ...any)  { l.log.Warn(msg, args...) }
func (l *slogLogger) Error(msg string, args ...any) { l.log.Error(msg, args...) }

func (l *slogLogger) With(args ...any) Logger {
	return &slogLogger{log: l.log.With(args...)}
}

func (l *slogLogger) WithGroup(name string) Logger {
	return &slogLogger{log: l.log.WithGroup(name)}
}

// Slog возвращает underlying *slog.Logger — нужен для middleware и slog.SetDefault.
func (l *slogLogger) Slog() *slog.Logger { return l.log }

// — Конструкторы ──────────────────────────────────────────────────

// New создаёт логгер из конфига.
// LOG_FORMAT=text  — читаемый вывод для разработки.
// LOG_FORMAT=json  — JSON для прода (Loki, CloudWatch).
// LOG_LEVEL=debug|info|warn|error
func New(format, level string) Logger {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
	}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return &slogLogger{log: slog.New(handler)}
}

// WithNode добавляет поле node ко всем логам.
// Удобно когда смотришь логи node-a, node-b, node-c вместе.
func WithNode(log Logger, nodeName string) Logger {
	return log.With("node", nodeName)
}

// Nop возвращает логгер который ничего не делает — для тестов.
func Nop() Logger {
	return &nopLogger{}
}

// SetDefault устанавливает логгер как глобальный slog.Default().
// Вызывается один раз в main.
func SetDefault(log Logger) {
	if l, ok := log.(*slogLogger); ok {
		slog.SetDefault(l.log)
	}
}

// — Контекст ───────────────────────────────────────────────────────

type ctxKey struct{}

// FromCtx достаёт логгер из контекста.
func FromCtx(ctx context.Context) Logger {
	if log, ok := ctx.Value(ctxKey{}).(Logger); ok {
		return log
	}
	return New("json", "info")
}

// WithCtx кладёт логгер в контекст.
func WithCtx(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, log)
}

// — Nop реализация (тесты) ─────────────────────────────────────────

type nopLogger struct{}

func (n *nopLogger) Debug(msg string, args ...any) {}
func (n *nopLogger) Info(msg string, args ...any)  {}
func (n *nopLogger) Warn(msg string, args ...any)  {}
func (n *nopLogger) Error(msg string, args ...any) {}
func (n *nopLogger) With(args ...any) Logger       { return n }
func (n *nopLogger) WithGroup(name string) Logger  { return n }

// — Хелперы ────────────────────────────────────────────────────────

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
