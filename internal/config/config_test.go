package config_test

import (
	"testing"

	"github.com/dlc-01/replicast/internal/config"
)

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("SHARED_SECRET", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestLoad_Valid(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x:x@localhost/x")
	t.Setenv("JWT_SECRET", "supersecretkey-that-is-long-enough-32ch")
	t.Setenv("SHARED_SECRET", "nodesharedsecret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.NodeName != "node-a" {
		t.Errorf("expected default node name node-a, got %s", cfg.NodeName)
	}
}
