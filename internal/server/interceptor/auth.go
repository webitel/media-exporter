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

		ctx = context.WithValue(ctx, SessionHeader, session)

		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}

		return resp, nil
	}
}
