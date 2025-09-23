package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/webitel/media-exporter/auth"
	"github.com/webitel/media-exporter/auth/manager/webitel_app"
	"github.com/webitel/media-exporter/internal/server"
	"github.com/webitel/media-exporter/internal/store"
	"github.com/webitel/media-exporter/internal/store/postgres"

	"github.com/webitel/media-exporter/api/storage"
	cfg "github.com/webitel/media-exporter/config"
	cache "github.com/webitel/media-exporter/internal/cache/redis"
	"github.com/webitel/media-exporter/internal/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type App struct {
	config         *cfg.AppConfig
	Store          store.Store
	server         *server.Server
	exitCh         chan error
	sessionManager auth.Manager
	shutdown       func(ctx context.Context) error
	log            *slog.Logger
	cache          *cache.RedisCache
	storageConn    *grpc.ClientConn
	storageClient  storage.FileServiceClient
	webitelAppConn *grpc.ClientConn
}

func New(config *cfg.AppConfig, shutdown func(ctx context.Context) error) (*App, error) {
	// --------- App Initialization ---------
	app := &App{config: config, shutdown: shutdown}
	var err error

	// --------- DB Initialization ---------
	if config.Database == nil {
		return nil, errors.New("error creating store, config is nil")
	}
	app.Store = BuildDatabase(config.Database)

	// --------- Storage gRPC Connection ---------
	app.storageConn, err = grpc.NewClient(fmt.Sprintf("consul://%s/storage?wait=14s", config.Consul.Address),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy": "round_robin"}`),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	app.storageClient = storage.NewFileServiceClient(app.storageConn)

	if err != nil {
		return nil, errors.New("unable to create storage client", errors.WithCause(err))
	}

	// --------- Webitel App gRPC Connection ---------
	app.webitelAppConn, err = grpc.NewClient(fmt.Sprintf("consul://%s/go.webitel.app?wait=14s", config.Consul.Address),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy": "round_robin"}`),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		return nil, errors.New("unable to create contact group client", errors.WithCause(err))
	}

	// --------- Session Manager Initialization ---------
	app.sessionManager, err = webitel_app.New(app.webitelAppConn)
	if err != nil {
		return nil, err
	}

	// --------- Redis Initialization ---------
	app.cache, err = cache.NewRedisCache(
		config.Redis.Addr,
		config.Redis.Password,
		config.Redis.DB,
	)
	if err != nil {
		return nil, errors.New("unable to initialize Redis cache", errors.WithCause(err))
	}

	// --------- gRPC Server Initialization ---------
	s, err := server.BuildServer(app.config.Consul, app.sessionManager, app.exitCh)
	if err != nil {
		return nil, err
	}
	app.server = s

	// --------- Service Registration ---------
	RegisterServices(app.server.Server, app)

	return app, nil
}

func BuildDatabase(config *cfg.DatabaseConfig) store.Store {
	return postgres.New(config)
}

func (a *App) Start() error { // Change return type to standard error
	err := a.Store.Open()
	if err != nil {
		return errors.New("failed to open store", errors.WithCause(err))
	}
	// * run grpc server
	go a.server.Start()
	return <-a.exitCh
}

func (a *App) Stop() error { // Change return type to standard error
	// close massive modules
	a.server.Stop()
	// close grpc connections
	err := a.storageConn.Close()
	if err != nil {
		return err
	}
	err = a.webitelAppConn.Close()
	if err != nil {
		return err
	}

	// ----- Call the shutdown function for OTel ----- //
	if a.shutdown != nil {
		err := a.shutdown(context.Background())
		if err != nil {
			return err
		}
	}

	return nil
}
