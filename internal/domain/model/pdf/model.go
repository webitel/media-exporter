package domain

import (
	"context"

	"google.golang.org/grpc/metadata"
)

type ExportStatus string

type ExportChannel string

const (
	ChannelCall            ExportChannel = "call"
	ChannelScreenRecording ExportChannel = "screenrecording"
	ChannelUnknown         ExportChannel = "unknown"
)

type GenerateExportRequest struct {
	AgentID  int64
	FileIDs  []int64
	Channel  int32
	From, To int64
}

type ExportHistory struct {
	ID         int64        `db:"id"`
	Name       string       `db:"name"`
	FileID     int64        `db:"file_id"`
	Mime       string       `db:"mime"`
	UploadedAt int64        `db:"uploaded_at"`
	UpdatedAt  int64        `db:"updated_at"`
	UploadedBy int64        `db:"uploaded_by"`
	UpdatedBy  int64        `db:"updated_by"`
	Status     ExportStatus `db:"status"`
	AgentID    int64        `db:"agent_id"`
	DomainID   int64        `db:"dc"`
}
type UpdateExportStatus struct {
	ID        int64        `db:"id"`
	FileID    *int64       `db:"file_id"`
	Status    ExportStatus `db:"status"`
	UpdatedBy int64        `db:"updated_by"`
}
type PdfExportMetadata struct {
	TaskID   string `db:"task_id"`
	FileName string `db:"file_name"`
	MimeType string `db:"mime_type"`
	Status   string `db:"status"`
}

type NewExportHistory struct {
	Name       string `db:"name"`
	Mime       string `db:"mime"`
	UploadedAt int64  `db:"uploaded_at"`
	UploadedBy int64  `db:"uploaded_by"`
	Status     string `db:"status"`
	AgentID    int64  `db:"agent_id"`
	FileID     int64  `db:"file_id"`
}

type ExportTask struct {
	TaskID   string            `db:"task_id"`
	AgentID  int64             `db:"agent_id"`
	UserID   int64             `db:"user_id"`
	DomainID int64             `db:"domain_id"`
	Channel  string            `db:"channel"`
	From     int64             `db:"from"`
	To       int64             `db:"to"`
	Headers  map[string]string `json:"headers"`
	IDs      []int64           `db:"ids"`
	Type     string            `db:"type"`
}

type PdfHistoryRequestOptions struct {
	AgentID int64 `db:"agent_id"`
	Page    int64 `db:"page"`
	Size    int64 `db:"size"`
}

type HistoryRecord struct {
	ID        int64  `db:"id"`
	Name      string `db:"name"`
	FileID    int64  `db:"file_id"`
	MimeType  string `db:"mime_type"`
	CreatedAt int64  `db:"created_at"`
	UpdatedAt int64  `db:"updated_at"`
	CreatedBy int64  `db:"created_by"`
	UpdatedBy int64  `db:"updated_by"`
	Status    string `db:"status"`
}

type HistoryResponse struct {
	Data  []*HistoryRecord `db:"data"`
	Total int64            `db:"total"`
	Next  bool             `db:"next"`
}

const PdfExportType = "pdf"

func ExtractHeadersFromContext(ctx context.Context, keys []string) map[string]string {
	headers := make(map[string]string, len(keys))
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, k := range keys {
			if v := md.Get(k); len(v) > 0 {
				headers[k] = v[0]
			}
		}
	}
	return headers
}
