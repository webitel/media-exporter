package model

// ExportTask is the job persisted in Redis. It must be JSON-serializable.
type ExportTask struct {
	TaskID  string            `json:"task_id"`
	UserID  int64             `json:"user_id"`
	Channel string            `json:"channel"`
	From    int64             `json:"from"`
	To      int64             `json:"to"`
	Headers map[string]string `json:"headers"` // serializable auth/metadata (e.g. "authorization")
	IDs     []int64           `json:"ids"`     // list of specific IDs to export, if any
	// NOTE: no context.Context field here — contexts are not serializable.
}
