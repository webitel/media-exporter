package store

import "github.com/webitel/media-exporter/internal/model"

type Store interface {
	MediaExporter() MediaExporterStore

	// ------------ Database Management ------------ //
	Open() error  // Return custom DB error
	Close() error // Return custom DB error
}

type MediaExporterStore interface {
	InsertExportHistory(input *model.NewExportHistory) (int64, error)
	UpdateExportStatus(input *model.UpdateExportStatus) error
}
