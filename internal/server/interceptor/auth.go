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

// AuthStreamServerInterceptor mirrors AuthUnaryServerInterceptor for streaming RPCs.
func AuthStreamServerInterceptor(authManager auth.Manager) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		session, err := authManager.AuthorizeFromContext(ctx)
		if err != nil {
			return errors.New(
				"unauthorized",
				errors.WithCause(err),
				errors.WithCode(codes.Unauthenticated),
				errors.WithID("auth.interceptor.unauthorized"),
			)
		}

		ctx = context.WithValue(ctx, SessionHeader, session)

		return handler(srv, &authenticatedServerStream{ServerStream: ss, ctx: ctx})
	}
}

type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
}
