package util

import (
	"archive/zip"
	"context"
	"fmt"
	"io"

	"github.com/webitel/media-exporter/api/storage"
)

type ZipStreamEntry struct {
	Name string // name to use for the file inside the archive
	Open func() (io.ReadCloser, error)
}

func StreamZipArchive(w io.Writer, entries []ZipStreamEntry) error {
	zw := zip.NewWriter(w)

	for _, entry := range entries {
		if err := addStreamEntry(zw, entry); err != nil {
			_ = zw.Close()
			return err
		}
	}

	return zw.Close()
}

func addStreamEntry(zw *zip.Writer, entry ZipStreamEntry) error {
	src, err := entry.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", entry.Name, err)
	}
	defer func() { _ = src.Close() }()

	fw, err := zw.CreateHeader(&zip.FileHeader{
		Name:   entry.Name,
		Method: zip.Store,
	})
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", entry.Name, err)
	}

	if _, err := io.Copy(fw, src); err != nil {
		return fmt.Errorf("write zip entry %s: %w", entry.Name, err)
	}
	return nil
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

	return &storageFileReader{source: stream, cancel: cancel}, nil
}

func (r *storageFileReader) Read(p []byte) (int, error) {
	for len(r.buffer) == 0 {
		frame, err := r.source.Recv()
		if err != nil {
			return 0, err
		}
		r.buffer = frame.GetChunk()
	}

	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

func (r *storageFileReader) Close() error {
	r.cancel()
	return nil
}
