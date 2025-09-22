package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Ping Redis to check the connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("cannot connect to Redis at %s: %w", addr, err)
	}

	return &RedisCache{client: rdb}, nil
}

func (r *RedisCache) SetRequest(from, to, channel string, ttl time.Duration) error {
	key := requestKey(from, to, channel)
	return r.client.Set(context.Background(), key, "1", ttl).Err()
}

func (r *RedisCache) Exists(from, to, channel string) (bool, error) {
	key := requestKey(from, to, channel)
	count, err := r.client.Exists(context.Background(), key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *RedisCache) Delete(from, to, channel string) error {
	key := requestKey(from, to, channel)
	return r.client.Del(context.Background(), key).Err()
}

// helper to standardize keys
func requestKey(from, to, channel string) string {
	return fmt.Sprintf("req:%s:%s:%s", from, to, channel)
}
