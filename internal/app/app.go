package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dlc-01/replicast/internal/auth"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/feed"
	"github.com/dlc-01/replicast/internal/follows"
	"github.com/dlc-01/replicast/internal/httpapi"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/posts"
	"github.com/dlc-01/replicast/internal/storage"
	"github.com/dlc-01/replicast/internal/users"
	"github.com/dlc-01/replicast/internal/worker"
)

// App — корневой объект приложения.
// Владеет всеми зависимостями и управляет их жизненным циклом.
type App struct {
	cfg    *config.Config
	log    logger.Logger
	db     *pgxpool.Pool
	server *http.Server
	worker *worker.OutboxWorker
}

// New собирает граф зависимостей.
// Все ошибки инициализации возвращаются — никакой паники.
func New(cfg *config.Config, log logger.Logger) (*App, error) {
	log.Info("initializing application",
		"node", cfg.NodeName,
		"base_url", cfg.BaseURL,
	)

	db, err := storage.NewPool(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("storage.NewPool: %w", err)
	}
	log.Info("database connected")

	if err := storage.RunMigrations(cfg.DatabaseURL); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage.RunMigrations: %w", err)
	}
	log.Info("migrations applied")

	// — Репозитории
	authRepo := auth.NewRepository(db)
	userRepo := users.NewRepository(db)
	postRepo := posts.NewRepository(db)
	followRepo := follows.NewRepository(db)
	feedRepo := feed.NewRepository(db)
	fedRepo := federation.NewRepository(db)

	// — Сервисы (logger пробрасывается во все)
	authSvc := auth.NewService(authRepo, log, cfg)
	userSvc := users.NewServiceWithLogger(userRepo, log, cfg)
	postSvc := posts.NewService(postRepo, feedRepo, fedRepo, userRepo, log, cfg)
	followSvc := follows.NewService(followRepo, userRepo, fedRepo, log, cfg)
	feedSvc := feed.NewService(feedRepo)
	fedSvc := federation.NewService(fedRepo, postRepo, userRepo, feedRepo, cfg)

	fedClient := federation.NewClient(cfg)
	outboxWorker := worker.NewOutboxWorker(fedRepo, fedClient, cfg)

	router := httpapi.NewRouter(httpapi.Deps{
		AuthSvc: authSvc, UserSvc: userSvc, PostSvc: postSvc,
		FollowSvc: followSvc, FeedSvc: feedSvc, FedSvc: fedSvc,
		Cfg: cfg,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &App{
		cfg:    cfg,
		log:    log,
		db:     db,
		server: srv,
		worker: outboxWorker,
	}, nil
}

// Run запускает все компоненты и блокируется до сигнала завершения.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Только для критических ошибок HTTP сервера
	serverErr := make(chan error, 1)

	// Worker останавливается по ctx — не пишет в канал при нормальном завершении
	go func() {
		a.log.Info("outbox worker started",
			"interval_ms", a.cfg.OutboxInterval.Milliseconds(),
		)
		a.worker.Run(ctx)
		a.log.Info("outbox worker stopped")
	}()

	go func() {
		a.log.Info("http server listening", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("http server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		a.log.Info("shutdown signal received")
	case err := <-serverErr:
		return err
	}

	return a.shutdown()
}

// shutdown корректно останавливает компоненты в обратном порядке инициализации.
func (a *App) shutdown() error {
	a.log.Info("shutting down gracefully...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.server.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	a.log.Info("http server stopped")

	a.db.Close()
	a.log.Info("database pool closed")

	a.log.Info("node stopped gracefully", "node", a.cfg.NodeName)
	return nil
}
