package main

import (
	"os"

	"github.com/dlc-01/replicast/internal/app"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"log/slog"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.WithNode(logger.New(cfg.LogFormat, cfg.LogLevel), cfg.NodeName)
	logger.SetDefault(log)

	application, err := app.New(cfg, log)
	if err != nil {
		return err
	}

	return application.Run()
}
