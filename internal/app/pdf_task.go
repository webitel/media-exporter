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

		_ = app.cache.SetExportStatus(task.TaskID, "failed")
		return fmt.Errorf("historyID missing for task %s: %w", task.TaskID, err)
	}

	if err := setTaskStatus(app, historyID, task.TaskID, "processing", session.UserID(), nil); err != nil {
		return fmt.Errorf("failed to set processing status: %w", err)
	}

	enumChannel, err := parseChannel(task.Channel)
	if err != nil {
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("failed to parse channel: %w", err)
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
		slog.ErrorContext(ctx, "SearchScreenRecordings failed", "taskID", task.TaskID, "error", err)
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("search recordings failed: %w", err)
	}

	tmpFiles, fileInfos, err := downloadScreenshotsForPDF(ctx, session, app, filesResp.Items)
	if err != nil {
		slog.ErrorContext(ctx, "downloadScreenshotsForPDF failed", "taskID", task.TaskID, "error", err)
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("download failed: %w", err)
	}
	defer cleanupFiles(tmpFiles)

	pdfBytes, err := maroto.GeneratePDF(tmpFiles, fileInfos)
	if err != nil {
		slog.ErrorContext(ctx, "GeneratePDF failed", "taskID", task.TaskID, "error", err)
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("PDF generation failed: %w", err)
	}

	fileName := fmt.Sprintf("%s.pdf", task.TaskID)
	if err := SavePDFToFile(pdfBytes, fileName); err != nil {
		slog.ErrorContext(ctx, "SavePDFToFile failed", "taskID", task.TaskID, "error", err)
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("save PDF failed: %w", err)
	}

	res, err := uploadPDFToStorage(ctx, session, app, fileName, task)
	if err != nil {
		slog.ErrorContext(ctx, "uploadPDFToStorage failed", "taskID", task.TaskID, "error", err)
		_ = setTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("upload failed: %w", err)
	}

	if err := setTaskStatus(app, historyID, task.TaskID, "done", session.UserID(), &res.FileId); err != nil {
		slog.ErrorContext(ctx, "failed to set done status", "taskID", task.TaskID, "error", err)
		return fmt.Errorf("failed to set done status: %w", err)
	}

	_ = app.cache.ClearExportTask(task.TaskID)

	slog.InfoContext(ctx, "PDF task completed successfully", "taskID", task.TaskID, "fileID", res.FileId)

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
