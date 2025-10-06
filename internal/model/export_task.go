package model

// ExportTask is the job persisted in Redis. It must be JSON-serializable.
type ExportTask struct {
	TaskID   string `json:"task_id"`
	UserID   int64  `json:"user_id"`
	AgentID  int64  `json:"agent_id"`
	DomainID int64  `json:"domain_id"`
	Channel  string `json:"channel"`
	// From / To are Unix milliseconds timestamps to filter files by upload date
	From    int64             `json:"from"`
	To      int64             `json:"to"`
	Type    string            `json:"type"`    // e.g. PDF, ZIP, etc.
	Headers map[string]string `json:"headers"` // serializable auth/metadata (e.g. "authorization")
	IDs     []int64           `json:"ids"`     // list of specific IDs to export, if any
}
