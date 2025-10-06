package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/model"
)

func downloadScreenshotsForPDF(ctx context.Context, session *model.Session, app *App, files []*storage.File) (map[string]string, map[string]*storage.File, error) {
	tmpFiles := make(map[string]string)
	fileInfos := make(map[string]*storage.File)
	var mu sync.Mutex
	var wg sync.WaitGroup

	errCh := make(chan error, len(files))

	for _, f := range files {
		wg.Add(1)
		go func(f *storage.File) {
			defer wg.Done()
			tmpPath, err := downloadAndResize(ctx, app.storageClient, session.DomainID(), f)
			if err != nil {
				// FIXME commented as we receive IDs from SearchScreenRecordings which do not exist / or have been deleted
				//errCh <- err
				slog.ErrorContext(ctx, "downloadAndResize failed", "file_id", f.Id, "error", err)
				return
			}
			mu.Lock()
			tmpFiles[fmt.Sprint(f.Id)] = tmpPath
			fileInfos[fmt.Sprint(f.Id)] = f
			mu.Unlock()
		}(f)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return nil, nil, err
		}
	}

	return tmpFiles, fileInfos, nil
}

func downloadAndResize(ctx context.Context, client storage.FileServiceClient, domainID int64, f *storage.File) (string, error) {
	if f.Id == 0 || f.Name == "" {
		return "", fmt.Errorf("invalid file: id=%d, name=%q", f.Id, f.Name)
	}
	if !isValidImageMime(f.MimeType) {
		slog.ErrorContext(ctx, "invalid file: mimeType=%q", f.MimeType)
		return "", nil
	}
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("%d_%s%s", f.Id, f.Name, getFileExt(f.MimeType)))
	if err := downloadToFile(ctx, client, domainID, f.Id, tmpPath); err != nil {
		return "", err
	}
	_ = resizeImage(tmpPath, 400)
	return tmpPath, nil
}

func isValidImageMime(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/bmp":
		return true
	default:
		return false
	}
}

func cleanupFiles(files map[string]string) {
	for _, path := range files {
		_ = os.Remove(path)
	}
}

func resizeImage(path string, width int) error {
	img, err := imaging.Open(path)
	if err != nil {
		return err
	}
	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	return imaging.Save(resized, path)
}

func downloadToFile(ctx context.Context, client storage.FileServiceClient, domainID, fileID int64, tmpPath string) error {
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create tmp file: %w", err)
	}
	defer func() { _ = out.Close() }()

	stream, err := client.DownloadFile(ctx, &storage.DownloadFileRequest{
		Id:       fileID,
		DomainId: domainID,
	})
	if err != nil {
		return fmt.Errorf("init download stream: %w", err)
	}

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("recv chunk: %w", err)
		}
		if _, err := out.Write(chunk.GetChunk()); err != nil {
			return fmt.Errorf("write chunk: %w", err)
		}
	}
	return nil
}
