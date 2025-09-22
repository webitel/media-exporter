package app

import (
	"log"

	mediaexporter "github.com/webitel/media-exporter/api/media_exporter"
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
	services := []serviceRegistration{
		{
			init: func(a *App) (any, error) { return NewMediaExporterService(a) },
			register: func(s *grpc.Server, svc any) {
				mediaexporter.RegisterMediaExporterServiceServer(s, svc.(mediaexporter.MediaExporterServiceServer))
			},
			name: "MediaExporter",
		},
	}

	// Initialize and register each service
	for _, service := range services {
		svc, err := service.init(appInstance)
		if err != nil {
			log.Printf("Error initializing %s service: %v", service.name, err)

			continue
		}
		service.register(grpcServer, svc)
		log.Printf("%s service registered successfully", service.name)
	}
}
