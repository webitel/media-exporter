package util

import (
	"context"

	"github.com/webitel/media-exporter/auth"
	"github.com/webitel/media-exporter/internal/server/interceptor"
)

func GetAutherOutOfContext(ctx context.Context) auth.Auther {
	return ctx.Value(interceptor.SessionHeader).(auth.Auther)
}
