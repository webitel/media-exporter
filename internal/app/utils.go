package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	pdfapi "github.com/webitel/media-exporter/api/pdf"
	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/model"
	"google.golang.org/grpc/metadata"
)

func extractHeadersFromContext(ctx context.Context, keys []string) map[string]string {
	headers := make(map[string]string, len(keys))
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, k := range keys {
			if v := md.Get(k); len(v) > 0 {
				headers[k] = v[0]
			}
		}
	}
	return headers
}

func parseChannel(channel string) (storage.UploadFileChannel, error) {
	switch channel {
	case "screenshot":
		return storage.UploadFileChannel_ScreenshotChannel, nil
	case "screensharing":
		return storage.UploadFileChannel_ScreenSharingChannel, nil
	default:
		return 0, fmt.Errorf("invalid channel: %s", channel)
	}
}

func SavePDFToFile(pdfBytes []byte, filename string) error {
	return os.WriteFile(filename, pdfBytes, 0644)
}

func setTaskStatus(app *App, historyID int64, taskID, status string, updatedBy int64, fileID *int64) error {
	_ = app.cache.SetExportStatus(taskID, status)
	return app.Store.Pdf().UpdatePdfExportStatus(&model.UpdateExportStatus{
		ID:        historyID,
		Status:    model.ExportStatus(status),
		UpdatedBy: updatedBy,
		FileID:    fileID,
	})
}

// streamDownloadFile streams file chunks via gRPC to the client
func streamDownloadFile(ctx context.Context, client storage.FileServiceClient, req *pdfapi.PdfDownloadRequest, stream pdfapi.PdfService_DownloadPdfExportServer) error {
	s, err := client.DownloadFile(ctx, &storage.DownloadFileRequest{
		Id:       req.GetExportId(),
		DomainId: req.GetDomainId(),
	})
	if err != nil {
		return fmt.Errorf("init download stream failed: %w", err)
	}
	for {
		chunk, err := s.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("recv chunk failed: %w", err)
		}
		if err := stream.Send(&pdfapi.PdfExportChunk{Data: chunk.GetChunk()}); err != nil {
			return fmt.Errorf("send chunk failed: %w", err)
		}
	}
	return nil
}

func getFileExt(mime string) string {
	switch mime {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "application/pdf":
		return ".pdf"
	default:
		if i := strings.LastIndex(mime, "/"); i != -1 && i < len(mime)-1 {
			return "." + mime[i+1:]
		}
		return ""
	}
}
