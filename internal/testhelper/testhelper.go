package testhelper

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartPostgres поднимает postgres контейнер и возвращает DSN.
func StartPostgres(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		if os.Getenv("DOCKER_HOST") == "" {
			t.Skip("docker not available")
		}
	}

	ctx := context.Background()
	container, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("replicast_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}

// RunMigrations запускает миграции используя абсолютный путь.
// Работает из любой директории теста.
func RunMigrations(t *testing.T, dsn string) {
	t.Helper()

	// Определяем корень проекта относительно этого файла:
	// testhelper/ → internal/ → project root
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		t.Fatalf("migrations: %v", err)
	}
}
