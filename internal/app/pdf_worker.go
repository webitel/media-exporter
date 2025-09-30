package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/webitel/media-exporter/internal/model/options"
)

// StartExportWorkers launches background workers to process tasks from queue.
func StartExportWorkers(ctx context.Context, opts *options.CreateOptions, n int, app *App) {
	for i := 0; i < n; i++ {
		go func(workerID int) {

			for {
				select {
				case <-ctx.Done():
					return
				default:
					task, err := app.cache.PopExportTask()
					if err != nil {
						time.Sleep(time.Second)
						continue
					}

					start := time.Now()
					if err := handleTask(ctx, opts, app, task); err != nil {
						slog.ErrorContext(ctx, fmt.Sprintf("[Worker %d] handle task err: %v", workerID, err))
					} else {
						slog.InfoContext(ctx, fmt.Sprintf("[Worker %d] completed task %s in %v", workerID, task, time.Since(start)))
					}
				}
			}
		}(i)
	}
}
