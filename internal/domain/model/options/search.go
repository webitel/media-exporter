package options

import (
	"context"
	"time"

	"github.com/webitel/media-exporter/auth"
)

type SearchOptions struct {
	context.Context
	Time time.Time
	Auth auth.Auther
}

func NewSearchOptions(ctx context.Context) (*SearchOptions, error) {
	opts := &SearchOptions{
		Context: ctx,
		Time:    time.Now().UTC(),
	}

	if err := setAuthFromContext(ctx, &opts.Auth); err != nil {
		return nil, err
	}

	return opts, nil
}
