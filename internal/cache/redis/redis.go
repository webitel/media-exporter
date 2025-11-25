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
	statusPrefix   = "export_status:"
	historyPrefix  = "export_history_id:"
	urlPrefix      = "export_url:"
	taskPrefix     = "export:task:"
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

// ----------------------- Task Queue -----------------------

func (r *RedisCache) PushExportTask(task model.ExportTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	if err := r.client.RPush(context.Background(), exportQueueKey, data).Err(); err != nil {
		return fmt.Errorf("failed to push task to queue: %w", err)
	}
	return nil
}

// cache/redis.go

func (r *RedisCache) PopExportTask() (model.ExportTask, error) {

	result, err := r.client.BRPop(context.Background(), 5*time.Second, exportQueueKey).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {

			return model.ExportTask{}, fmt.Errorf("queue empty (timeout)")
		}
		return model.ExportTask{}, err
	}

	if len(result) < 2 {
		return model.ExportTask{}, fmt.Errorf("unexpected BRPop result format")
	}

	data := []byte(result[1])

	var task model.ExportTask
	if err := json.Unmarshal(data, &task); err != nil {
		return model.ExportTask{}, fmt.Errorf("failed to unmarshal task: %w", err)
	}
	return task, nil
}

// ----------------------- Status -----------------------

func (r *RedisCache) Exists(taskID string) (bool, error) {
	ctx := context.Background()
	key := statusPrefix + taskID
	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *RedisCache) SetExportStatus(taskID, status string) error {
	key := statusPrefix + taskID
	return r.client.Set(context.Background(), key, status, 24*time.Hour).Err()
}

func (r *RedisCache) GetExportStatus(taskID string) (string, error) {
	key := statusPrefix + taskID
	val, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// ----------------------- History -----------------------

func (r *RedisCache) SetExportHistoryID(taskID string, historyID int64) error {
	key := historyPrefix + taskID
	return r.client.Set(context.Background(), key, historyID, 24*time.Hour).Err()
}

func (r *RedisCache) GetExportHistoryID(taskID string) (int64, error) {
	key := historyPrefix + taskID
	val, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
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

// ----------------------- URL -----------------------

func (r *RedisCache) SetExportURL(taskID, url string) error {
	key := urlPrefix + taskID
	return r.client.Set(context.Background(), key, url, 24*time.Hour).Err()
}

func (r *RedisCache) GetExportURL(taskID string) (string, error) {
	key := urlPrefix + taskID
	val, err := r.client.Get(context.Background(), key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// ----------------------- Clear Task -----------------------

func (r *RedisCache) ClearExportTask(taskID string) error {
	ctx := context.Background()
	keys := []string{
		statusPrefix + taskID,
		historyPrefix + taskID,
		urlPrefix + taskID,
		taskPrefix + taskID,
	}

	for _, key := range keys {
		if err := r.client.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("failed to delete key %s: %w", key, err)
		}
	}

	return nil
}

// ----------------------- Debug -----------------------

// ListExportQueue returns all tasks in the queue (debug only)
func (r *RedisCache) ListExportQueue() ([]model.ExportTask, error) {
	items, err := r.client.LRange(context.Background(), exportQueueKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	var tasks []model.ExportTask
	for _, item := range items {
		var t model.ExportTask
		if err := json.Unmarshal([]byte(item), &t); err != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (r *RedisCache) Clear() error {
	ctx := context.Background()
	if err := r.client.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("failed to clear redis: %w", err)
	}
	return nil
}

// ListAllStatuses returns all keys and their status (debug only)
func (r *RedisCache) ListAllStatuses() (map[string]string, error) {
	ctx := context.Background()
	keys, err := r.client.Keys(ctx, statusPrefix+"*").Result()
	if err != nil {
		return nil, err
	}
	res := make(map[string]string)
	for _, key := range keys {
		val, _ := r.client.Get(ctx, key).Result()
		res[key] = val
	}
	return res, nil
}
