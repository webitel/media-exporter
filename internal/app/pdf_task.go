package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/model"
	"github.com/webitel/media-exporter/internal/pdf/maroto"
	"github.com/webitel/storage/gen/engine"
	"google.golang.org/grpc/metadata"
)

func handlePdfTask(ctx context.Context, session *model.Session, app *App, task model.ExportTask) error {

	historyID, err := app.cache.GetExportHistoryID(task.TaskID)
	if err != nil {
		_ = app.cache.ClearExportTask(task.TaskID)
		return fmt.Errorf("historyID missing for task %s", task.TaskID)
	}

	_ = setTaskStatus(app, historyID, task.TaskID, "processing", session.UserID(), nil)

	enumChannel, err := parseChannel(task.Channel)
	if err != nil {
		_ = app.cache.ClearExportTask(task.TaskID)
		return err
	}

	ctx = contextWithHeaders(task.Headers)

	filesResp, err := app.storageClient.SearchScreenRecordings(ctx, &storage.SearchScreenRecordingsRequest{
		Id:      task.IDs,
		Channel: enumChannel,
		UserId:  session.UserID(),
		UploadedAt: &engine.FilterBetween{
			From: task.From,
			To:   task.To,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "SearchScreenRecordings failed", "error", err)
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return err
	}

	tmpFiles, fileInfos, err := downloadScreenshotsForPDF(ctx, session, app, filesResp.Items)
	if err != nil {
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		_ = app.cache.ClearExportTask(task.TaskID)
		return err
	}
	defer cleanupFiles(tmpFiles)

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

	res, err := uploadPDFToStorage(ctx, session, app, fileName, task)
	if err != nil {
		slog.ErrorContext(ctx, "uploadPDFToStorage failed", "error", err)
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return err
	}

	_ = setTaskStatus(app, historyID, task.TaskID, "done", session.UserID(), &res.FileId)
	_ = app.cache.ClearExportTask(task.TaskID)

	return nil
}

// build a new context.Background() with outgoing metadata created from headers map
func contextWithHeaders(headers map[string]string) context.Context {
	ctx := context.Background()
	if len(headers) == 0 {
		return ctx
	}
	pairs := make([]string, 0, len(headers)*2)
	for k, v := range headers {
		pairs = append(pairs, k, v)
	}
	md := metadata.Pairs(pairs...)
	return metadata.NewOutgoingContext(ctx, md)
}
