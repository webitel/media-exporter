package cache

import "time"

type Cache interface {
	SetRequest(key string, ttl time.Duration) error
	Exists(key string) (bool, error)
	Delete(key string) error
}
