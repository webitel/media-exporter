package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/domain/model"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	"github.com/webitel/media-exporter/internal/util"
	"github.com/webitel/media-exporter/internal/util/pdf/maroto"
	"github.com/webitel/storage/gen/engine"
)

func (app *App) HandlePdfTask(ctx context.Context, session *model.Session, task domain.ExportTask) error {
	historyID, err := app.Cache.GetExportHistoryID(task.TaskID)
	if err != nil {

		_ = app.Cache.SetExportStatus(task.TaskID, "failed")
		return fmt.Errorf("historyID missing for task %s: %w", task.TaskID, err)
	}

	if err := SetTaskStatus(app, historyID, task.TaskID, "processing", session.UserID(), nil); err != nil {
		return fmt.Errorf("failed to set processing status: %w", err)
	}

	ctx = util.ContextWithHeaders(task.Headers)

	channel, err := ParseChannel(task.Channel)
	if err != nil {
		_ = app.Cache.SetExportStatus(task.TaskID, "failed")
		return fmt.Errorf("channel missing for task %s: %w", task.TaskID, err)
	}

	filesResp, err := app.StorageClient.SearchScreenRecordings(ctx, &storage.SearchScreenRecordingsRequest{
		Id:      task.IDs,
		Type:    storage.ScreenrecordingType_SCREENSHOT,
		Channel: channel,
		UserId:  task.UserID,
		UploadedAt: &engine.FilterBetween{
			From: task.From,
			To:   task.To,
		},
	})

	if filesResp == nil || filesResp.Items == nil || len(filesResp.Items) == 0 {
		_ = SetTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("failed to find files: %w", err)
	}

	if err != nil {
		slog.ErrorContext(ctx, "SearchScreenRecordings failed", "taskID", task.TaskID, "error", err)
		_ = SetTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("search recordings failed: %w", err)
	}

	tmpFiles, fileInfos, err := downloadScreenshotsForPDF(ctx, session, app, filesResp.Items)
	if err != nil {
		slog.ErrorContext(ctx, "downloadScreenshotsForPDF failed", "taskID", task.TaskID, "error", err)
		_ = SetTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("download failed: %w", err)
	}
	defer util.CleanupFiles(tmpFiles)

	pdfBytes, err := maroto.GeneratePDF(tmpFiles, fileInfos)
	if err != nil {
		slog.ErrorContext(ctx, "GeneratePDF failed", "taskID", task.TaskID, "error", err)
		_ = SetTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("PDF generation failed: %w", err)
	}

	now := time.Now()
	var fileName string
	switch task.Channel {
	case "call":
		fileName = fmt.Sprintf("pdf_vc_%d_%04d-%02d-%02d_%02d_%02d_%02d.pdf",
			session.UserID(),
			now.Year(), now.Month(), now.Day(),
			now.Hour(), now.Minute(), now.Second(),
		)
	case "screenrecording":
		fileName = fmt.Sprintf("pdf_ss_%d_%04d-%02d-%02d_%02d_%02d_%02d.pdf",
			session.UserID(),
			now.Year(), now.Month(), now.Day(),
			now.Hour(), now.Minute(), now.Second(),
		)
	default:
		fileName = fmt.Sprintf("pdf_unknown_%d_%04d-%02d-%02d_%02d_%02d_%02d.pdf",
			session.UserID(),
			now.Year(), now.Month(), now.Day(),
			now.Hour(), now.Minute(), now.Second(),
		)
	}

	tempFilePath := filepath.Join(app.Config.TempDir, fileName)
	if err := util.SavePDFToTemp(tempFilePath, pdfBytes); err != nil {
		slog.ErrorContext(ctx, "SavePDFToFile failed", "taskID", task.TaskID, "error", err)
		_ = SetTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("save PDF failed: %w", err)
	}

	res, err := uploadPDFToStorage(ctx, session, app, tempFilePath, task)
	if err != nil {
		slog.ErrorContext(ctx, "uploadPDFToStorage failed", "taskID", task.TaskID, "error", err)
		_ = SetTaskStatus(app, historyID, task.TaskID, "failed", session.UserID(), nil)
		return fmt.Errorf("upload failed: %w", err)
	}

	if err := SetTaskStatus(app, historyID, task.TaskID, "done", session.UserID(), &res.FileId); err != nil {
		slog.ErrorContext(ctx, "failed to set done status", "taskID", task.TaskID, "error", err)
		return fmt.Errorf("failed to set done status: %w", err)
	}

	_ = app.Cache.ClearExportTask(task.TaskID)

	slog.InfoContext(ctx, "PDF task completed successfully", "taskID", task.TaskID, "fileID", res.FileId)

	return nil
}

func ParseChannel(channel string) (storage.ScreenrecordingChannel, error) {
	switch channel {
	case "call":
		return storage.ScreenrecordingChannel_CALL, nil
	case "screenrecording":
		return storage.ScreenrecordingChannel_SCREENRECORDING, nil
	default:
		return 0, fmt.Errorf("invalid channel: %v", channel)
	}
}

func SetTaskStatus(app *App, historyID int64, taskID, status string, updatedBy int64, fileID *int64) error {
	_ = app.Cache.SetExportStatus(taskID, status)
	return app.Store.Pdf().UpdatePdfExportStatus(&domain.UpdateExportStatus{
		ID:        historyID,
		Status:    status,
		UpdatedBy: updatedBy,
		FileID:    fileID,
	})
}
