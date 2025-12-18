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
	InsertPdfExportHistory(opts *options.CreateOptions, input *model.NewExportHistory) (int64, error)
	UpdatePdfExportStatus(input *model.UpdateExportStatus) error
	GetScreenrecordingPdfExportHistory(opts *options.SearchOptions, request *pdfapi.PdfScreenrecordingHistoryRequest) (*pdfapi.PdfHistoryResponse, error)
	GetCallPdfExportHistory(opts *options.SearchOptions, request *pdfapi.PdfCallPdfHistoryRequest) (*pdfapi.PdfHistoryResponse, error)
	DeletePdfExportRecord(opts *options.DeleteOptions, request *pdfapi.DeletePdfExportRecordRequest) error
}
