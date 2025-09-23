package cmd

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	conf "github.com/webitel/media-exporter/config"
	"github.com/webitel/media-exporter/internal/app"
	"github.com/webitel/media-exporter/internal/model"
	logging "github.com/webitel/media-exporter/internal/otel"

	// ------------ logging ------------ //
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	// -------------------- plugin(s) -------------------- //
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/log/otlp"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/log/stdout"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/metric/otlp"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/metric/stdout"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/trace/otlp"
	_ "github.com/webitel/webitel-go-kit/infra/otel/sdk/trace/stdout"
)

func Run() {

	// Load configuration
	config, appErr := conf.LoadConfig()
	if appErr != nil {
		slog.Error("media_exporter.main.configuration_error", slog.String("error", appErr.Error()))
		return
	}

	// slog + OTEL logging
	service := resource.NewSchemaless(
		semconv.ServiceName(model.AppServiceName),
		semconv.ServiceVersion(model.CurrentVersion),
		semconv.ServiceInstanceID(config.Consul.Id),
		semconv.ServiceNamespace(model.NamespaceName),
	)
	shutdown := logging.Setup(service)

	// Initialize the application
	application, appErr := app.New(config, shutdown)
	if appErr != nil {
		slog.Error("media_exporter.main.application_initialization_error", slog.String("error", appErr.Error()))
		return
	}

	// Initialize signal handling for graceful shutdown
	initSignals(application)

	// Log the configuration
	slog.Debug("media_exporter.main.configuration_loaded",
		slog.String("consul", config.Consul.Address),
		slog.String("grpc_address", config.Consul.Address),
		slog.String("consul_id", config.Consul.Id),
	)

	// Start the application
	slog.Info("media_exporter.main.starting_application")
	startErr := application.Start()
	if startErr != nil {
		slog.Error("media_exporter.main.application_start_error", slog.String("error", startErr.Error()))
	} else {
		slog.Info("media_exporter.main.application_started_successfully")
	}

}

func initSignals(application *app.App) {
	slog.Info("media_exporter.main.initializing_stop_signals", slog.String("main", "initializing_stop_signals"))
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch)

	go func() {
		for {
			s := <-sigch
			handleSignals(s, application)
		}
	}()
}

func handleSignals(signal os.Signal, application *app.App) {
	if signal == syscall.SIGTERM || signal == syscall.SIGINT || signal == syscall.SIGKILL {
		err := application.Stop()
		if err != nil {
			return
		}
		slog.Info(
			"media_exporter.main.received_kill_signal",
			slog.String(
				"signal",
				signal.String(),
			),
			slog.String(
				"status",
				"service gracefully stopped",
			),
		)
		os.Exit(0)
	}
}
