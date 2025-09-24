package options

import (
	"context"
	"time"

	"github.com/webitel/media-exporter/auth"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/model/options/util"
	"google.golang.org/grpc/codes"
)

type UpdateOptions struct {
	context.Context
	Time time.Time
	Auth auth.Auther
}

func NewUpdateOptions(ctx context.Context) (*UpdateOptions, error) {
	opts := &UpdateOptions{
		Context: ctx,
		Time:    time.Now().UTC(),
	}

	if err := setAuthFromContext(ctx, &opts.Auth); err != nil {
		return nil, err
	}

	return opts, nil
}

func setAuthFromContext(ctx context.Context, target *auth.Auther) error {
	if sess := util.GetAutherOutOfContext(ctx); sess != nil {
		*target = sess
		return nil
	}
	return errors.New("can't authorize user", errors.WithCode(codes.Unauthenticated))
}

// Getters
func (o *CreateOptions) RequestTime() time.Time { return o.Time }
func (o *CreateOptions) GetAuth() auth.Auther   { return o.Auth }

func (o *UpdateOptions) RequestTime() time.Time { return o.Time }
func (o *UpdateOptions) GetAuth() auth.Auther   { return o.Auth }
