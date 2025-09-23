package cache

import "github.com/webitel/media-exporter/internal/model"

type Cache interface {
	Exists(taskID string) (bool, error)
	PushExportTask(task model.ExportTask) error
	PopExportTask() (model.ExportTask, error)
	SetExportStatus(taskID, status string) error
	GetExportStatus(taskID string) (string, error)
	SetExportURL(taskID, url string) error
	GetExportURL(taskID string) (string, error)
}
