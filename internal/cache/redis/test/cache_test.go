package test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	cache "github.com/webitel/media-exporter/internal/cache/redis"
	"github.com/webitel/media-exporter/internal/model"
)

const (
	testRedisAddr     = "localhost:6379"
	testRedisPassword = ""
	testRedisDB       = 0
)

func getTestCache(t *testing.T) *cache.RedisCache {
	c, err := cache.NewRedisCache(testRedisAddr, testRedisPassword, testRedisDB)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v. Ensure Redis is running locally.", err)
	}

	if err := c.Clear(); err != nil {
		t.Fatalf("Failed to clear Redis DB: %v", err)
	}
	return c
}

func TestConcurrentPushPop(t *testing.T) {
	c := getTestCache(t)
	totalTasks := 1000
	numWorkers := 10

	var wg sync.WaitGroup
	processedTasks := make(chan model.ExportTask, totalTasks)
	errors := make(chan error, totalTasks)

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for {
				task, err := c.PopExportTask()
				if err != nil {

					if err.Error() == "queue empty (timeout)" {
						return
					}

					errors <- fmt.Errorf("worker %d PopExportTask failed: %w", workerID, err)
					return
				}
				processedTasks <- task
			}
		}(i)
	}

	tasksToPush := make([]model.ExportTask, totalTasks)
	for i := 0; i < totalTasks; i++ {
		tasksToPush[i] = model.ExportTask{
			TaskID:  fmt.Sprintf("test:%d", i),
			AgentID: int64(i),
		}
		if err := c.PushExportTask(tasksToPush[i]); err != nil {
			t.Fatalf("Failed to push task %d: %v", i, err)
		}
	}

	collectedTasks := 0

	timeout := time.After(10 * time.Second)

	for collectedTasks < totalTasks {
		select {
		case task := <-processedTasks:
			assert.Contains(t, task.TaskID, "test:", "Processed task has incorrect TaskID format")
			collectedTasks++
		case err := <-errors:
			t.Errorf("Error during processing: %v", err)
			return
		case <-timeout:
			t.Fatalf("Timeout waiting for all tasks to be processed. Processed: %d, Expected: %d", collectedTasks, totalTasks)
			return
		}
	}

	wg.Wait()

	assert.Equal(t, totalTasks, collectedTasks, "Not all tasks were processed")
	t.Logf("Successfully processed %d tasks with %d workers", collectedTasks, numWorkers)
}
