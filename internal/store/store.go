package store

import (
	pdfapi "github.com/webitel/media-exporter/api/pdf"
	"github.com/webitel/media-exporter/internal/model"
	"github.com/webitel/media-exporter/internal/model/options"
)

type Store interface {
	Pdf() PdfStore

	// ------------ Database Management ------------ //
	Open() error  // Return custom DB error
	Close() error // Return custom DB error
}

type PdfStore interface {
	InsertPdfExportHistory(input *model.NewExportHistory) (int64, error)
	UpdatePdfExportStatus(input *model.UpdateExportStatus) error
	GetPdfExportHistory(opts *options.SearchOptions, request *pdfapi.PdfHistoryRequest) (*pdfapi.PdfHistoryResponse, error)
}
