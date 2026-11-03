package service

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"slices"
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

	records  []*domain.HistoryRecord
	domainID int64
	request  *domain.DownloadAgentPdfArchiveRequest
}

func (s *fakePdfStore) ListDonePdfExports(_ context.Context, domainID int64, req *domain.DownloadAgentPdfArchiveRequest) ([]*domain.HistoryRecord, error) {
	s.domainID = domainID
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

	pages     [][]*storage.File
	searchErr error
	searches  []*storage.SearchScreenRecordingsRequest
}

func (c *fakeFileClient) SearchScreenRecordings(_ context.Context, in *storage.SearchScreenRecordingsRequest, _ ...grpc.CallOption) (*storage.ListFile, error) {
	c.searches = append(c.searches, proto.Clone(in).(*storage.SearchScreenRecordingsRequest))
	if c.searchErr != nil {
		return nil, c.searchErr
	}

	page := int(in.GetPage()) - 1
	if page >= len(c.pages) {
		return &storage.ListFile{}, nil
	}

	return &storage.ListFile{Items: c.pages[page], Next: page+1 < len(c.pages)}, nil
}

func (c *fakeFileClient) DownloadFile(_ context.Context, in *storage.DownloadFileRequest, _ ...grpc.CallOption) (storage.FileService_DownloadFileClient, error) {
	return &fakeDownloadStream{frames: [][]byte{[]byte("pdf-content")}}, nil
}

func pdfFile(id int64) *storage.File {
	return &storage.File{Id: id, ViewName: "stored.pdf.pdf", MimeType: "application/pdf"}
}

func TestDownloadAgentPdfArchive(t *testing.T) {
	record := func(fileID int64, name string) *domain.HistoryRecord {
		return &domain.HistoryRecord{FileID: fileID, Name: name}
	}
	permissionDenied := status.Error(codes.PermissionDenied, "forbidden")

	tests := []struct {
		name         string
		request      *domain.DownloadAgentPdfArchiveRequest
		records      []*domain.HistoryRecord
		pages        [][]*storage.File
		searchErr    error
		wantCode     codes.Code
		wantEntries  []string
		wantIDs      []int64
		wantSearches int
	}{
		{
			name:         "uses history names for every done export",
			request:      &domain.DownloadAgentPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(11, "b.pdf")},
			pages:        [][]*storage.File{{pdfFile(10), pdfFile(11)}},
			wantEntries:  []string{"a.pdf", "b.pdf"},
			wantIDs:      []int64{10, 11},
			wantSearches: 1,
		},
		{
			name:         "follows storage pagination",
			request:      &domain.DownloadAgentPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(11, "b.pdf")},
			pages:        [][]*storage.File{{pdfFile(10)}, {pdfFile(11)}},
			wantEntries:  []string{"a.pdf", "b.pdf"},
			wantIDs:      []int64{10, 11},
			wantSearches: 2,
		},
		{
			name:         "numbers duplicate names",
			request:      &domain.DownloadAgentPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(11, "a.pdf")},
			pages:        [][]*storage.File{{pdfFile(10), pdfFile(11)}},
			wantEntries:  []string{"a.pdf", "a (2).pdf"},
			wantIDs:      []int64{10, 11},
			wantSearches: 1,
		},
		{
			name:         "leaves out files storage does not return",
			request:      &domain.DownloadAgentPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf"), record(11, "removed.pdf")},
			pages:        [][]*storage.File{{pdfFile(10)}},
			wantEntries:  []string{"a.pdf"},
			wantIDs:      []int64{10, 11},
			wantSearches: 1,
		},
		{
			name:     "requires agent id",
			request:  &domain.DownloadAgentPdfArchiveRequest{},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "no done exports skips storage",
			request:  &domain.DownloadAgentPdfArchiveRequest{AgentID: 1},
			wantCode: codes.NotFound,
		},
		{
			name:         "no files in storage",
			request:      &domain.DownloadAgentPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf")},
			wantCode:     codes.NotFound,
			wantIDs:      []int64{10},
			wantSearches: 1,
		},
		{
			name:         "keeps storage permission error",
			request:      &domain.DownloadAgentPdfArchiveRequest{AgentID: 1},
			records:      []*domain.HistoryRecord{record(10, "a.pdf")},
			searchErr:    permissionDenied,
			wantCode:     codes.PermissionDenied,
			wantIDs:      []int64{10},
			wantSearches: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pdfStore := &fakePdfStore{records: tc.records}
			client := &fakeFileClient{pages: tc.pages, searchErr: tc.searchErr}
			svc := &PdfServiceImpl{store: pdfStore, storageClient: client}

			var archive bytes.Buffer

			err := svc.DownloadAgentPdfArchive(context.Background(), &options.CreateOptions{Auth: fakeAuth{}}, tc.request, &archive)
			if tc.wantCode != codes.OK {
				if got := errors.Code(err); got != tc.wantCode {
					t.Fatalf("code = %v, want %v (err: %v)", got, tc.wantCode, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(client.searches) != tc.wantSearches {
				t.Fatalf("searches = %d, want %d", len(client.searches), tc.wantSearches)
			}

			if tc.wantSearches > 0 {
				search := client.searches[0]
				if !slices.Equal(search.GetId(), tc.wantIDs) {
					t.Errorf("searched ids = %v, want %v", search.GetId(), tc.wantIDs)
				}

				if search.GetType() != storage.ScreenrecordingType_PDF {
					t.Errorf("type = %v, want PDF", search.GetType())
				}

				if search.GetChannel() != storage.ScreenrecordingChannel_SCREENRECORDING {
					t.Errorf("channel = %v, want SCREENRECORDING", search.GetChannel())
				}

				if pdfStore.domainID != 7 {
					t.Errorf("domain = %d, want 7", pdfStore.domainID)
				}
			}

			if tc.wantEntries != nil {
				assertArchiveEntries(t, archive.Bytes(), tc.wantEntries)
			}
		})
	}
}

func TestDownloadAgentPdfArchivePassesRequestToStore(t *testing.T) {
	pdfStore := &fakePdfStore{records: []*domain.HistoryRecord{{FileID: 10, Name: "a.pdf"}}}
	client := &fakeFileClient{pages: [][]*storage.File{{pdfFile(10)}}}
	svc := &PdfServiceImpl{store: pdfStore, storageClient: client}
	request := &domain.DownloadAgentPdfArchiveRequest{AgentID: 1, FileIDs: []int64{10}, From: 100, To: 200}

	var archive bytes.Buffer
	if err := svc.DownloadAgentPdfArchive(context.Background(), &options.CreateOptions{Auth: fakeAuth{}}, request, &archive); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pdfStore.request != request {
		t.Errorf("store request = %+v, want %+v", pdfStore.request, request)
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
