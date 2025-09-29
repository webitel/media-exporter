package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/model/options"
)

func downloadFilesWithPool(ctx context.Context, opts *options.CreateOptions, app *App, files []*storage.File) (map[string]string, map[string]*storage.File, error) {
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
				tmpPath, err := downloadAndResize(ctx, app.storageClient, opts.Auth.GetDomainId(), j.file)
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

func downloadAndResize(ctx context.Context, client storage.FileServiceClient, domainID int64, f *storage.File) (string, error) {
	if f.Id == 0 || f.Name == "" {
		return "", fmt.Errorf("invalid file: id=%d, name=%q", f.Id, f.Name)
	}
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("%d_%s%s", f.Id, f.Name, getFileExt(f.MimeType)))
	if err := downloadToFile(ctx, client, domainID, f.Id, tmpPath); err != nil {
		return "", err
	}
	_ = resizeImage(tmpPath, 400)
	return tmpPath, nil
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
