package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/webitel/media-exporter/api/storage"
	model2 "github.com/webitel/media-exporter/internal/domain/model"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
)

func uploadPDFToStorage(ctx context.Context, session *model2.Session, app *App, filePath string, task domain.ExportTask) (*storage.UploadFileResponse, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file failed: %w", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			slog.ErrorContext(ctx, "close file failed", slog.String("file", filePath), err)
		}
	}(f)

	stream, err := app.StorageClient.UploadFile(ctx)
	if err != nil {
		return nil, fmt.Errorf("UploadFile init failed: %w", err)
	}

	if err := sendFileMetadata(stream, session, task); err != nil {
		return nil, err
	}
	if err := sendFileChunks(stream, f); err != nil {
		return nil, err
	}

	return stream.CloseAndRecv()
}

func sendFileMetadata(stream storage.FileService_UploadFileClient, session *model2.Session, task domain.ExportTask) error {
	channel, err := ParseUploadFileChannel(task.Channel)
	if err != nil {
		return err
	}
	return stream.Send(&storage.UploadFileRequest{
		Data: &storage.UploadFileRequest_Metadata_{
			Metadata: &storage.UploadFileRequest_Metadata{
				Name:           task.TaskID + ".pdf",
				MimeType:       "application/pdf",
				Uuid:           task.TaskID,
				StreamResponse: true,
				Channel:        channel,
				UploadedBy:     session.UserID(),
				DomainId:       session.DomainID(),
				CreatedAt:      time.Now().UnixMilli(),
			},
		},
	})
}

func ParseUploadFileChannel(channel string) (storage.UploadFileChannel, error) {
	switch channel {
	case "call":
		return storage.UploadFileChannel_ScreenRecordingChannel, nil
	case "screenrecording":
		return storage.UploadFileChannel_ScreenRecordingChannel, nil
	default:
		return 0, fmt.Errorf("invalid channel: %v", channel)
	}
}

func sendFileChunks(stream storage.FileService_UploadFileClient, f *os.File) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if sendErr := stream.Send(&storage.UploadFileRequest{
				Data: &storage.UploadFileRequest_Chunk{Chunk: buf[:n]},
			}); sendErr != nil {
				return fmt.Errorf("send chunk failed: %w", sendErr)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read file failed: %w", err)
		}
	}
}
