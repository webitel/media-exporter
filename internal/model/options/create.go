package options

import (
	"context"
	"time"

	"github.com/webitel/media-exporter/auth"
)

type CreateOptions struct {
	context.Context
	Time time.Time
	Auth auth.Auther
}

// NewCreateOptions автоматично сетить Time = now і Auth з ctx
func NewCreateOptions(ctx context.Context) (*CreateOptions, error) {
	opts := &CreateOptions{
		Context: ctx,
		Time:    time.Now().UTC(),
	}

	if err := setAuthFromContext(ctx, &opts.Auth); err != nil {
		return nil, err
	}

	return opts, nil
}
