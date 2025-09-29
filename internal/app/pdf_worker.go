package app

import (
	"context"
	"fmt"
	"time"

	"github.com/webitel/media-exporter/internal/model/options"
)

// StartExportWorkers launches background workers to process tasks from queue.
func StartExportWorkers(ctx context.Context, opts *options.CreateOptions, n int, app *App) {
	for i := 0; i < n; i++ {
		go func(workerID int) {
			fmt.Printf("[Worker %d] started\n", workerID)

			for {
				select {
				case <-ctx.Done():
					fmt.Printf("[Worker %d] shutting down\n", workerID)
					return
				default:
					task, err := app.cache.PopExportTask()
					if err != nil {
						fmt.Printf("[Worker %d] no task found: %v\n", workerID, err)
						time.Sleep(time.Second)
						continue
					}

					fmt.Printf("[Worker %d] picked task %s\n", workerID, task.TaskID)

					start := time.Now()
					if err := handleTask(ctx, opts, app, task); err != nil {
						fmt.Printf("[Worker %d] task %s failed: %v\n", workerID, task.TaskID, err)
					} else {
						fmt.Printf("[Worker %d] task %s done in %s\n", workerID, task.TaskID, time.Since(start))
					}
				}
			}
		}(i)
	}
}
