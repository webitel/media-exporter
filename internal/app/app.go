package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/auth"
	"github.com/webitel/media-exporter/auth/manager/webitel_app"
	cfg "github.com/webitel/media-exporter/config"
	cache "github.com/webitel/media-exporter/internal/cache/redis"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/server"
	"github.com/webitel/media-exporter/internal/store"
	"github.com/webitel/media-exporter/internal/store/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type App struct {
	config         *cfg.AppConfig
	log            *slog.Logger
	exitCh         chan error
	shutdown       func(ctx context.Context) error
	Store          store.Store
	sessionManager auth.Manager
	cache          *cache.RedisCache
	server         *server.Server
	storageClient  storage.FileServiceClient

	// gRPC connections
	storageConn    *grpc.ClientConn
	webitelAppConn *grpc.ClientConn
}

// New creates a fully initialized App.
func New(config *cfg.AppConfig, shutdown func(ctx context.Context) error) (*App, error) {
	app := &App{
		config:   config,
		shutdown: shutdown,
		exitCh:   make(chan error),
	}

	if err := app.initStore(); err != nil {
		return nil, err
	}
	if err := app.initRedis(); err != nil {
		return nil, err
	}
	if err := app.initGRPCClients(); err != nil {
		return nil, err
	}
	if err := app.initSessionManager(); err != nil {
		return nil, err
	}
	if err := app.initServer(); err != nil {
		return nil, err
	}

	// --------- Service Registration (GRPC) ---------
	RegisterServices(app.server.Server, app)

	return app, nil
}

// --------- Private init methods ---------

func (app *App) initStore() error {
	if app.config.Database == nil {
		return errors.New("database config is nil")
	}
	app.Store = postgres.New(app.config.Database)
	return nil
}

func (app *App) initRedis() error {
	redisCache, err := cache.NewRedisCache(app.config.Redis.Addr, app.config.Redis.Password, app.config.Redis.DB)
	if err != nil {
		return errors.New("unable to initialize Redis", errors.WithCause(err))
	}
	app.cache = redisCache
	return nil
}

func (app *App) initGRPCClients() error {
	var err error

	app.storageConn, err = grpc.NewClient(
		fmt.Sprintf("consul://%s/storage?wait=14s", app.config.Consul.Address),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy": "round_robin"}`),
	)
	if err != nil {
		return errors.New("unable to create storage client", errors.WithCause(err))
	}
	app.storageClient = storage.NewFileServiceClient(app.storageConn)

	app.webitelAppConn, err = grpc.NewClient(
		fmt.Sprintf("consul://%s/go.webitel.app?wait=14s", app.config.Consul.Address),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy": "round_robin"}`),
	)
	if err != nil {
		return errors.New("unable to create webitel app client", errors.WithCause(err))
	}
	return nil
}

func (app *App) initSessionManager() error {
	manager, err := webitel_app.New(app.webitelAppConn)
	if err != nil {
		return errors.New("failed to init session manager", errors.WithCause(err))
	}
	app.sessionManager = manager
	return nil
}

func (app *App) initServer() error {
	srv, err := server.BuildServer(app.config.Consul, app.sessionManager, app.exitCh)
	if err != nil {
		return errors.New("failed to build server", errors.WithCause(err))
	}
	app.server = srv
	return nil
}

// Start runs DB, gRPC server and background workers
func (app *App) Start(ctx context.Context) error {
	if err := app.Store.Open(); err != nil {
		return errors.New("failed to open store", errors.WithCause(err))
	}

	go app.server.Start()
	app.StartExportWorker(ctx)

	return <-app.exitCh
}

// Stop gracefully shuts down all services
func (app *App) Stop() error {
	slog.Info("media_exporter.main.stop_starting")

	if app.server != nil {
		app.server.Stop()
		slog.Info("server stopped")
	}

	if app.storageConn != nil {
		if err := app.storageConn.Close(); err != nil {
			slog.Error("storageConn close error", "err", err)
		} else {
			slog.Info("storageConn closed")
		}
	}
	if app.webitelAppConn != nil {
		if err := app.webitelAppConn.Close(); err != nil {
			slog.Error("webitelAppConn close error", "err", err)
		} else {
			slog.Info("webitelAppConn closed")
		}
	}

	if app.cache != nil {
		if err := app.cache.Clear(); err != nil {
			slog.Error("redis cache clear error", "err", err)
		} else {
			slog.Info("redis cache cleared")
		}
	}

	if app.shutdown != nil {
		if err := app.shutdown(context.Background()); err != nil {
			slog.Error("shutdown hook error", "err", err)
		} else {
			slog.Info("shutdown hook executed")
		}
	}

	slog.Info("media_exporter.main.stop_complete")
	return nil
}
