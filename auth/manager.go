package auth

import (
	"context"
)

type Manager interface {
	AuthorizeFromContext(ctx context.Context) (Auther, error)
}
