package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/webitel/media-exporter/api/storage"
)

// downloadAndResize downloads a file from storage and resizes it to width=400px
func downloadAndResize(ctx context.Context, client storage.FileServiceClient, domainID int64, f *storage.File) (string, error) {
	tmpDir := os.TempDir()
	ext := getFileExt(f.MimeType)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%d_%s%s", f.Id, f.Name, ext))

	if f.Id == 0 || f.Name == "" {
		return "", fmt.Errorf("invalid file: id=%d, name='%s'", f.Id, f.Name)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	stream, err := client.DownloadFile(ctx, &storage.DownloadFileRequest{
		Id:       f.Id,
		DomainId: domainID,
	})
	if err != nil {
		return "", err
	}

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		_, err = out.Write(chunk.GetChunk())
		if err != nil {
			return "", err
		}
	}

	img, err := imaging.Open(tmpPath)
	if err != nil {
		return tmpPath, nil
	}

	resized := imaging.Resize(img, 400, 0, imaging.Lanczos)
	_ = imaging.Save(resized, tmpPath)

	return tmpPath, nil
}

func getFileExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	default:
		return ".jpg"
	}
}
