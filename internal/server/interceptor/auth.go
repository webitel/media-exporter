package interceptor

import (
	"context"

	"github.com/webitel/media-exporter/auth"
	"github.com/webitel/media-exporter/internal/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const (
	SessionHeader = "session"
)

// AuthUnaryServerInterceptor authenticates and authorizes unary RPCs.
func AuthUnaryServerInterceptor(authManager auth.Manager) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

		session, err := authManager.AuthorizeFromContext(ctx)
		if err != nil {
			return nil, errors.New(
				"unauthorized",
				errors.WithCause(err),
				errors.WithCode(codes.Unauthenticated),
				errors.WithID("auth.interceptor.unauthorized"),
			)
		}

		hasPermission := false
		for _, perm := range session.GetPermissions() {
			if perm == "control_agent_screen" {
				hasPermission = true
				break
			}
		}
		if !hasPermission {
			return nil, errors.New(
				"permission denied",
				errors.WithCode(codes.PermissionDenied),
				errors.WithCause(errors.New("missing required permission control_agent_screen")),
				errors.WithID("auth.interceptor.permission"),
			)
		}

		ctx = context.WithValue(ctx, SessionHeader, session)

		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}

		return resp, nil
	}
}
