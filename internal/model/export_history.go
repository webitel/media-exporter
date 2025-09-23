// internal/model/export_history.go
package model

type ExportStatus string

const (
	ExportStatusPending   ExportStatus = "pending"
	ExportStatusCompleted ExportStatus = "completed"
	ExportStatusFailed    ExportStatus = "failed"
)

type ExportHistory struct {
	ID         int64        `db:"id"`
	Name       string       `db:"name"`
	FileID     int64        `db:"file_id"`
	Mime       string       `db:"mime"`
	UploadedAt int64        `db:"uploaded_at"`
	UpdatedAt  int64        `db:"updated_at"`
	UploadedBy *int64       `db:"uploaded_by"`
	UpdatedBy  *int64       `db:"updated_by"`
	Status     ExportStatus `db:"status"`
}

type NewExportHistory struct {
	Name       string       `db:"name"`
	FileID     int64        `db:"file_id"`
	Mime       string       `db:"mime"`
	UploadedAt int64        `db:"uploaded_at"`
	UploadedBy *int64       `db:"uploaded_by"`
	Status     ExportStatus `db:"status"`
}

type UpdateExportStatus struct {
	ID        int64        `db:"id"`
	Status    ExportStatus `db:"status"`
	UpdatedBy *int64       `db:"updated_by"`
}
