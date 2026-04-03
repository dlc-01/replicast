package users_test

import (
	"context"
	"os"
	"testing"

	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/storage"
	"github.com/dlc-01/replicast/internal/testhelper"
	"github.com/dlc-01/replicast/internal/users"
)

func TestUserRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("docker not available")
	}

	ctx := context.Background()
	dsn := testhelper.StartPostgres(t)

	pool, err := storage.NewPool(dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	testhelper.RunMigrations(t, dsn)

	repo := users.NewRepository(pool)

	t.Run("create and get by global id", func(t *testing.T) {
		u := port.User{
			GlobalID:      "alice@node-a",
			LocalUsername: "alice",
			HomeNode:      "node-a",
			DisplayName:   "Alice",
			Bio:           "hello",
			PasswordHash:  "hash",
		}
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("create: %v", err)
		}

		got, err := repo.GetByGlobalID(ctx, "alice@node-a")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got == nil {
			t.Fatal("expected user, got nil")
		}
		if got.GlobalID != u.GlobalID {
			t.Errorf("global_id = %q, want %q", got.GlobalID, u.GlobalID)
		}
	})

	t.Run("username exists", func(t *testing.T) {
		exists, err := repo.UsernameExists(ctx, "alice")
		if err != nil {
			t.Fatalf("exists: %v", err)
		}
		if !exists {
			t.Error("expected alice to exist")
		}
	})

	t.Run("get by username not found", func(t *testing.T) {
		_, err := repo.GetByUsername(ctx, "nobody")
		if err == nil {
			t.Error("expected error for unknown user, got nil")
		}
	})
}
