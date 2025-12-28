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

// UpdateExportStatus is used to change the status of an export
// and attach the final file reference after processing.
type UpdateExportStatus struct {
	ID        int64  `db:"id"`
	FileID    *int64 `db:"file_id"` // Reference to the generated file in storage
	Status    string `db:"status"`  // New status (e.g., "done", "failed")
	Size      int64  `db:"size"`    // Final file size in bytes
	UpdatedBy int64  `db:"updated_by"`
	UpdatedAt int64  `db:"updated_at"`
}

const PdfExportType = "pdf"

// --- Request Models ---

// GenerateExportRequest used for Screenrecording
type GenerateExportRequest struct {
	AgentID int64
	FileIDs []int64
	From    int64
	To      int64
}

// GenerateCallExportRequest used for Calls
type GenerateCallExportRequest struct {
	CallID  string
	FileIDs []int64
	From    int64
	To      int64
}

type PdfHistoryRequestOptions struct {
	AgentID int64
	Page    int32
	Size    int32
	Sort    string
}

type CallHistoryRequestOptions struct {
	CallID string
	Page   int32
	Size   int32
	Sort   string
}

// --- Task & Metadata Models ---

type ExportTask struct {
	TaskID   string            `json:"task_id"`
	AgentID  int64             `json:"agent_id,omitempty"`
	CallID   string            `json:"call_id,omitempty"`
	UserID   int64             `json:"user_id"`
	DomainID int64             `json:"domain_id"`
	Channel  string            `json:"channel"`
	From     int64             `json:"from"`
	To       int64             `json:"to"`
	Headers  map[string]string `json:"headers"`
	IDs      []int64           `json:"ids"`
	Type     string            `json:"type"`
}

type PdfExportMetadata struct {
	TaskID   string `db:"task_id"`
	FileName string `db:"file_name"`
	MimeType string `db:"mime_type"`
	Status   string `db:"status"`
	Size     int64  `db:"size"`
}

// --- Persistence Models (Storage/DB) ---

type NewExportHistory struct {
	Name       string `db:"name"`
	Mime       string `db:"mime"`
	UploadedAt int64  `db:"uploaded_at"`
	UploadedBy int64  `db:"uploaded_by"`
	Status     string `db:"status"`
	AgentID    int64  `db:"agent_id,omitempty"`
	CallID     string `db:"call_id,omitempty"`
	FileID     int64  `db:"file_id"`
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

// --- Utils ---

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
