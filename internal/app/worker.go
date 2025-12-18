package app

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/webitel/media-exporter/internal/domain/model"
)

const (
	PdfExportType       = "pdf"
	ZipExportType       = "zip"
	authorizationHeader = "x-webitel-access"
)

// StartExportWorker launches background workers to process export tasks concurrently.
// If too many workers are configured, the number is automatically limited based on available CPU cores.
func (app *App) StartExportWorker(ctx context.Context) {
	numWorkers := app.Config.Export.Workers
	if numWorkers <= 0 {
		numWorkers = 4
	}

	maxWorkers := runtime.NumCPU() * 2
	if numWorkers > maxWorkers {
		numWorkers = maxWorkers
	}

	slog.InfoContext(ctx, "starting export workers", "count", numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					task, err := app.Cache.PopExportTask()
					if err != nil {
						time.Sleep(time.Second)
						continue
					}

					session, err := model.NewSession(task.UserID, task.DomainID, task.Headers[authorizationHeader])
					if err != nil {

						_ = app.Cache.ClearExportTask(task.TaskID)
						continue
					}

					switch task.Type {

					case PdfExportType:
						if err := app.HandlePdfTask(ctx, session, task); err != nil {
							slog.ErrorContext(ctx, "PDF task failed", "taskID", task.TaskID, "error", err)
							_ = app.Cache.ClearExportTask(task.TaskID)
						}
					case ZipExportType:
						panic("not implemented")
					default:
						slog.WarnContext(ctx, "unknown export type",
							"workerID", workerID,
							"type", task.Type,
							"taskID", task.TaskID)
					}
				}
			}
		}(i + 1)
	}
}
