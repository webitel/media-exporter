package app

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	pdfapi "github.com/webitel/media-exporter/api/pdf"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/model"
	"github.com/webitel/media-exporter/internal/model/options"
)

// PdfService handles PDF export requests (gRPC endpoints).
type PdfService struct {
	app *App
	//onceWorkers sync.Once
	pdfapi.UnimplementedPdfServiceServer
}

func NewPdfService(app *App) (*PdfService, error) {
	if app == nil {
		return nil, errors.Internal("app is nil")
	}
	return &PdfService{app: app}, nil
}

func (s *PdfService) GetPdfExportHistory(ctx context.Context, req *pdfapi.PdfHistoryRequest) (*pdfapi.PdfHistoryResponse, error) {
	if req.AgentId == 0 {
		return nil, fmt.Errorf("agent_id is required")
	}

	opts, err := options.NewSearchOptions(ctx)
	if err != nil {
		return nil, err
	}
	return s.app.Store.Pdf().GetPdfExportHistory(opts, req)
}

func (s *PdfService) GeneratePdfExport(ctx context.Context, req *pdfapi.PdfGenerateRequest) (*pdfapi.PdfExportMetadata, error) {
	taskID := fmt.Sprintf("%d:%d:%s", req.AgentId, req.To, req.Channel)

	//if err := s.app.cache.Clear(); err != nil {
	//	return nil, fmt.Errorf("failed to clear cache: %w", err)
	//}

	status, err := s.app.cache.GetExportStatus(taskID)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to get task status: %w", err)
	}
	if status == "pending" || status == "processing" {
		return nil, fmt.Errorf("task already in progress: %s", taskID)
	}
	if exists, err := s.app.cache.Exists(taskID); err != nil {
		return nil, fmt.Errorf("failed to check task existence: %w", err)
	} else if exists {
		return nil, fmt.Errorf("task already in progress: %s", taskID)
	}

	opts, err := options.NewCreateOptions(ctx)
	if err != nil {
		return nil, err
	}
	history := &model.NewExportHistory{
		Name:       fmt.Sprintf("%s.pdf", taskID),
		Mime:       "application/pdf",
		UploadedAt: opts.Time.UnixMilli(),
		UploadedBy: opts.Auth.GetUserId(),
		Status:     "pending",
		AgentID:    req.AgentId,
	}
	historyID, err := s.app.Store.Pdf().InsertPdfExportHistory(history)
	if err != nil {

		return nil, fmt.Errorf("insert history failed: %w", err)
	}
	if err := s.app.cache.SetExportHistoryID(taskID, historyID); err != nil {

		return nil, fmt.Errorf("cache set historyID failed: %w", err)
	}

	task := model.ExportTask{
		TaskID:   taskID,
		AgentID:  req.AgentId,
		UserID:   opts.Auth.GetUserId(),
		DomainID: opts.Auth.GetDomainId(),
		Channel:  req.Channel,
		From:     req.From,
		To:       req.To,
		Headers:  extractHeadersFromContext(ctx, []string{"authorization", "x-req-id", "x-webitel-access"}),
		IDs:      req.FileIds,
		// specify export type
		Type: PdfExportType,
	}

	if err := s.app.cache.PushExportTask(task); err != nil {
		return nil, fmt.Errorf("push task failed: %w", err)
	}

	if err := s.app.cache.SetExportStatus(taskID, "pending"); err != nil {
		return nil, fmt.Errorf("cache set status failed: %w", err)
	}

	//s.onceWorkers.Do(func() {
	//	fmt.Println("[PdfService] starting workers...")
	//	go StartExportWorker(context.Background(), s.app)
	//})

	return &pdfapi.PdfExportMetadata{
		TaskId:   taskID,
		FileName: history.Name,
		MimeType: history.Mime,
		Status:   "pending",
	}, nil
}

func (s *PdfService) DownloadPdfExport(req *pdfapi.PdfDownloadRequest, stream pdfapi.PdfService_DownloadPdfExportServer) error {
	return streamDownloadFile(stream.Context(), s.app.storageClient, req, stream)
}
