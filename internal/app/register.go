package app

import (
	"fmt"
	"log/slog"

	mediaexporter "github.com/webitel/media-exporter/api/pdf"
	grpc2 "github.com/webitel/media-exporter/internal/handler/grpc"
	"github.com/webitel/media-exporter/internal/service"
	"google.golang.org/grpc"
)

// serviceRegistration holds information for initializing and registering a gRPC service.
type serviceRegistration struct {
	init     func(*App) (any, error)                    // Initialization function for *App
	register func(grpcServer *grpc.Server, service any) // Registration function for gRPC server
	name     string                                     // Service name for logging
}

// RegisterServices initializes and registers all necessary gRPC services.
func RegisterServices(grpcServer *grpc.Server, appInstance *App) {
	if appInstance.log == nil {
		appInstance.log = slog.Default()
	}
	log := appInstance.log

	services := []serviceRegistration{
		{
			init: func(a *App) (any, error) {
				pdfService, err := service.NewPdfService(
					a.Store.Pdf(),
					a.Cache,
					log,
				)
				if err != nil {
					return nil, fmt.Errorf("failed to init pdf s: %w", err)
				}

				pdfHandler, err := grpc2.NewPdfHandler(pdfService)
				if err != nil {
					return nil, fmt.Errorf("failed to init pdf handler: %w", err)
				}

				return pdfHandler, nil
			},

			register: func(s *grpc.Server, svc any) {
				mediaexporter.RegisterPdfServiceServer(s, svc.(mediaexporter.PdfServiceServer))
			},
			name: "Pdf",
		},
	}

	for _, s := range services {
		svc, err := s.init(appInstance)
		if err != nil {
			continue
		}
		s.register(grpcServer, svc)
		slog.Info("registered service " + s.name)
	}
}
