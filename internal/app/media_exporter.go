package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	mediaexporter "github.com/webitel/media-exporter/api/media_exporter"
	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/model"
	"google.golang.org/grpc/metadata"
)

// MediaExporterService handles media export requests.
type MediaExporterService struct {
	app         *App
	onceWorkers sync.Once
	mediaexporter.UnimplementedMediaExporterServiceServer
}

func NewMediaExporterService(app *App) (*MediaExporterService, error) {
	if app == nil {
		return nil, errors.Internal("internal is nil")
	}
	fmt.Println("[MediaExporterService] Service created")
	return &MediaExporterService{app: app}, nil
}

// GeneratePDF creates a new export task asynchronously and returns metadata.
// It extracts serializable headers from the incoming context and stores them in the task.
func (m *MediaExporterService) GeneratePDF(ctx context.Context, req *mediaexporter.ExportRequest) (*mediaexporter.ExportMetadata, error) {
	taskID := fmt.Sprintf("%d:%d:%s", req.AgentId, req.To, req.Channel)
	fmt.Printf("[GeneratePDF] Received request: TaskID=%s\n", taskID)

	// Start workers once on a background context (worker lifecycle independent from request ctx)
	m.onceWorkers.Do(func() {
		fmt.Println("[GeneratePDF] Starting export workers...")
		go StartExportWorkers(context.Background(), 4, m.app)
	})

	// Extract interesting headers from request context metadata and store them (serializable)
	headers := extractHeadersFromContext(ctx, []string{"authorization", "x-request-id", "x-webitel-access"})

	// Check if task exists
	exists, err := m.app.cache.Exists(taskID)
	if err != nil {
		fmt.Printf("[GeneratePDF] Cache check error: %v\n", err)
		return nil, err
	}

	if !exists {
		task := model.ExportTask{
			TaskID:  taskID,
			UserID:  req.AgentId,
			Channel: req.Channel,
			From:    req.From,
			To:      req.To,
			Headers: headers,
		}
		fmt.Printf("[GeneratePDF] Pushing new task: %+v\n", task)
		if err := m.app.cache.PushExportTask(task); err != nil {
			return nil, err
		}
		if err := m.app.cache.SetExportStatus(taskID, "pending"); err != nil {
			return nil, err
		}
	} else {
		fmt.Printf("[GeneratePDF] Task already exists: %s\n", taskID)
	}

	// Return metadata immediately
	fileName := fmt.Sprintf("%s.pdf", taskID)
	status, _ := m.app.cache.GetExportStatus(taskID)
	return &mediaexporter.ExportMetadata{
		TaskId:   taskID,
		FileName: fileName,
		MimeType: "application/pdf",
		Status:   status,
		Size:     0,
	}, nil
}

// DownloadPDF streams generated PDF file by task_id
func (m *MediaExporterService) DownloadPDF(req *mediaexporter.DownloadRequest, stream mediaexporter.MediaExporterService_DownloadPDFServer) error {
	fmt.Printf("[DownloadPDF] Request for TaskID=%s\n", req.TaskId)

	status, _ := m.app.cache.GetExportStatus(req.TaskId)
	if status != "done" {
		return fmt.Errorf("file not ready, current status: %s", status)
	}
	fileName, err := m.app.cache.GetExportURL(req.TaskId)
	if err != nil {
		return fmt.Errorf("cannot get file path: %v", err)
	}

	fmt.Printf("[DownloadPDF] Streaming file: %s\n", fileName)

	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Printf("[DownloadPDF] Error closing file: %v\n", err)
		}
	}(file)

	const chunkSize = 32 * 1024 // 32 KB
	buf := make([]byte, chunkSize)

	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if err := stream.Send(&mediaexporter.ExportResponse{Chunk: buf[:n]}); err != nil {
				return fmt.Errorf("failed to send chunk: %w", err)
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return fmt.Errorf("file read error: %w", readErr)
			}
			break
		}
	}

	fmt.Printf("[DownloadPDF] Finished streaming TaskID=%s\n", req.TaskId)
	return nil
}

// helper: take keys from incoming metadata and return a simple map[string]string
func extractHeadersFromContext(ctx context.Context, keys []string) map[string]string {
	out := map[string]string{}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, k := range keys {
			if vals := md.Get(k); len(vals) > 0 {
				out[k] = vals[0]
			}
		}
	}
	return out
}

// StartExportWorkers runs background workers to process export queue.
// workerCtx controls worker lifecycle (use a server-level context on shutdown).
func StartExportWorkers(workerCtx context.Context, n int, app *App) {
	fmt.Printf("[StartExportWorkers] Starting %d workers\n", n)
	for i := 0; i < n; i++ {
		go func(workerID int) {
			fmt.Printf("[Worker-%d] Started\n", workerID)
			for {
				// Respect shutdown from workerCtx
				select {
				case <-workerCtx.Done():
					fmt.Printf("[Worker-%d] Stopping (ctx.Done)\n", workerID)
					return
				default:
				}

				// Pop tasks in the loop (pop should return meaningful error when empty)
				task, err := app.cache.PopExportTask()
				if err != nil {
					// log error returned by PopExportTask, but do not quit the worker
					fmt.Printf("[Worker-%d] PopExportTask error: %v\n", workerID, err)
					time.Sleep(time.Second)
					continue
				}
				fmt.Printf("[Worker-%d] Got task from queue: %+v\n", workerID, task)

				_ = app.cache.SetExportStatus(task.TaskID, "processing")
				// Rebuild a context for storage calls using headers saved in task
				ctxWithHeaders := contextWithHeaders(task.Headers)
				if err := processExportTask(ctxWithHeaders, app, task); err != nil {
					_ = app.cache.SetExportStatus(task.TaskID, "failed")
					fmt.Printf("[Worker-%d] Task failed: %s, err=%v\n", workerID, task.TaskID, err)
				} else {
					_ = app.cache.SetExportStatus(task.TaskID, "done")
					fmt.Printf("[Worker-%d] Task done: %s\n", workerID, task.TaskID)
				}
			}
		}(i)
	}
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

// processExportTask generates PDF and saves it locally. It uses the ctx passed (which should
// contain auth metadata via metadata.NewOutgoingContext).
func processExportTask(ctx context.Context, app *App, task model.ExportTask) error {
	fmt.Printf("[processExportTask] Start task=%s user=%d\n", task.TaskID, task.UserID)

	enumChannel, err := parseChannel(task.Channel)
	if err != nil {
		return err
	}

	filesResp, err := app.storageClient.SearchScreenRecordings(ctx, &storage.SearchScreenRecordingsRequest{
		Channel: enumChannel,
		UserId:  task.UserID,
	})
	if err != nil {
		return fmt.Errorf("SearchScreenRecordings failed: %w", err)
	}

	tmpFiles, fileInfos, err := downloadFilesWithPool(ctx, app, filesResp.Items)
	if err != nil {
		return fmt.Errorf("downloadFilesWithPool failed: %w", err)
	}
	// remove only tmpFiles (images), not the generated PDF
	defer cleanupFiles(tmpFiles)

	fmt.Printf("[processExportTask] Downloaded %d files, starting PDF gen\n", len(tmpFiles))

	pdfBytes, err := generatePDF(tmpFiles, fileInfos)
	if err != nil {
		fmt.Printf("[processExportTask] generatePDF failed: %v\n", err)
		return err
	}
	fmt.Printf("[processExportTask] PDF generated in memory, size=%d bytes\n", len(pdfBytes))

	fileName := fmt.Sprintf("%s.pdf", task.TaskID)
	if err := savePDFToFile(pdfBytes, fileName); err != nil {
		fmt.Printf("[processExportTask] savePDFToFile failed: %v\n", err)
		return err
	}

	info, _ := os.Stat(fileName)
	fileSize := int64(0)
	if info != nil {
		fileSize = info.Size()
	}

	fmt.Printf("[processExportTask] PDF generated successfully\n  TaskID: %s\n  UserID: %d\n  Channel: %s\n  From: %d, To: %d\n  File: %s (%d bytes)\n",
		task.TaskID, task.UserID, task.Channel, task.From, task.To, fileName, fileSize)

	if err := app.cache.SetExportURL(task.TaskID, fileName); err != nil {
		fmt.Printf("[processExportTask] SetExportURL failed: %v\n", err)
		return err
	}

	return nil
}

// downloadFilesWithPool downloads multiple files concurrently
func downloadFilesWithPool(ctx context.Context, app *App, files []*storage.File) (map[string]string, map[string]*storage.File, error) {
	tmpFiles := make(map[string]string)
	fileInfos := make(map[string]*storage.File)
	var mu sync.Mutex

	type job struct{ file *storage.File }
	jobs := make(chan job, len(files))
	results := make(chan error, len(files))

	numWorkers := 4
	for w := 0; w < numWorkers; w++ {
		go func() {
			for j := range jobs {
				tmpPath, err := downloadAndResize(ctx, app.storageClient, 1, j.file)
				if err == nil {
					mu.Lock()
					tmpFiles[fmt.Sprint(j.file.Id)] = tmpPath
					fileInfos[fmt.Sprint(j.file.Id)] = j.file
					mu.Unlock()
				}
				results <- err
			}
		}()
	}

	for _, f := range files {
		jobs <- job{file: f}
	}
	close(jobs)

	for range files {
		if err := <-results; err != nil {
			return nil, nil, err
		}
	}

	return tmpFiles, fileInfos, nil
}

// cleanupFiles deletes temporary files
func cleanupFiles(files map[string]string) {
	for _, path := range files {
		_ = os.Remove(path)
	}
}

// parseChannel converts string channel to enum
func parseChannel(channel string) (storage.UploadFileChannel, error) {
	channels := map[string]storage.UploadFileChannel{
		"screenshot":    storage.UploadFileChannel_ScreenshotChannel,
		"screensharing": storage.UploadFileChannel_ScreenSharingChannel,
	}
	if val, ok := channels[channel]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("invalid channel: %s", channel)
}
