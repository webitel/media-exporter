package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/webitel/media-exporter/internal/cache"
	"github.com/webitel/media-exporter/internal/domain/model/options"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/store"
)

type PdfService interface {
	// Screenrecording methods
	GenerateExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateExportRequest) (*domain.PdfExportMetadata, error)
	GetHistory(ctx context.Context, reqOpts *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error)

	// Call methods
	GenerateCallExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateCallExportRequest) (*domain.PdfExportMetadata, error)
	GetCallHistory(ctx context.Context, reqOpts *domain.CallHistoryRequestOptions) (*domain.HistoryResponse, error)

	// Common
	DeleteRecord(ctx context.Context, opts *options.DeleteOptions, recordID int64) error
}

type PdfServiceImpl struct {
	store store.PdfStore
	cache cache.Cache
	log   *slog.Logger
}

func NewPdfService(s store.PdfStore, c cache.Cache, log *slog.Logger) (PdfService, error) {
	if s == nil || c == nil {
		return nil, errors.Internal("store or cache is nil in PdfService")
	}
	return &PdfServiceImpl{store: s, cache: c, log: log}, nil
}

// --- Screenrecording Exports ---

func (s *PdfServiceImpl) GenerateExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateExportRequest) (*domain.PdfExportMetadata, error) {
	if req.AgentID == 0 {
		return nil, errors.BadRequest("agent_id is required")
	}
	// Logic moved to a helper to reuse code between Call and Screenrecording
	return s.createExportTask(ctx, opts, domain.ChannelScreenRecording, req.AgentID, "", req.FileIDs, req.From, req.To)
}

func (s *PdfServiceImpl) GetHistory(ctx context.Context, req *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error) {
	if req.AgentID == 0 {
		return nil, errors.BadRequest("agent_id is required")
	}
	return s.store.GetPdfExportHistory(req)
}

// --- Call Exports ---

func (s *PdfServiceImpl) GenerateCallExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateCallExportRequest) (*domain.PdfExportMetadata, error) {
	if req.CallID == "" {
		return nil, errors.BadRequest("call_id is required")
	}
	return s.createExportTask(ctx, opts, domain.ChannelCall, 0, req.CallID, req.FileIDs, req.From, req.To)
}

func (s *PdfServiceImpl) GetCallHistory(ctx context.Context, req *domain.CallHistoryRequestOptions) (*domain.HistoryResponse, error) {
	if req.CallID == "" {
		return nil, errors.BadRequest("call_id is required")
	}
	return s.store.GetCallPdfExportHistory(req)
}

// --- General ---

func (s *PdfServiceImpl) DeleteRecord(ctx context.Context, opts *options.DeleteOptions, recordID int64) error {
	if recordID == 0 {
		return errors.BadRequest("id is required for delete operation")
	}
	return s.store.DeletePdfExportRecord(opts, recordID)
}

// --- Internal Helper ---

func (s *PdfServiceImpl) createExportTask(
	ctx context.Context,
	opts *options.CreateOptions,
	channel domain.ExportChannel,
	agentID int64,
	callID string,
	fileIDs []int64,
	from, to int64,
) (*domain.PdfExportMetadata, error) {
	now := time.Now()

	// Generate a meaningful task identifier
	// Example: pdf_CALL_user123_2023-10-27_10_20_30
	fileName := fmt.Sprintf("pdf_%s_%d_%s.pdf",
		channel,
		opts.Auth.GetUserId(),
		now.Format("2006-01-02_15_04_05"),
	)

	taskID := fileName

	// Check if task is already running in cache
	status, err := s.cache.GetExportStatus(taskID)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to get task status: %w", err)
	}
	if status == "pending" || status == "processing" {
		return nil, errors.BadRequest(fmt.Sprintf("task already in progress: %s", taskID))
	}

	// Prepare history record for DB
	var fileID int64
	if len(fileIDs) > 0 {
		fileID = fileIDs[0]
	}

	history := &domain.NewExportHistory{
		Name:       fileName,
		Mime:       "application/pdf",
		UploadedAt: opts.Time.UnixMilli(),
		UploadedBy: opts.Auth.GetUserId(),
		Status:     "pending",
		AgentID:    agentID,
		CallID:     callID,
		FileID:     fileID,
	}

	historyID, err := s.store.InsertPdfExportHistory(opts, history)
	if err != nil {
		return nil, fmt.Errorf("insert history failed: %w", err)
	}

	if err := s.cache.SetExportHistoryID(taskID, historyID); err != nil {
		return nil, fmt.Errorf("cache set historyID failed: %w", err)
	}

	// Prepare task for Redis Queue
	task := domain.ExportTask{
		TaskID:   taskID,
		AgentID:  agentID,
		CallID:   callID,
		UserID:   opts.Auth.GetUserId(),
		DomainID: opts.Auth.GetDomainId(),
		Channel:  string(channel),
		From:     from,
		To:       to,
		Headers:  domain.ExtractHeadersFromContext(ctx, []string{"authorization", "x-req-id", "x-webitel-access"}),
		IDs:      fileIDs,
		Type:     domain.PdfExportType,
	}

	if err := s.cache.PushExportTask(task); err != nil {
		return nil, fmt.Errorf("push task failed: %w", err)
	}

	s.log.InfoContext(ctx, "PUSHED TASK TO REDIS QUEUE", "taskID", taskID, "channel", channel)

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
