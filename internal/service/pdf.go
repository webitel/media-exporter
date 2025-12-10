package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	pdfapi "github.com/webitel/media-exporter/api/pdf"
	"github.com/webitel/media-exporter/internal/cache"
	"github.com/webitel/media-exporter/internal/domain/model/options"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/store"
)

type PdfService interface {
	GetHistory(ctx context.Context, reqOpts *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error)
	GenerateExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateExportRequest) (*domain.PdfExportMetadata, error)
	DeleteRecord(ctx context.Context, opts *options.DeleteOptions, recordID int64) error
}

type PdfServiceImpl struct {
	store store.PdfStore
	cache cache.Cache
	log   *slog.Logger
}

func NewPdfService(s store.PdfStore, c cache.Cache, log *slog.Logger) (PdfService, error) {
	if s == nil {
		return nil, errors.Internal("store or cache is nil in PdfService")
	}
	return &PdfServiceImpl{store: s, cache: c, log: log}, nil
}

func (s *PdfServiceImpl) GetHistory(ctx context.Context, req *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error) {
	if req.AgentID == 0 {
		return nil, fmt.Errorf("agent_id is required")
	}

	return s.store.GetPdfExportHistory(req)
}

func (s *PdfServiceImpl) GenerateExport(
	ctx context.Context,
	opts *options.CreateOptions,
	req *domain.GenerateExportRequest,
) (*domain.PdfExportMetadata, error) {
	if req.AgentID == 0 {
		return nil, fmt.Errorf("agent_id is required")
	}

	now := time.Now()

	var channelStr domain.ExportChannel
	switch req.Channel {
	case int32(pdfapi.PdfChannel_CALL):
		channelStr = domain.ChannelCall
	case int32(pdfapi.PdfChannel_SCREENRECORDING):
		channelStr = domain.ChannelScreenRecording
	default:
		channelStr = domain.ChannelUnknown
	}

	fileName := fmt.Sprintf("pdf_%s_%d_%04d-%02d-%02d_%02d_%02d_%02d.pdf",
		channelStr,
		opts.Auth.GetUserId(),
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(),
	)

	taskID := fileName
	s.log.InfoContext(ctx, "GeneratePdfExport taskID", "taskID", taskID)

	status, err := s.cache.GetExportStatus(taskID)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to get task status: %w", err)
	}
	if status == "pending" || status == "processing" {
		return nil, fmt.Errorf("task already in progress: %s", taskID)
	}
	if exists, err := s.cache.Exists(taskID); err != nil {
		return nil, fmt.Errorf("failed to check task existence: %w", err)
	} else if exists {
		return nil, fmt.Errorf("task already in progress: %s", taskID)
	}

	history := &domain.NewExportHistory{
		Name:       fmt.Sprintf("%s.pdf", taskID),
		Mime:       "application/pdf",
		UploadedAt: opts.Time.UnixMilli(),
		UploadedBy: opts.Auth.GetUserId(),
		Status:     "pending",
		AgentID:    req.AgentID,
	}
	historyID, err := s.store.InsertPdfExportHistory(opts, history)
	if err != nil {
		return nil, fmt.Errorf("insert history failed: %w", err)
	}

	if err := s.cache.SetExportHistoryID(taskID, historyID); err != nil {
		return nil, fmt.Errorf("cache set historyID failed: %w", err)
	}

	task := domain.ExportTask{
		TaskID:   taskID,
		AgentID:  req.AgentID,
		UserID:   opts.Auth.GetUserId(),
		DomainID: opts.Auth.GetDomainId(),
		Channel:  string(channelStr),
		From:     req.From,
		To:       req.To,
		Headers:  domain.ExtractHeadersFromContext(ctx, []string{"authorization", "x-req-id", "x-webitel-access"}),
		IDs:      req.FileIDs,
		Type:     domain.PdfExportType,
	}

	if err := s.cache.PushExportTask(task); err != nil {
		return nil, fmt.Errorf("push task failed: %w", err)
	}

	if err := s.cache.SetExportStatus(taskID, "pending"); err != nil {
		return nil, fmt.Errorf("cache set status failed: %w", err)
	}

	return &domain.PdfExportMetadata{
		TaskID:   taskID,
		FileName: history.Name,
		MimeType: history.Mime,
		Status:   "pending",
	}, nil
}

func (s *PdfServiceImpl) DeleteRecord(ctx context.Context, opts *options.DeleteOptions, recordID int64) error {
	if recordID == 0 {
		return fmt.Errorf("id is required for delete operation")
	}

	return s.store.DeletePdfExportRecord(opts, recordID)
}
