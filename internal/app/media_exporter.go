package app

import (
	"context"
	"fmt"
	"os"
	"sync"

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
}

func NewMediaExporterService(app *App) (*MediaExporterService, error) {
	if app == nil {
		return nil, errors.Internal("internal is nil")
	}
	return &MediaExporterService{app: app}, nil
}

func GetAutherOutOfContext(ctx context.Context) auth.Auther {
	return ctx.Value(interceptor.SessionHeader).(auth.Auther)
}

func (m *MediaExporterService) ExportPDF(ctx context.Context, req *mediaexporter.ExportRequest) (*mediaexporter.ExportResponse, error) {
	enumChannel, err := parseChannel(req.Channel)
	if err != nil {
		return nil, err
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

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errors.Forbidden("internal.grpc.get_context: Not found")
	}
	newCtx := metadata.NewOutgoingContext(ctx, md)
	sess := GetAutherOutOfContext(ctx)

	filesResp, err := m.app.storageClient.SearchScreenRecordings(newCtx, &storage.SearchScreenRecordingsRequest{
		Channel: enumChannel,
		UserId:  req.AgentId,
	})
	if err != nil {
		return nil, err
	}

	tmpFiles, fileInfos, err := m.downloadFilesConcurrently(ctx, sess.GetDomainId(), filesResp.Items)
	if err != nil {
		return nil, err
	}
	defer cleanupFiles(tmpFiles)

	output, err := generatePDF(tmpFiles, fileInfos)
	if err != nil {
		return nil, err
	}

	if err := savePDFToFile(output, "output.pdf"); err != nil {
		return nil, err
	}

	return &mediaexporter.ExportResponse{
		Data: &mediaexporter.ExportResponse_Chunk{Chunk: output},
	}, nil
}

func (m *MediaExporterService) ExportZIP(ctx context.Context, req *mediaexporter.ExportRequest) (*mediaexporter.ExportResponse, error) {
	panic("implement me")
}

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

func (m *MediaExporterService) downloadFilesConcurrently(ctx context.Context, domainID int64, files []*storage.File) (map[string]string, map[string]*storage.File, error) {
	tmpFiles := make(map[string]string)
	fileInfos := make(map[string]*storage.File)
	var mu sync.Mutex
	g := new(errgroup.Group)

	for _, f := range files {
		f := f
		g.Go(func() error {
			tmpPath, err := downloadAndResize(ctx, m.app.storageClient, domainID, f)
			if err != nil {
				return err
			}
			mu.Lock()
			tmpFiles[fmt.Sprint(f.Id)] = tmpPath
			fileInfos[fmt.Sprint(f.Id)] = f
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	return tmpFiles, fileInfos, nil
}

func cleanupFiles(files map[string]string) {
	for _, path := range files {
		_ = os.Remove(path)
	}
}
