package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	mediaexporter "github.com/webitel/media-exporter/api/media_exporter"
	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/auth"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/server/interceptor"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/metadata"
)

type MediaExporterService struct {
	app *App
	mediaexporter.UnimplementedMediaExporterServiceServer
	objClassName string
}

func GetAutherOutOfContext(ctx context.Context) auth.Auther {
	return ctx.Value(interceptor.SessionHeader).(auth.Auther)
}

func (m *MediaExporterService) ExportPDF(ctx context.Context, req *mediaexporter.ExportRequest) (*mediaexporter.ExportResponse, error) {
	var channel = map[string]storage.UploadFileChannel{
		"screenshot":    storage.UploadFileChannel_ScreenshotChannel,
		"screensharing": storage.UploadFileChannel_ScreenSharingChannel,
	}
	enumChannel, ok := channel[req.Channel]
	if !ok {
		return nil, fmt.Errorf("invalid channel: %s", req.Channel)
	}

	//	//// 1. Redis check
	//	//exists, err := m.app.cache.Exists(strconv.FormatInt(req.From, 10), strconv.FormatInt(req.To, 10), req.Channel)
	//	//if err != nil {
	//	//	return nil, err
	//	//}
	//	//if exists {
	//	//	return nil, errors.Internal("export already in progress")
	//	//}
	//	//
	//	//err = m.app.cache.SetRequest(strconv.FormatInt(req.From, 10), strconv.FormatInt(req.To, 10), req.Channel, 30*time.Minute)
	//	//if err != nil {
	//	//	return nil, err
	//	//}

	info, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errors.Forbidden("internal.grpc.get_context: Not found")
	}
	newCtx := metadata.NewOutgoingContext(ctx, info)
	sess := GetAutherOutOfContext(ctx)

	//Fetch files from Storage API
	resp, err := m.app.storageClient.SearchScreenRecordings(newCtx, &storage.SearchScreenRecordingsRequest{
		Channel: enumChannel,
		UserId:  req.AgentId,
	})
	if err != nil {
		return nil, err
	}

	//download to temp
	tmpFiles := make(map[string]string)
	fileInfos := make(map[string]*storage.File)
	var mu sync.Mutex
	g := new(errgroup.Group)

	for _, f := range resp.Items {
		f := f
		g.Go(func() error {
			tmpPath, err := m.downloadToTemp(ctx, sess.GetDomainId(), f)
			if err != nil {
				return err
			}
			mu.Lock()
			tmpFiles[strconv.FormatInt(f.Id, 10)] = tmpPath
			fileInfos[strconv.FormatInt(f.Id, 10)] = f
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	//Generate PDF as bytes
	output, err := m.generatePDFBytes(tmpFiles, fileInfos)
	if err != nil {
		return nil, err
	}

	////Save PDF to file (optional)
	//if err := savePDFToFile(output, "output.pdf"); err != nil {
	//	return nil, err
	//}

	//Cleanup
	for _, path := range tmpFiles {
		_ = os.Remove(path)
	}
	_ = m.app.cache.Delete(strconv.FormatInt(req.From, 10), strconv.FormatInt(req.To, 10), req.Channel)

	return &mediaexporter.ExportResponse{
		Data: &mediaexporter.ExportResponse_Chunk{
			Chunk: output,
		},
	}, nil
}

func (m *MediaExporterService) downloadToTemp(ctx context.Context, domainID int64, f *storage.File) (string, error) {
	tmpDir := os.TempDir()

	ext := ".jpg"
	if f.MimeType != "" {
		switch f.MimeType {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		}
	}

	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%d_%s%s", f.Id, f.Name, ext))

	if f.Id == 0 || f.Name == "" {
		return "", fmt.Errorf("invalid file: id=%d, name='%s'", f.Id, f.Name)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	stream, err := m.app.storageClient.DownloadFile(ctx, &storage.DownloadFileRequest{
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
	if err := imaging.Save(resized, tmpPath); err != nil {
		return tmpPath, nil
	}

	return tmpPath, nil
}

func (m *MediaExporterService) generatePDFBytes(files map[string]string, fileInfos map[string]*storage.File) ([]byte, error) {
	maroto := pdf.NewMaroto(consts.Portrait, consts.A4)
	maroto.SetBorder(false)

	imageHeight := 60.0
	imageSpacing := 10.0
	textHeight := 10.0

	type tmpFile struct {
		Path      string
		Name      string
		Timestamp string
	}

	tmpFiles := make([]tmpFile, 0, len(files))

	for id, path := range files {
		file := fileInfos[id]
		filename := file.Name
		timestamp := "unknown"
		if file.UploadedAt > 0 {
			t := time.Unix(file.UploadedAt/1000, (file.UploadedAt%1000)*1e6)
			timestamp = t.Format("15:04 02.01.2006")
		}

		tmpFiles = append(tmpFiles, tmpFile{
			Path:      path,
			Name:      filename,
			Timestamp: timestamp,
		})
	}

	for i := 0; i < len(tmpFiles); i += 2 {
		maroto.Row(imageHeight, func() {
			maroto.Col(6, func() {
				if err := maroto.FileImage(tmpFiles[i].Path); err != nil {
					fmt.Println("Error adding image:", err)
				}
			})
			if i+1 < len(tmpFiles) {
				maroto.Col(6, func() {
					if err := maroto.FileImage(tmpFiles[i+1].Path); err != nil {
						fmt.Println("Error adding image:", err)
					}
				})
			}
		})

		maroto.Row(textHeight, func() {
			maroto.Col(6, func() {
				maroto.Text(tmpFiles[i].Name+"\n"+tmpFiles[i].Timestamp, props.Text{
					Size:  10,
					Align: consts.Center,
				})
			})
			if i+1 < len(tmpFiles) {
				maroto.Col(6, func() {
					maroto.Text(tmpFiles[i+1].Name+"\n"+tmpFiles[i+1].Timestamp, props.Text{
						Size:  10,
						Align: consts.Center,
					})
				})
			}
		})

		maroto.Row(imageSpacing, func() {})
	}

	buf, err := maroto.Output()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func savePDFToFile(pdfBytes []byte, filename string) error {
	return os.WriteFile(filename, pdfBytes, 0644)
}

func (m MediaExporterService) ExportZIP(
	ctx context.Context,
	request *mediaexporter.ExportRequest,
) (*mediaexporter.ExportResponse, error) {
	//TODO implement me
	panic("implement me")
}

// NewMediaExporterService creates a new MediaExporterService.
func NewMediaExporterService(app *App) (*MediaExporterService, error) {
	if app == nil {
		return nil, errors.Internal("internal is nil")
	}
	return &MediaExporterService{app: app, objClassName: "media_exporter"}, nil
}
