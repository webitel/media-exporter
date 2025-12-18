package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	pdfapi "github.com/webitel/media-exporter/api/pdf"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/model"
	"github.com/webitel/media-exporter/internal/model/options"
)

// PdfService handles PDF export requests (gRPC endpoints).
type PdfService struct {
	app *App
	pdfapi.UnimplementedPdfServiceServer
}

//var _ pdfapi.PdfServiceServer = (*PdfService)(nil)

func NewPdfService(app *App) (*PdfService, error) {
	if app == nil {
		return nil, errors.Internal("app is nil")
	}
	return &PdfService{app: app}, nil
}

// --- gRPC Methods ---

func (s *PdfService) GenerateScreenrecordingPdfExport(ctx context.Context, req *pdfapi.ScreenrecordingPdfGenerateRequest) (*pdfapi.PdfExportMetadata, error) {
	if req.AgentId == 0 {
		return nil, fmt.Errorf("agent_id is required")
	}
	return s.generatePdfTask(ctx, req.AgentId, "", pdfapi.PdfChannel_SCREENRECORDING, req.From, req.To, req.FileIds)
}

func (s *PdfService) GenerateCallPdfExport(ctx context.Context, req *pdfapi.PdfGenerateCallPdfRequest) (*pdfapi.PdfExportMetadata, error) {
	if req.CallId == "" {
		return nil, fmt.Errorf("call_id is required")
	}
	return s.generatePdfTask(ctx, 0, req.CallId, pdfapi.PdfChannel_CALL, 0, 0, nil)
}

func (s *PdfService) GetScreenrecordingPdfExportHistory(ctx context.Context, req *pdfapi.PdfScreenrecordingHistoryRequest) (*pdfapi.PdfHistoryResponse, error) {
	if req.AgentId == 0 {
		return nil, fmt.Errorf("agent_id is required")
	}
	opts, err := options.NewSearchOptions(ctx)
	if err != nil {
		return nil, err
	}
	return s.app.Store.Pdf().GetScreenrecordingPdfExportHistory(opts, req)
}

func (s *PdfService) GetCallPdfHistory(ctx context.Context, req *pdfapi.PdfCallPdfHistoryRequest) (*pdfapi.PdfHistoryResponse, error) {
	if req.CallId == "" {
		return nil, fmt.Errorf("call_id is required")
	}
	opts, err := options.NewSearchOptions(ctx)
	if err != nil {
		return nil, err
	}
	return s.app.Store.Pdf().GetCallPdfExportHistory(opts, req)
}

func (s *PdfService) DownloadPdfExport(req *pdfapi.PdfDownloadRequest, stream pdfapi.PdfService_DownloadPdfExportServer) error {
	return streamDownloadFile(stream.Context(), s.app.storageClient, req, stream)
}

func (s *PdfService) DeletePdfExportRecord(ctx context.Context, req *pdfapi.DeletePdfExportRecordRequest) (*pdfapi.DeletePdfExportRecordResponse, error) {
	if req.Id == 0 {
		return nil, fmt.Errorf("id is required for delete operation")
	}
	opts, err := options.NewDeleteOptions(ctx, []int64{req.Id})
	if err != nil {
		return nil, err
	}
	err = s.app.Store.Pdf().DeletePdfExportRecord(opts, req)
	if err != nil {
		return nil, err
	}
	return &pdfapi.DeletePdfExportRecordResponse{Id: req.Id}, nil
}

// --- Internal Helper Logic ---

func (s *PdfService) generatePdfTask(ctx context.Context, agentID int64, callID string, channel pdfapi.PdfChannel, from, to int64, fileIDs []int64) (*pdfapi.PdfExportMetadata, error) {
	opts, err := options.NewCreateOptions(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var channelStr string
	var prefix string

	switch channel {
	case pdfapi.PdfChannel_CALL:
		channelStr, prefix = "call", "vc"
	case pdfapi.PdfChannel_SCREENRECORDING:
		channelStr, prefix = "screenrecording", "ss"
	default:
		channelStr, prefix = "unknown", "unknown"
	}

	subjectID := fmt.Sprintf("%d", agentID)
	if callID != "" {
		subjectID = callID
	}

	taskID := fmt.Sprintf("pdf_%s_%s_%04d-%02d-%02d_%02d_%02d_%02d",
		prefix, subjectID,
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(),
	)

	slog.InfoContext(ctx, "Generating PDF export task", "taskID", taskID, "agentID", agentID, "callID", callID)

	status, err := s.app.cache.GetExportStatus(taskID)
	if err == nil && (status == "pending" || status == "processing") {
		return nil, fmt.Errorf("task already in progress: %s", taskID)
	}
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to check task status: %w", err)
	}

	history := &model.NewExportHistory{
		Name:       fmt.Sprintf("%s.pdf", taskID),
		Mime:       "application/pdf",
		UploadedAt: opts.Time.UnixMilli(),
		UploadedBy: opts.Auth.GetUserId(),
		Status:     "pending",
		AgentID:    agentID,
		CallID:     callID,
	}

	historyID, err := s.app.Store.Pdf().InsertPdfExportHistory(opts, history)
	if err != nil {
		return nil, fmt.Errorf("failed to insert history: %w", err)
	}

	if err := s.app.cache.SetExportHistoryID(taskID, historyID); err != nil {
		return nil, fmt.Errorf("failed to cache history ID: %w", err)
	}

	task := model.ExportTask{
		TaskID:   taskID,
		AgentID:  agentID,
		CallID:   callID,
		UserID:   opts.Auth.GetUserId(),
		DomainID: opts.Auth.GetDomainId(),
		Channel:  channelStr,
		From:     from,
		To:       to,
		IDs:      fileIDs,
		Headers:  extractHeadersFromContext(ctx, []string{"authorization", "x-req-id", "x-webitel-access"}),
		Type:     PdfExportType,
	}

	if err := s.app.cache.PushExportTask(task); err != nil {
		return nil, fmt.Errorf("failed to push task to queue: %w", err)
	}

	if err := s.app.cache.SetExportStatus(taskID, "pending"); err != nil {
		return nil, fmt.Errorf("failed to set task status in cache: %w", err)
	}

	return &pdfapi.PdfExportMetadata{
		TaskId:   taskID,
		FileName: history.Name,
		MimeType: history.Mime,
		Status:   "pending",
	}, nil
}
