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
	"github.com/dlc-01/replicast/internal/comments"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/dms"
	"github.com/dlc-01/replicast/internal/e2e"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/feed"
	"github.com/dlc-01/replicast/internal/follows"
	"github.com/dlc-01/replicast/internal/httpapi"
	"github.com/dlc-01/replicast/internal/likes"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/posts"
	"github.com/dlc-01/replicast/internal/search"
	"github.com/dlc-01/replicast/internal/signing"
	"github.com/dlc-01/replicast/internal/storage"
	"github.com/dlc-01/replicast/internal/users"
	"github.com/dlc-01/replicast/internal/worker"
)

type App struct {
	cfg    *config.Config
	log    logger.Logger
	db     *pgxpool.Pool
	server *http.Server
	worker *worker.OutboxWorker
}

func New(cfg *config.Config, log logger.Logger) (*App, error) {
	log.Info("initializing application", "node", cfg.NodeName, "base_url", cfg.BaseURL)

	db, err := storage.NewPool(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("storage.NewPool: %w", err)
	}

	if err := storage.RunMigrations(cfg.DatabaseURL); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage.RunMigrations: %w", err)
	}
	log.Info("migrations applied")

	// — Инициализация RSA ключей узла для подписи межузловых запросов
	nodeKeyPair, err := initNodeKeys(cfg, log)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init node keys: %w", err)
	}
	if nodeKeyPair != nil {
		// Регистрируем функцию верификации в signing пакете
		signing.VerifyNodeSignatureFunc = e2e.VerifyNodeSignature
		log.Info("node RSA keys loaded", "public_key_len", len(nodeKeyPair.PublicKeyPEM))
	}

	// — Репозитории
	authRepo := auth.NewRepository(db)
	userRepo := users.NewRepository(db)
	postRepo := posts.NewRepository(db)
	followRepo := follows.NewRepository(db)
	feedRepo := feed.NewRepository(db)
	fedRepo := federation.NewRepository(db)
	likeRepo := likes.NewRepository(db)
	commentRepo := comments.NewRepository(db)
	dmRepo := dms.NewRepository(db)

	// — Federation client
	fedClient := federation.NewClient(cfg)

	// — Сервисы
	authSvc := auth.NewService(authRepo, log, cfg)
	userSvc := users.NewServiceWithLogger(userRepo, log, cfg)
	postSvc := posts.NewService(postRepo, feedRepo, fedRepo, userRepo, log, cfg)
	followSvc := follows.NewService(followRepo, userRepo, fedRepo, fedRepo, fedClient, log, cfg)
	feedSvc := feed.NewService(feedRepo)
	fedSvc := federation.NewService(fedRepo, postRepo, userRepo, feedRepo, followRepo, log, cfg)
	likeSvc := likes.NewService(likeRepo, fedRepo, postRepo, log, cfg)
	commentSvc := comments.NewService(commentRepo, fedRepo, postRepo, log, cfg)
	dmSvc := dms.NewService(dmRepo, fedRepo, log, cfg)
	searchSvc := search.NewService(userRepo, fedClient, cfg)

	outboxWorker := worker.NewOutboxWorker(fedRepo, fedClient, cfg)

	router := httpapi.NewRouter(httpapi.Deps{
		AuthSvc:    authSvc,
		UserSvc:    userSvc,
		PostSvc:    postSvc,
		FollowSvc:  followSvc,
		FeedSvc:    feedSvc,
		FedSvc:     fedSvc,
		LikeSvc:    likeSvc,
		CommentSvc: commentSvc,
		DMSvc:      dmSvc,
		SearchSvc:  searchSvc,
		Cfg:        cfg,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &App{cfg: cfg, log: log, db: db, server: srv, worker: outboxWorker}, nil
}

func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)

	go func() {
		a.log.Info("outbox worker started", "interval_ms", a.cfg.OutboxInterval.Milliseconds())
		a.worker.Run(ctx)
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

func (a *App) shutdown() error {
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.server.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	a.db.Close()
	a.log.Info("node stopped gracefully", "node", a.cfg.NodeName)
	return nil
}

// initNodeKeys загружает или генерирует RSA ключи узла.
// Если NODE_KEY_PATH задан — читаем из файла.
// Иначе — генерируем новую пару (эфемерная, не сохраняется между рестартами).
func initNodeKeys(cfg *config.Config, log logger.Logger) (*e2e.NodeKeyPair, error) {
	if cfg.NodeKeyPath != "" {
		data, err := os.ReadFile(cfg.NodeKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read node key: %w", err)
		}
		priv, err := e2e.ParsePrivateKey(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse node key: %w", err)
		}
		kp, err := e2e.NodeKeyPairFromPrivate(priv)
		if err != nil {
			return nil, err
		}
		log.Info("node RSA key loaded from file", "path", cfg.NodeKeyPath)
		return kp, nil
	}

	// Генерируем эфемерный ключ — новый при каждом старте
	kp, err := e2e.GenerateNodeKeyPair()
	if err != nil {
		return nil, err
	}
	log.Info("node RSA key generated (ephemeral)")
	return kp, nil
}
