// File: interceptor/validate_unary_server_interceptor.go

package interceptor

import (
	"context"
	"errors"

	"github.com/bufbuild/protovalidate-go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto" // Required for proto.Message type assertion

	cerr "github.com/webitel/media-exporter/internal/errors"
)

// ValidateUnaryServerInterceptor returns a gRPC interceptor for request validation.
func ValidateUnaryServerInterceptor(val *protovalidate.Validator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if v, ok := req.(proto.Message); ok {
			if err := validateMessage(val, v); err != nil {
				return nil, err
			}
		}
		// Proceed to api_handler if validation passes
		return handler(ctx, req)
	}
}

// ValidateStreamServerInterceptor mirrors ValidateUnaryServerInterceptor for streaming RPCs,
// validating each message as it comes in rather than a single upfront request.
func ValidateStreamServerInterceptor(val *protovalidate.Validator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &validatingServerStream{ServerStream: ss, validator: val})
	}
}

type validatingServerStream struct {
	grpc.ServerStream
	validator *protovalidate.Validator
}

func (s *validatingServerStream) RecvMsg(m any) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	if v, ok := m.(proto.Message); ok {
		return validateMessage(s.validator, v)
	}
	return nil
}

// validateMessage runs protovalidate against a single message, shared by the unary and
// streaming interceptors above.
func validateMessage(val *protovalidate.Validator, v proto.Message) error {
	if err := val.Validate(v); err != nil {
		var ve *protovalidate.ValidationError
		if errors.As(err, &ve) && len(ve.Violations) > 0 {
			violation := ve.Violations[0]
			return cerr.Internal(
				violation.GetMessage(),
				cerr.WithID(violation.GetConstraintId()),
			)
		}
		return cerr.Internal(
			err.Error(),
			cerr.WithID("unknown"),
		)
	}
	return nil
}
