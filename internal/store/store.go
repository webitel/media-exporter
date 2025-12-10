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
	InsertPdfExportHistory(opts *options.CreateOptions, input *domain.NewExportHistory) (int64, error)
	UpdatePdfExportStatus(input *domain.UpdateExportStatus) error
	GetPdfExportHistory(req *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error)
	DeletePdfExportRecord(opts *options.DeleteOptions, recordID int64) error
}
