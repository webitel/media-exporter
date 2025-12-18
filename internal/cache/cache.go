package cache

import (
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
)

type Cache interface {
	Exists(taskID string) (bool, error)
	PushExportTask(task domain.ExportTask) error
	PopExportTask() (domain.ExportTask, error)
	SetExportStatus(taskID, status string) error
	GetExportStatus(taskID string) (string, error)
	SetExportURL(taskID, url string) error
	GetExportURL(taskID string) (string, error)
	SetExportHistoryID(taskID string, historyID int64) error
	GetExportHistoryID(taskID string) (int64, error)
	ClearExportTask(taskID string) error

	//FIXME needs to be deleted later
	// made for development purposes only
	Clear() error
}
