package drafts

//
//import (
//	"context"
//	"fmt"
//	"io"
//	"os"
//	"path/filepath"
//	"sync"
//	"time"
//
//	"github.com/disintegration/imaging"
//	"github.com/redis/go-redis/v9"
//	pdfapi "github.com/webitel/media-exporter/api/pdf"
//	"github.com/webitel/media-exporter/api/storage"
//	"github.com/webitel/media-exporter/internal/errors"
//	"github.com/webitel/media-exporter/internal/model"
//	"github.com/webitel/media-exporter/internal/model/options"
//	"github.com/webitel/media-exporter/internal/pdf/maroto"
//	"github.com/webitel/storage/gen/engine"
//	"google.golang.org/grpc/metadata"
//)
//
//// PdfService handles media export requests.
//type PdfService struct {
//	app         *App
//	onceWorkers sync.Once
//	pdfapi.UnimplementedPdfServiceServer
//}
//
//func NewPdfService(app *App) (*PdfService, error) {
//	if app == nil {
//		return nil, errors.Internal("internal is nil")
//	}
//	return &PdfService{app: app}, nil
//}
//
//func (m *PdfService) GetPdfExportHistory(ctx context.Context, req *pdfapi.PdfHistoryRequest) (*pdfapi.PdfHistoryResponse, error) {
//	if req.AgentId == 0 {
//		return nil, fmt.Errorf("agent_id is required")
//	}
//
//	opts, err := options.NewSearchOptions(ctx)
//	if err != nil {
//		return nil, err
//	}
//
//	return m.app.Store.Pdf().GetPdfExportHistory(opts, req)
//}
//
//// GeneratePdfExport creates a new export task asynchronously and returns metadata.
//// It ensures the export history ID is set in cache before pushing the task to the queue.
//func (m *PdfService) GeneratePdfExport(ctx context.Context, req *pdfapi.PdfGenerateRequest) (*pdfapi.PdfExportMetadata, error) {
//	taskID := fmt.Sprintf("%d:%d:%s", req.AgentId, req.To, req.Channel)
//
//	//if err := m.app.cache.Clear(); err != nil {
//	//	return nil, fmt.Errorf("failed to clear cache: %w", err)
//	//}
//
//	status, err := m.app.cache.GetExportStatus(taskID)
//	if err != nil && !errors.Is(err, redis.Nil) {
//		return nil, fmt.Errorf("failed to get task status: %w", err)
//	}
//	if status == "pending" || status == "processing" {
//		return nil, fmt.Errorf("task already in progress: %s", taskID)
//	}
//
//	exists, err := m.app.cache.Exists(taskID)
//	if err != nil {
//		return nil, fmt.Errorf("failed to check task existence: %w", err)
//	}
//	if exists {
//		return nil, fmt.Errorf("task already in progress: %s", taskID)
//	}
//
//	opts, err := options.NewCreateOptions(ctx)
//	if err != nil {
//		return nil, err
//	}
//
//	headers := extractHeadersFromContext(ctx, []string{"authorization", "x-req-id", "x-webitel-access"})
//
//	history := &model.NewExportHistory{
//		Name:       fmt.Sprintf("%s.pdf", taskID),
//		FileID:     nil,
//		Mime:       "application/pdf",
//		UploadedAt: opts.Time.UnixMilli(),
//		UploadedBy: opts.Auth.GetUserId(),
//		Status:     "pending",
//		AgentID:    req.AgentId,
//	}
//	historyID, err := m.app.Store.Pdf().InsertPdfExportHistory(history)
//	if err != nil {
//		return nil, fmt.Errorf("failed to insert export history: %w", err)
//	}
//
//	if err := m.app.cache.SetExportHistoryID(taskID, historyID); err != nil {
//		return nil, fmt.Errorf("failed to set historyID in cache: %w", err)
//	}
//
//	task := model.ExportTask{
//		TaskID:  taskID,
//		UserID:  req.AgentId,
//		Channel: req.Channel,
//		From:    req.From,
//		To:      req.To,
//		Headers: headers,
//		IDs:     req.FileIds,
//	}
//
//	if err := m.app.cache.PushExportTask(task); err != nil {
//		return nil, fmt.Errorf("failed to push task to queue: %w", err)
//	}
//	if err := m.app.cache.SetExportStatus(taskID, "pending"); err != nil {
//		return nil, fmt.Errorf("failed to set task status: %w", err)
//	}
//
//	m.onceWorkers.Do(func() {
//		go StartExportWorker(context.Background(), opts, 4, m.app)
//	})
//
//	fileName := fmt.Sprintf("%s.pdf", taskID)
//	status, _ = m.app.cache.GetExportStatus(taskID)
//
//	return &pdfapi.PdfExportMetadata{
//		TaskId:   taskID,
//		FileName: fileName,
//		MimeType: "application/pdf",
//		Status:   status,
//		Size:     0,
//	}, nil
//}
//
//func (m *PdfService) DownloadPdfExport(req *pdfapi.PdfDownloadRequest, stream pdfapi.PdfService_DownloadPdfExportServer) error {
//
//	s, err := m.app.storageClient.DownloadFile(stream.Context(), &storage.DownloadFileRequest{
//		Id:       req.GetExportId(),
//		DomainId: req.GetDomainId(),
//	})
//	if err != nil {
//		return fmt.Errorf("failed to init download stream: %w", err)
//	}
//
//	for {
//		chunk, err := s.Recv()
//		if err == io.EOF {
//			break
//		}
//		if err != nil {
//			return fmt.Errorf("download stream error: %w", err)
//		}
//
//		if err := stream.Send(&pdfapi.PdfExportChunk{Data: chunk.GetChunk()}); err != nil {
//			return fmt.Errorf("failed to send chunk: %w", err)
//		}
//	}
//
//	return nil
//}
//
//// helper: take keys from incoming metadata and return a simple map[string]string
//func extractHeadersFromContext(ctx context.Context, keys []string) map[string]string {
//	out := map[string]string{}
//	if md, ok := metadata.FromIncomingContext(ctx); ok {
//		for _, k := range keys {
//			if vals := md.Get(k); len(vals) > 0 {
//				out[k] = vals[0]
//			}
//		}
//	}
//	return out
//}
//
//// StartExportWorker runs background workers to process export queue.
//// workerCtx controls worker lifecycle (use a server-level context on shutdown).
//func StartExportWorker(workerCtx context.Context, opts *options.CreateOptions, n int, app *App) {
//	for i := 0; i < n; i++ {
//		go func(workerID int) {
//			for {
//				// Respect shutdown from workerCtx
//				select {
//				case <-workerCtx.Done():
//					return
//				default:
//				}
//
//				// Pop tasks in the loop (pop should return meaningful error when empty)
//				task, err := app.cache.PopExportTask()
//				if err != nil {
//					time.Sleep(time.Second)
//					continue
//				}
//
//				_ = app.cache.SetExportStatus(task.TaskID, "processing")
//				// Rebuild a context for storage calls using headers saved in task
//				ctxWithHeaders := contextWithHeaders(task.Headers)
//				if err := processExportTask(ctxWithHeaders, opts, app, task); err != nil {
//					_ = app.cache.SetExportStatus(task.TaskID, "failed")
//				} else {
//					_ = app.cache.SetExportStatus(task.TaskID, "done")
//				}
//			}
//		}(i)
//	}
//}
//
//// build a new context.Background() with outgoing metadata created from headers map
//func contextWithHeaders(headers map[string]string) context.Context {
//	ctx := context.Background()
//	if len(headers) == 0 {
//		return ctx
//	}
//	// convert headers map to key/value pairs for metadata.Pairs
//	pairs := make([]string, 0, len(headers)*2)
//	for k, v := range headers {
//		pairs = append(pairs, k, v)
//	}
//	md := metadata.Pairs(pairs...)
//	return metadata.NewOutgoingContext(ctx, md)
//}
//
//func processExportTask(ctx context.Context, opts *options.CreateOptions, app *App, task model.ExportTask) error {
//
//	historyID, err := app.cache.GetExportHistoryID(task.TaskID)
//	if err != nil {
//		_ = app.cache.ClearExportTask(task.TaskID)
//		return fmt.Errorf("historyID missing for task: %s", task.TaskID)
//	}
//
//	_ = app.cache.SetExportStatus(task.TaskID, "processing")
//	_ = app.Store.Pdf().UpdatePdfExportStatus(&model.UpdateExportStatus{
//		ID:        historyID,
//		Status:    "processing",
//		UpdatedBy: opts.Auth.GetUserId(),
//		FileID:    nil,
//	})
//
//	enumChannel, err := parseChannel(task.Channel)
//	if err != nil {
//		_ = app.cache.ClearExportTask(task.TaskID)
//		return err
//	}
//
//	filesResp, err := app.storageClient.SearchScreenRecordings(ctx, &storage.SearchScreenRecordingsRequest{
//		Id:      task.IDs,
//		Channel: enumChannel,
//		UserId:  task.UserID,
//		UploadedAt: &engine.FilterBetween{
//			From: task.From,
//			To:   task.To,
//		},
//	})
//	if err != nil {
//		_ = app.Store.Pdf().UpdatePdfExportStatus(&model.UpdateExportStatus{
//			ID:        historyID,
//			Status:    "failed",
//			UpdatedBy: opts.Auth.GetUserId(),
//			FileID:    nil,
//		})
//
//		_ = app.cache.ClearExportTask(task.TaskID)
//		return err
//	}
//
//	tmpFiles, fileInfos, err := downloadFilesWithPool(ctx, opts, app, filesResp.Items)
//	if err != nil {
//		_ = app.cache.ClearExportTask(task.TaskID)
//		return err
//	}
//	defer cleanupFiles(tmpFiles)
//
//	pdfBytes, err := maroto.GeneratePDF(tmpFiles, fileInfos)
//	if err != nil {
//		_ = app.cache.ClearExportTask(task.TaskID)
//		return err
//	}
//
//	fileName := fmt.Sprintf("%s.pdf", task.TaskID)
//	if err := SavePDFToFile(pdfBytes, fileName); err != nil {
//		_ = app.cache.ClearExportTask(task.TaskID)
//		return err
//	}
//
//	res, err := uploadPDFToStorage(ctx, opts, app, fileName, task)
//	if err != nil {
//		_ = app.Store.Pdf().UpdatePdfExportStatus(&model.UpdateExportStatus{
//			ID:        historyID,
//			Status:    "failed",
//			UpdatedBy: opts.Auth.GetUserId(),
//			FileID:    nil,
//		})
//		_ = app.cache.SetExportStatus(task.TaskID, "failed")
//		_ = app.cache.ClearExportTask(task.TaskID)
//		return err
//	}
//
//	historyUpdate := &model.UpdateExportStatus{
//		ID:        historyID,
//		FileID:    &res.FileId,
//		Status:    "done",
//		UpdatedBy: opts.Auth.GetUserId(),
//	}
//	_ = app.Store.Pdf().UpdatePdfExportStatus(historyUpdate)
//
//	_ = app.cache.SetExportStatus(task.TaskID, "done")
//	_ = app.cache.SetExportURL(task.TaskID, fileName)
//	_ = app.cache.ClearExportTask(task.TaskID)
//
//	return nil
//}
//
//func uploadPDFToStorage(ctx context.Context, opts *options.CreateOptions, app *App, filePath string, task model.ExportTask) (*storage.UploadFileResponse, error) {
//	f, err := os.Open(filePath)
//	if err != nil {
//		return nil, fmt.Errorf("open file failed: %w", err)
//	}
//	defer func(f *os.File) {
//		err := f.Close()
//		if err != nil {
//			fmt.Printf("[uploadPDFToStorage] Error closing file: %v\n", err)
//		}
//	}(f)
//
//	chEnum, err := parseChannel(task.Channel)
//	if err != nil {
//		return nil, err
//	}
//
//	stream, err := app.storageClient.UploadFile(ctx)
//	if err != nil {
//		return nil, fmt.Errorf("UploadFile init failed: %w", err)
//	}
//
//	metadataMsg := &storage.UploadFileRequest{
//		Data: &storage.UploadFileRequest_Metadata_{
//			Metadata: &storage.UploadFileRequest_Metadata{
//				Name:           fmt.Sprintf("%s.pdf", task.TaskID),
//				MimeType:       "application/pdf",
//				Uuid:           task.TaskID,
//				StreamResponse: true,
//				Channel:        chEnum,
//				UploadedBy:     opts.Auth.GetUserId(),
//				DomainId:       opts.Auth.GetDomainId(),
//				CreatedAt:      time.Now().UnixMilli(),
//			},
//		},
//	}
//	if err := stream.Send(metadataMsg); err != nil {
//		return nil, fmt.Errorf("failed to send metadata: %w", err)
//	}
//
//	buf := make([]byte, 32*1024) // 32KB
//	for {
//		n, readErr := f.Read(buf)
//		if n > 0 {
//			chunkMsg := &storage.UploadFileRequest{
//				Data: &storage.UploadFileRequest_Chunk{
//					Chunk: buf[:n],
//				},
//			}
//			if err := stream.Send(chunkMsg); err != nil {
//				return nil, fmt.Errorf("failed to send chunk: %w", err)
//			}
//		}
//		if readErr == io.EOF {
//			break
//		}
//		if readErr != nil {
//			return nil, fmt.Errorf("read file failed: %w", readErr)
//		}
//	}
//
//	resp, err := stream.CloseAndRecv()
//	if err != nil {
//		return nil, fmt.Errorf("UploadFile close failed: %w", err)
//	}
//
//	return resp, nil
//}
//
//// downloadFilesWithPool downloads multiple files concurrently
//func downloadFilesWithPool(ctx context.Context, opts *options.CreateOptions, app *App, files []*storage.File) (map[string]string, map[string]*storage.File, error) {
//	tmpFiles := make(map[string]string)
//	fileInfos := make(map[string]*storage.File)
//	var mu sync.Mutex
//
//	type job struct{ file *storage.File }
//	jobs := make(chan job, len(files))
//	results := make(chan error, len(files))
//
//	numWorkers := 4
//	for w := 0; w < numWorkers; w++ {
//		go func() {
//			for j := range jobs {
//				tmpPath, err := downloadAndResize(ctx, app.storageClient, opts.GetAuth().GetDomainId(), j.file)
//				if err == nil {
//					mu.Lock()
//					tmpFiles[fmt.Sprint(j.file.Id)] = tmpPath
//					fileInfos[fmt.Sprint(j.file.Id)] = j.file
//					mu.Unlock()
//				}
//				results <- err
//			}
//		}()
//	}
//
//	for _, f := range files {
//		jobs <- job{file: f}
//	}
//	close(jobs)
//
//	for range files {
//		if err := <-results; err != nil {
//			return nil, nil, err
//		}
//	}
//
//	return tmpFiles, fileInfos, nil
//}
//
//// cleanupFiles deletes temporary files
//func cleanupFiles(files map[string]string) {
//	for _, path := range files {
//		_ = os.Remove(path)
//	}
//}
//
//// parseChannel converts string channel to enum
//func parseChannel(channel string) (storage.UploadFileChannel, error) {
//	channels := map[string]storage.UploadFileChannel{
//		"screenshot":    storage.UploadFileChannel_ScreenshotChannel,
//		"screensharing": storage.UploadFileChannel_ScreenSharingChannel,
//	}
//	if val, ok := channels[channel]; ok {
//		return val, nil
//	}
//	return 0, fmt.Errorf("invalid channel: %s", channel)
//}
//
//// downloadAndResize downloads a file from storage and resizes it to width=400px
//func downloadAndResize(ctx context.Context, client storage.FileServiceClient, domainID int64, f *storage.File) (string, error) {
//	tmpDir := os.TempDir()
//	ext := getFileExt(f.MimeType)
//	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%d_%s%s", f.Id, f.Name, ext))
//
//	if f.Id == 0 || f.Name == "" {
//		return "", fmt.Errorf("invalid file: id=%d, name='%s'", f.Id, f.Name)
//	}
//
//	out, err := os.Create(tmpPath)
//	if err != nil {
//		return "", err
//	}
//	defer func(out *os.File) {
//		err := out.Close()
//		if err != nil {
//			fmt.Printf("[downloadAndResize] Error closing file: %v\n", err)
//		}
//	}(out)
//
//	stream, err := client.DownloadFile(ctx, &storage.DownloadFileRequest{
//		Id:       f.Id,
//		DomainId: domainID,
//	})
//	if err != nil {
//		return "", err
//	}
//
//	for {
//		chunk, err := stream.Recv()
//		if err == io.EOF {
//			break
//		}
//		if err != nil {
//			return "", err
//		}
//		_, err = out.Write(chunk.GetChunk())
//		if err != nil {
//			return "", err
//		}
//	}
//
//	img, err := imaging.Open(tmpPath)
//	if err != nil {
//		return tmpPath, nil
//	}
//
//	resized := imaging.Resize(img, 400, 0, imaging.Lanczos)
//	_ = imaging.Save(resized, tmpPath)
//
//	return tmpPath, nil
//}
//
//func getFileExt(mime string) string {
//	switch mime {
//	case "image/png":
//		return ".png"
//	case "image/jpeg":
//		return ".jpg"
//	case "image/gif":
//		return ".gif"
//	default:
//		return ".jpg"
//	}
//}
//
//func SavePDFToFile(pdfBytes []byte, filename string) error {
//	return os.WriteFile(filename, pdfBytes, 0644)
//}
