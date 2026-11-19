package service

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/auth"
	"github.com/webitel/media-exporter/internal/domain/model/options"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/store"
)

type fakeAuth struct {
	auth.Auther
}

func (fakeAuth) GetDomainId() int64 { return 7 }

type fakePdfStore struct {
	store.PdfStore

	records []*domain.HistoryRecord
	opts    *options.CreateOptions
	request *domain.PdfArchiveRequestOptions
}

func (s *fakePdfStore) ListDonePdfExports(opts *options.CreateOptions, req *domain.PdfArchiveRequestOptions) ([]*domain.HistoryRecord, error) {
	s.opts = opts
	s.request = req

	return s.records, nil
}

type fakeDownloadStream struct {
	grpc.ClientStream

	frames [][]byte
}

func (s *fakeDownloadStream) Recv() (*storage.StreamFile, error) {
	if len(s.frames) == 0 {
		return nil, io.EOF
	}

	frame := &storage.StreamFile{Data: &storage.StreamFile_Chunk{Chunk: s.frames[0]}}
	s.frames = s.frames[1:]

	return frame, nil
}

type fakeFileClient struct {
	storage.FileServiceClient

	files     []*storage.File
	searchErr error
	searches  []*storage.SearchScreenRecordingsRequest
}

func (c *fakeFileClient) SearchScreenRecordings(_ context.Context, in *storage.SearchScreenRecordingsRequest, _ ...grpc.CallOption) (*storage.ListFile, error) {
	c.searches = append(c.searches, proto.Clone(in).(*storage.SearchScreenRecordingsRequest))
	if c.searchErr != nil {
		return nil, c.searchErr
	}

	var items []*storage.File

	for _, file := range c.files {
		if slices.Contains(in.GetId(), file.GetId()) {
			items = append(items, file)
		}
	}

	return &storage.ListFile{Items: items, Next: true}, nil
}

func (c *fakeFileClient) DownloadFile(_ context.Context, _ *storage.DownloadFileRequest, _ ...grpc.CallOption) (storage.FileService_DownloadFileClient, error) {
	return &fakeDownloadStream{frames: [][]byte{[]byte("pdf-content")}}, nil
}

func pdfFile(id int64) *storage.File {
	return &storage.File{Id: id, ViewName: "stored.pdf.pdf", MimeType: "application/pdf"}
}

func record(fileID int64, name string) *domain.HistoryRecord {
	return &domain.HistoryRecord{FileID: fileID, Name: name}
}

func manyRecords(n int) []*domain.HistoryRecord {
	records := make([]*domain.HistoryRecord, 0, n)
	for i := range n {
		records = append(records, record(int64(i+1), "a.pdf"))
	}

	return records
}

func TestDownloadPdfArchive(t *testing.T) {
	tests := []struct {
		name         string
		request      *domain.DownloadPdfArchiveRequest
		records      []*domain.HistoryRecord
		files        []*storage.File
		searchErr    error
		wantCode     codes.Code
		wantEntries  []string
		wantIDs      []int64
		wantStore    bool
		wantSearches int
		wantLog      string
	}{
		{
			name:         "uses history names for every done export",
			request:      &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(11, "b.pdf")},
			files:        []*storage.File{pdfFile(10), pdfFile(11)},
			wantEntries:  []string{"a.pdf", "b.pdf"},
			wantIDs:      []int64{10, 11},
			wantStore:    true,
			wantSearches: 1,
		},
		{
			name:         "sends each file id once and ignores next",
			request:      &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(10, "a.pdf"), record(11, "b.pdf")},
			files:        []*storage.File{pdfFile(10), pdfFile(11)},
			wantEntries:  []string{"a.pdf", "b.pdf"},
			wantIDs:      []int64{10, 11},
			wantStore:    true,
			wantSearches: 1,
		},
		{
			name:         "numbers duplicate names",
			request:      &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(11, "a.pdf")},
			files:        []*storage.File{pdfFile(10), pdfFile(11)},
			wantEntries:  []string{"a.pdf", "a (2).pdf"},
			wantIDs:      []int64{10, 11},
			wantStore:    true,
			wantSearches: 1,
		},
		{
			name:         "empty history name falls back to pdf",
			request:      &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "")},
			files:        []*storage.File{pdfFile(10)},
			wantEntries:  []string{"pdf.pdf"},
			wantIDs:      []int64{10},
			wantStore:    true,
			wantSearches: 1,
		},
		{
			name:         "logs files storage does not return",
			request:      &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(11, "removed.pdf")},
			files:        []*storage.File{pdfFile(10)},
			wantEntries:  []string{"a.pdf"},
			wantIDs:      []int64{10, 11},
			wantStore:    true,
			wantSearches: 1,
			wantLog:      "missing_file_ids=[11]",
		},
		{
			name:     "requires agent id",
			request:  &domain.DownloadPdfArchiveRequest{},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "rejects too many file ids",
			request:  &domain.DownloadPdfArchiveRequest{AgentID: 1, FileIDs: make([]int64, maxPdfArchiveFiles+1)},
			wantCode: codes.InvalidArgument,
		},
		{
			name:      "rejects too many done exports",
			request:   &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:   manyRecords(maxPdfArchiveFiles + 1),
			wantCode:  codes.InvalidArgument,
			wantStore: true,
		},
		{
			name:      "no done exports skips storage",
			request:   &domain.DownloadPdfArchiveRequest{AgentID: 1},
			wantCode:  codes.NotFound,
			wantStore: true,
		},
		{
			name:         "no files in storage",
			request:      &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf")},
			wantCode:     codes.NotFound,
			wantIDs:      []int64{10},
			wantStore:    true,
			wantSearches: 1,
		},
		{
			name:         "keeps storage permission error",
			request:      &domain.DownloadPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf")},
			searchErr:    status.Error(codes.PermissionDenied, "forbidden"),
			wantCode:     codes.PermissionDenied,
			wantIDs:      []int64{10},
			wantStore:    true,
			wantSearches: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer

			pdfStore := &fakePdfStore{records: tc.records}
			client := &fakeFileClient{files: tc.files, searchErr: tc.searchErr}
			svc := &PdfServiceImpl{store: pdfStore, storageClient: client, log: slog.New(slog.NewTextHandler(&logs, nil))}
			opts := &options.CreateOptions{Context: context.Background(), Auth: fakeAuth{}}

			var archive bytes.Buffer

			err := svc.DownloadPdfArchive(context.Background(), opts, tc.request, &archive)
			if tc.wantCode != codes.OK {
				if got := errors.Code(err); got != tc.wantCode {
					t.Fatalf("code = %v, want %v (err: %v)", got, tc.wantCode, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if (pdfStore.request != nil) != tc.wantStore {
				t.Fatalf("store called = %v, want %v", pdfStore.request != nil, tc.wantStore)
			}

			if tc.wantStore && pdfStore.opts != opts {
				t.Errorf("store got opts %p, want %p", pdfStore.opts, opts)
			}

			if len(client.searches) != tc.wantSearches {
				t.Fatalf("searches = %d, want %d", len(client.searches), tc.wantSearches)
			}

			if tc.wantSearches > 0 {
				search := client.searches[0]
				if !slices.Equal(search.GetId(), tc.wantIDs) {
					t.Errorf("searched ids = %v, want %v", search.GetId(), tc.wantIDs)
				}

				if search.GetSize() != 1000 || search.GetPage() != 1 {
					t.Errorf("page/size = %d/%d, want 1/1000", search.GetPage(), search.GetSize())
				}

				if search.GetType() != storage.ScreenrecordingType_PDF {
					t.Errorf("type = %v, want PDF", search.GetType())
				}

				if search.GetChannel() != storage.ScreenrecordingChannel_SCREENRECORDING {
					t.Errorf("channel = %v, want SCREENRECORDING", search.GetChannel())
				}
			}

			if tc.wantEntries != nil {
				assertArchiveEntries(t, archive.Bytes(), tc.wantEntries)
			}

			if tc.wantLog != "" && !strings.Contains(logs.String(), tc.wantLog) {
				t.Errorf("log %q does not contain %q", logs.String(), tc.wantLog)
			}

			if tc.wantLog == "" && strings.Contains(logs.String(), "did not return") {
				t.Errorf("unexpected missing-files log: %q", logs.String())
			}
		})
	}
}

func TestDownloadPdfArchivePassesFilterToStore(t *testing.T) {
	pdfStore := &fakePdfStore{records: []*domain.HistoryRecord{record(10, "a.pdf")}}
	client := &fakeFileClient{files: []*storage.File{pdfFile(10)}}
	svc := &PdfServiceImpl{store: pdfStore, storageClient: client, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := &domain.DownloadPdfArchiveRequest{AgentID: 1, FileIDs: []int64{10}, From: 100, To: 200}

	var archive bytes.Buffer
	if err := svc.DownloadPdfArchive(context.Background(), &options.CreateOptions{Auth: fakeAuth{}}, request, &archive); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := domain.PdfArchiveRequestOptions{AgentID: 1, FileIDs: []int64{10}, From: 100, To: 200, Limit: maxPdfArchiveFiles + 1}
	got := pdfStore.request

	if got.AgentID != want.AgentID || !slices.Equal(got.FileIDs, want.FileIDs) || got.From != want.From || got.To != want.To || got.Limit != want.Limit {
		t.Errorf("store filter = %+v, want %+v", *got, want)
	}
}

func TestArchiveEntryName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		mimeType string
		want     string
	}{
		{"keeps matching extension", "a.pdf", "application/pdf", "a.pdf"},
		{"adds extension from mime", "shot", "image/png; charset=binary", "shot.png"},
		{"strips directories and colons", "../dir/12:30.png", "image/png", "12-30.png"},
		{"empty name uses fallback", "", "image/jpeg", "screenshot.jpg"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := archiveEntryName(tc.input, tc.mimeType, "screenshot"); got != tc.want {
				t.Errorf("archiveEntryName(%q, %q) = %q, want %q", tc.input, tc.mimeType, got, tc.want)
			}
		})
	}
}

func assertArchiveEntries(t *testing.T, archive []byte, want []string) {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}

	got := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		got = append(got, file.Name)

		src, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}

		content, err := io.ReadAll(src)
		_ = src.Close()

		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}

		if string(content) != "pdf-content" {
			t.Errorf("%s content = %q", file.Name, content)
		}
	}

	if !slices.Equal(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}
}
