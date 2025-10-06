package app

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/webitel/media-exporter/internal/model"
)

const (
	PdfExportType       = "pdf"
	ZipExportType       = "zip"
	authorizationHeader = "x-webitel-access"
)

// StartExportWorker launches background workers to process export tasks concurrently.
// If too many workers are configured, the number is automatically limited based on available CPU cores.
func (app *App) StartExportWorker(ctx context.Context) {
	numWorkers := app.config.Export.Workers
	if numWorkers <= 0 {
		numWorkers = 4 // fallback by default
	}

	maxWorkers := runtime.NumCPU() * 2
	if numWorkers > maxWorkers {
		numWorkers = maxWorkers
	}

	for i := 0; i < numWorkers; i++ {
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

					session, err := model.NewSession(task.UserID, task.DomainID, task.Headers[authorizationHeader])
					if err != nil {
						_ = app.cache.ClearExportTask(task.TaskID)
						continue
					}

					switch task.Type {
					case PdfExportType:
						if err := handlePdfTask(ctx, session, app, task); err != nil {
							_ = app.cache.ClearExportTask(task.TaskID)
						}
					case ZipExportType:
						panic("not implemented")
					//	if err := handleZipTask(ctx, session, app, task); err != nil {
					//		_ = app.cache.ClearExportTask(task.TaskID)
					default:
						fmt.Printf("[WORKER %d] unknown export type: %s\n", workerID, task.Type)
					}
				}
			}
		}(i + 1)
	}
}
