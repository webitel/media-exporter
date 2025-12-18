package store

import (
	"github.com/webitel/media-exporter/internal/domain/model/options"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
)

type Store interface {
	Pdf() PdfStore

	// ------------ Database Management ------------ //
	Open() error
	Close() error
}

type PdfStore interface {
	// InsertPdfExportHistory adds a new record to the export history table.
	// Used by both screenrecordings and calls.
	InsertPdfExportHistory(opts *options.CreateOptions, input *domain.NewExportHistory) (int64, error)

	// UpdatePdfExportStatus updates the processing status and final file reference.
	UpdatePdfExportStatus(input *domain.UpdateExportStatus) error

	// GetPdfExportHistory retrieves paginated history for screenrecordings by AgentID.
	GetPdfExportHistory(req *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error)

	// GetCallPdfExportHistory retrieves paginated history for calls by CallID.
	GetCallPdfExportHistory(req *domain.CallHistoryRequestOptions) (*domain.HistoryResponse, error)

	// DeletePdfExportRecord removes a specific record from the history.
	DeletePdfExportRecord(opts *options.DeleteOptions, recordID int64) error
}
