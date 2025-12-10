package options

import (
	"context"
	"time"

	"github.com/webitel/media-exporter/auth"
)

type DeleteOptions struct {
	context.Context
	Time time.Time
	Auth auth.Auther
	IDs  []int64
}

func NewDeleteOptions(ctx context.Context, ids []int64) (*DeleteOptions, error) {
	opts := &DeleteOptions{
		Context: ctx,
		Time:    time.Now().UTC(),
		IDs:     ids,
	}

	if err := setAuthFromContext(ctx, &opts.Auth); err != nil {
		return nil, err
	}

	return opts, nil
}
