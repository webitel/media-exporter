package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/model"
)

type RedisCache struct {
	client *redis.Client
}

const (
	exportQueueKey = "export_queue"
)

func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("cannot connect to Redis at %s: %w", addr, err)
	}

	return &RedisCache{client: rdb}, nil
}

func (r *RedisCache) SetExportHistoryID(taskID string, historyID int64) error {
	key := fmt.Sprintf("export_history_id:%s", taskID)
	return r.client.Set(context.Background(), key, historyID, 24*time.Hour).Err()
}

func (r *RedisCache) GetExportHistoryID(taskID string) (int64, error) {
	key := fmt.Sprintf("export_history_id:%s", taskID)
	val, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}

	var id int64
	_, err = fmt.Sscan(val, &id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *RedisCache) Exists(taskID string) (bool, error) {
	count, err := r.client.Exists(context.Background(), taskID).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *RedisCache) PushExportTask(task model.ExportTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return r.client.RPush(context.Background(), exportQueueKey, data).Err()

}

func (r *RedisCache) PopExportTask() (model.ExportTask, error) {
	data, err := r.client.LPop(context.Background(), exportQueueKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return model.ExportTask{}, fmt.Errorf("queue empty")
		}
		return model.ExportTask{}, err
	}

	var task model.ExportTask
	if err := json.Unmarshal(data, &task); err != nil {
		return model.ExportTask{}, err
	}
	return task, nil
}

func (r *RedisCache) SetExportStatus(taskID, status string) error {
	key := fmt.Sprintf("export_status:%s", taskID)
	return r.client.Set(context.Background(), key, status, 24*time.Hour).Err()
}

func (r *RedisCache) GetExportStatus(taskID string) (string, error) {
	key := fmt.Sprintf("export_status:%s", taskID)
	return r.client.Get(context.Background(), key).Result()
}

func (r *RedisCache) SetExportURL(taskID, url string) error {
	key := fmt.Sprintf("export_url:%s", taskID)
	return r.client.Set(context.Background(), key, url, 24*time.Hour).Err()
}

func (r *RedisCache) GetExportURL(taskID string) (string, error) {
	key := fmt.Sprintf("export_url:%s", taskID)
	return r.client.Get(context.Background(), key).Result()
}

func (r *RedisCache) ClearExportTask(taskID string) error {
	ctx := context.Background()
	keys := []string{
		fmt.Sprintf("export:task:%s", taskID),
		fmt.Sprintf("export:status:%s", taskID),
		fmt.Sprintf("export:url:%s", taskID),
		fmt.Sprintf("export:history:%s", taskID),
	}

	for _, key := range keys {
		if err := r.client.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("failed to delete key %s: %w", key, err)
		}
	}

	return nil
}

func (r *RedisCache) Clear() error {
	ctx := context.Background()
	if err := r.client.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("failed to clear redis: %w", err)
	}
	return nil
}
