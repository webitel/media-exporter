package util

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ZipStreamEntry struct {
	Name string // name to use for the file inside the archive
	Open func() (io.ReadCloser, error)
}

func StreamZipArchive(w io.Writer, entries []ZipStreamEntry) error {
	zw := zip.NewWriter(w)
	added := 0

	for _, entry := range entries {
		ok, err := addStreamEntry(zw, entry)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if ok {
			added++
		}
	}

	if added == 0 {
		_ = zw.Close()
		return errors.NotFound("no available files to archive")
	}

	return zw.Close()
}

func addStreamEntry(zw *zip.Writer, entry ZipStreamEntry) (bool, error) {
	src, err := entry.Open()
	if err != nil {
		if status.Code(err) == codes.NotFound {
			slog.Warn("skipping missing ZIP entry", "name", entry.Name, "error", err)
			return false, nil
		}
		return false, fmt.Errorf("open %s: %w", entry.Name, err)
	}
	defer func() { _ = src.Close() }()

	fw, err := zw.CreateHeader(&zip.FileHeader{
		Name:   entry.Name,
		Method: zip.Store,
	})
	if err != nil {
		return false, fmt.Errorf("create zip entry %s: %w", entry.Name, err)
	}

	if _, err := io.Copy(fw, src); err != nil {
		return false, fmt.Errorf("write zip entry %s: %w", entry.Name, err)
	}
	return true, nil
}

type storageFileReader struct {
	source storage.FileService_DownloadFileClient
	cancel context.CancelFunc
	buffer []byte
}

func NewStorageFileReader(ctx context.Context, client storage.FileServiceClient, domainID, fileID int64) (io.ReadCloser, error) {
	streamCtx, cancel := context.WithCancel(ctx)

	stream, err := client.DownloadFile(streamCtx, &storage.DownloadFileRequest{
		Id:       fileID,
		DomainId: domainID,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open download stream for file %d: %w", fileID, err)
	}

	reader := &storageFileReader{source: stream, cancel: cancel}
	if err := reader.receive(); err != nil {
		_ = reader.Close()
		if err == io.EOF {
			return nil, status.Errorf(codes.NotFound, "file %d has no content", fileID)
		}
		return nil, fmt.Errorf("read first chunk for file %d: %w", fileID, err)
	}

	return reader, nil
}

func (r *storageFileReader) receive() error {
	for len(r.buffer) == 0 {
		frame, err := r.source.Recv()
		if err != nil {
			return err
		}
		r.buffer = frame.GetChunk()
	}
	return nil
}

func (r *storageFileReader) Read(p []byte) (int, error) {
	if err := r.receive(); err != nil {
		return 0, err
	}

	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

func (r *storageFileReader) Close() error {
	r.cancel()
	return nil
}
