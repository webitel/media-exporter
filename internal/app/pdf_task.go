package app

import (
	"context"
	"fmt"

	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/model"
	"github.com/webitel/media-exporter/internal/model/options"
	"github.com/webitel/media-exporter/internal/pdf/maroto"
	"github.com/webitel/storage/gen/engine"
	"google.golang.org/grpc/metadata"
)

func handleTask(ctx context.Context, opts *options.CreateOptions, app *App, task model.ExportTask) error {
	historyID, err := app.cache.GetExportHistoryID(task.TaskID)
	if err != nil {
		_ = app.cache.ClearExportTask(task.TaskID)
		return fmt.Errorf("historyID missing for task %s", task.TaskID)
	}

	_ = setTaskStatus(app, historyID, task.TaskID, "processing", opts.Auth.GetUserId(), nil)

	enumChannel, err := parseChannel(task.Channel)
	if err != nil {
		_ = app.cache.ClearExportTask(task.TaskID)
		return err
	}

	ctx = contextWithHeaders(task.Headers)

	filesResp, err := app.storageClient.SearchScreenRecordings(ctx, &storage.SearchScreenRecordingsRequest{
		Id:      task.IDs,
		Channel: enumChannel,
		UserId:  opts.Auth.GetUserId(),
		UploadedAt: &engine.FilterBetween{
			From: task.From,
			To:   task.To,
		},
	})
	if err != nil {
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", opts.Auth.GetUserId(), nil)
		return err
	}

	tmpFiles, fileInfos, err := downloadFilesWithPool(ctx, opts, app, filesResp.Items)
	if err != nil {
		_ = app.cache.ClearExportTask(task.TaskID)
		return err
	}
	defer cleanupFiles(tmpFiles)

	// генеруємо PDF
	pdfBytes, err := maroto.GeneratePDF(tmpFiles, fileInfos)
	if err != nil {
		_ = app.cache.ClearExportTask(task.TaskID)
		return err
	}
	fileName := fmt.Sprintf("%s.pdf", task.TaskID)
	if err := SavePDFToFile(pdfBytes, fileName); err != nil {
		_ = app.cache.ClearExportTask(task.TaskID)
		return err
	}

	res, err := uploadPDFToStorage(ctx, opts, app, fileName, task)
	if err != nil {
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", opts.Auth.GetUserId(), nil)
		return err
	}

	_ = setTaskStatus(app, historyID, task.TaskID, "done", opts.Auth.GetUserId(), &res.FileId)
	_ = app.cache.ClearExportTask(task.TaskID)
	return nil
}

// build a new context.Background() with outgoing metadata created from headers map
func contextWithHeaders(headers map[string]string) context.Context {
	ctx := context.Background()
	if len(headers) == 0 {
		return ctx
	}
	// convert headers map to key/value pairs for metadata.Pairs
	pairs := make([]string, 0, len(headers)*2)
	for k, v := range headers {
		pairs = append(pairs, k, v)
	}
	md := metadata.Pairs(pairs...)
	return metadata.NewOutgoingContext(ctx, md)
}
