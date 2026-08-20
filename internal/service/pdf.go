package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/cache"
	"github.com/webitel/media-exporter/internal/domain/model/options"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/store"
	"github.com/webitel/media-exporter/internal/util"
	"github.com/webitel/storage/gen/engine"
)

type PdfService interface {
	// Screenrecording methods
	GenerateExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateExportRequest) (*domain.PdfExportMetadata, error)
	GetHistory(ctx context.Context, reqOpts *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error)

	// Screenrecording archive (ZIP) methods.
	PrepareScreenrecordingArchiveMetadata(ctx context.Context, req *domain.DownloadScreenrecordingArchiveRequest) (*domain.ArchiveMetadata, error)
	DownloadScreenrecordingArchive(ctx context.Context, opts *options.CreateOptions, req *domain.DownloadScreenrecordingArchiveRequest, w io.Writer) error

	// Call methods
	GenerateCallExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateCallExportRequest) (*domain.PdfExportMetadata, error)
	GetCallHistory(ctx context.Context, reqOpts *domain.CallHistoryRequestOptions) (*domain.HistoryResponse, error)

	// Call archive (ZIP) methods.
	PrepareCallArchiveMetadata(ctx context.Context, req *domain.DownloadCallArchiveRequest) (*domain.ArchiveMetadata, error)
	DownloadCallArchive(ctx context.Context, opts *options.CreateOptions, req *domain.DownloadCallArchiveRequest, w io.Writer) error

	// Common
	DeleteRecord(ctx context.Context, opts *options.DeleteOptions, recordID int64) error
}

type PdfServiceImpl struct {
	store         store.PdfStore
	cache         cache.Cache
	log           *slog.Logger
	storageClient storage.FileServiceClient
}

func NewPdfService(s store.PdfStore, c cache.Cache, log *slog.Logger, storageClient storage.FileServiceClient) (PdfService, error) {
	if s == nil || c == nil || storageClient == nil {
		return nil, errors.Internal("store, cache or storageClient is nil in PdfService")
	}

	return &PdfServiceImpl{store: s, cache: c, log: log, storageClient: storageClient}, nil
}

// --- Screenrecording Exports ---

func (s *PdfServiceImpl) GenerateExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateExportRequest) (*domain.PdfExportMetadata, error) {
	if req.AgentID == 0 {
		return nil, errors.BadRequest("agent_id is required")
	}

	sort, err := domain.NormalizeScreenshotSort(req.Sort)
	if err != nil {
		return nil, errors.BadRequest(err.Error())
	}

	// Logic moved to a helper to reuse code between Call and Screenrecording
	return s.createExportTask(ctx, opts, domain.ChannelScreenRecording, req.AgentID, "", req.FileIDs, req.From, req.To, sort)
}

func (s *PdfServiceImpl) GetHistory(ctx context.Context, req *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error) {
	if req.AgentID == 0 {
		return nil, errors.BadRequest("agent_id is required")
	}

	return s.store.GetPdfExportHistory(req)
}

// --- Screenrecording Archive (ZIP) Exports ---

func (s *PdfServiceImpl) PrepareScreenrecordingArchiveMetadata(_ context.Context, req *domain.DownloadScreenrecordingArchiveRequest) (*domain.ArchiveMetadata, error) {
	if req.AgentID == 0 {
		return nil, errors.BadRequest("agent_id is required")
	}

	name := fmt.Sprintf("archive_screenrecordings_agent_%d_%s.zip", req.AgentID, time.Now().Format("2006-01-02_15_04_05"))

	return &domain.ArchiveMetadata{
		Name:     name,
		MimeType: "application/zip",
	}, nil
}

// searchScreenrecordingArchiveFiles retrieves all matching screen recording
// videos from storage, following pagination until the result set is exhausted.
func (s *PdfServiceImpl) searchScreenrecordingArchiveFiles(ctx context.Context, req *domain.DownloadScreenrecordingArchiveRequest) ([]*storage.File, error) {
	if req.AgentID == 0 {
		return nil, errors.BadRequest("agent_id is required")
	}

	const pageSize int32 = 1000

	search := &storage.SearchScreenRecordingsByAgentRequest{
		AgentId: req.AgentID,
		Size:    pageSize,
		Sort:    "+id",
		Type:    storage.ScreenrecordingType_SCREENSHARING,
		Channel: storage.ScreenrecordingChannel_SCREENRECORDING,
	}

	// Explicitly selected files take precedence over the current date filter.
	if len(req.FileIDs) > 0 {
		search.Id = req.FileIDs
	} else if req.From != 0 || req.To != 0 {
		search.UploadedAt = &engine.FilterBetween{
			From: req.From,
			To:   req.To,
		}
	}

	var files []*storage.File

	for page := int32(1); ; page++ {
		search.Page = page

		response, err := s.storageClient.SearchScreenRecordingsByAgent(ctx, search)
		if err != nil {
			return nil, fmt.Errorf("SearchScreenRecordingsByAgent failed: %w", err)
		}

		if response == nil || len(response.GetItems()) == 0 {
			break
		}

		files = append(files, response.GetItems()...)

		if !response.GetNext() {
			break
		}
	}

	if len(files) == 0 {
		return nil, errors.BadRequest("no screen recordings found")
	}

	return files, nil
}

// DownloadScreenrecordingArchive streams the selected screen recordings
// from storage directly into a ZIP archive.
func (s *PdfServiceImpl) DownloadScreenrecordingArchive(ctx context.Context, opts *options.CreateOptions, req *domain.DownloadScreenrecordingArchiveRequest, w io.Writer) error {
	if req.AgentID == 0 {
		return errors.BadRequest("agent_id is required")
	}

	ctx = util.ForwardIncomingHeaders(ctx, []string{"authorization", "x-req-id", "x-webitel-access"})

	files, err := s.searchScreenrecordingArchiveFiles(ctx, req)
	if err != nil {
		return err
	}

	domainID := opts.Auth.GetDomainId()
	entries := make([]util.ZipStreamEntry, 0, len(files))

	for _, file := range files {
		fileID := file.GetId()

		entries = append(entries, util.ZipStreamEntry{
			Name: screenrecordingArchiveEntryName(file),
			Open: func() (io.ReadCloser, error) {
				return util.NewStorageFileReader(
					ctx,
					s.storageClient,
					domainID,
					fileID,
				)
			},
		})
	}

	if err := util.StreamZipArchive(w, entries); err != nil {
		return fmt.Errorf("stream screen recording ZIP archive: %w", err)
	}

	return nil
}

// screenrecordingArchiveEntryName builds a safe and unique ZIP entry name.
func screenrecordingArchiveEntryName(file *storage.File) string {
	name := file.GetViewName()
	if name == "" {
		name = file.GetName()
	}

	// Remove directory components to prevent paths inside the archive.
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == "/" || name == ".." {
		name = "screenrecording"
	}

	if ext := path.Ext(name); ext == "" || ext == "." {
		mimeType := strings.TrimSpace(
			strings.SplitN(file.GetMimeType(), ";", 2)[0],
		)
		name += util.GetFileExt(mimeType)
	}

	return fmt.Sprintf("%d_%s", file.GetId(), name)
}

// --- Call Exports ---

func (s *PdfServiceImpl) GenerateCallExport(ctx context.Context, opts *options.CreateOptions, req *domain.GenerateCallExportRequest) (*domain.PdfExportMetadata, error) {
	if req.CallID == "" {
		return nil, errors.BadRequest("call_id is required")
	}

	return s.createExportTask(ctx, opts, domain.ChannelCall, 0, req.CallID, req.FileIDs, req.From, req.To, "")
}

func (s *PdfServiceImpl) GetCallHistory(ctx context.Context, req *domain.CallHistoryRequestOptions) (*domain.HistoryResponse, error) {
	if req.CallID == "" {
		return nil, errors.BadRequest("call_id is required")
	}

	return s.store.GetCallPdfExportHistory(req)
}

// --- Call Archive (ZIP) Exports ---

func (s *PdfServiceImpl) PrepareCallArchiveMetadata(_ context.Context, req *domain.DownloadCallArchiveRequest) (*domain.ArchiveMetadata, error) {
	if req.CallID == "" {
		return nil, errors.BadRequest("call_id is required")
	}

	name := fmt.Sprintf("archive_call_%s_%s.zip", req.CallID, time.Now().Format("2006-01-02_15_04_05"))

	return &domain.ArchiveMetadata{
		Name:     name,
		MimeType: "application/zip",
	}, nil
}

func (s *PdfServiceImpl) DownloadCallArchive(ctx context.Context, opts *options.CreateOptions, req *domain.DownloadCallArchiveRequest, w io.Writer) error {
	if req.CallID == "" {
		return errors.BadRequest("call_id is required")
	}

	fileIDs, err := s.resolveArchiveFileIDs(req.CallID, req.FileIDs)
	if err != nil {
		return err
	}

	ctx = util.ForwardIncomingHeaders(ctx, []string{"authorization", "x-req-id", "x-webitel-access"})

	filesResp, err := s.storageClient.SearchScreenRecordings(ctx, &storage.SearchScreenRecordingsRequest{
		Id:      fileIDs,
		Type:    storage.ScreenrecordingType_PDF,
		Channel: storage.ScreenrecordingChannel_CALL,
		Size:    int32(len(fileIDs)),
	})
	if err != nil {
		return fmt.Errorf("SearchScreenRecordings failed: %w", err)
	}

	if filesResp == nil || len(filesResp.GetItems()) == 0 {
		return errors.BadRequest("no files found for the given file_ids")
	}

	domainID := opts.Auth.GetDomainId()

	entries := make([]util.ZipStreamEntry, 0, len(filesResp.GetItems()))
	for _, f := range filesResp.GetItems() {
		entries = append(entries, util.ZipStreamEntry{
			Name: fmt.Sprintf("%d_%s", f.GetId(), f.GetName()),
			Open: func() (io.ReadCloser, error) {
				return util.NewStorageFileReader(ctx, s.storageClient, domainID, f.GetId())
			},
		})
	}

	if err := util.StreamZipArchive(w, entries); err != nil {
		return fmt.Errorf("stream zip archive failed: %w", err)
	}

	return nil
}

func (s *PdfServiceImpl) resolveArchiveFileIDs(callID string, fileIDs []int64) ([]int64, error) {
	if len(fileIDs) > 0 {
		return fileIDs, nil
	}

	history, err := s.store.GetCallPdfExportHistory(&domain.CallHistoryRequestOptions{
		CallID: callID,
		Size:   1000,
	})
	if err != nil {
		return nil, fmt.Errorf("GetCallPdfExportHistory failed: %w", err)
	}

	resolved := make([]int64, 0, len(history.Data))
	for _, rec := range history.Data {
		if rec.Status == "done" && rec.FileID != 0 {
			resolved = append(resolved, rec.FileID)
		}
	}

	if len(resolved) == 0 {
		return nil, errors.BadRequest("no completed PDF exports found for this call")
	}
	return resolved, nil
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
	sort string,
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
		Sort:     sort,
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
