package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgconn"
	"github.com/webitel/media-exporter/internal/domain/model/options"

	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	dberr "github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/store"
)

type Pdf struct {
	storage *Store
}

// --- Screenrecording History ---

func (m *Pdf) GetPdfExportHistory(req *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error) {
	filter := sq.And{
		sq.Eq{"h.agent_id": req.AgentID},
		sq.Or{
			sq.Eq{"h.file_id": nil},
			sq.Expr("EXISTS (SELECT 1 FROM storage.files f WHERE f.id = h.file_id AND f.removed IS NULL)"),
		},
	}
	return m.listHistory(filter, int64(req.Page), int64(req.Size), req.Sort)
}

// --- Call History ---

func (m *Pdf) GetCallPdfExportHistory(req *domain.CallHistoryRequestOptions) (*domain.HistoryResponse, error) {
	filter := sq.And{
		sq.Eq{"h.call_id": req.CallID},
		sq.Or{
			sq.Eq{"h.file_id": nil},
			sq.Expr("EXISTS (SELECT 1 FROM storage.files f WHERE f.id = h.file_id AND f.removed IS NULL)"),
		},
	}
	return m.listHistory(filter, int64(req.Page), int64(req.Size), req.Sort)
}

// Internal helper for paginated history fetching
func (m *Pdf) listHistory(filter sq.Sqlizer, page, size int64, sort string) (*domain.HistoryResponse, error) {
	db, err := m.storage.Database()
	if err != nil {
		return nil, dberr.NewDBInternalError("list_history", err)
	}

	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}

	offset := (page - 1) * size
	limit := size + 1

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query := psql.
		Select(
			"h.id", "h.name", "h.file_id", "h.mime",
			"h.uploaded_at", "h.updated_at", "h.uploaded_by", "h.updated_by", "h.status",
		).
		From("media_exporter.pdf_export_history h").
		Where(filter).
		OrderBy(parseSort(sort)).
		Offset(uint64(offset)).
		Limit(uint64(limit))

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return nil, dberr.NewDBInternalError("list_history", err)
	}

	rows, err := db.Query(context.Background(), sqlStr, args...)
	if err != nil {
		return nil, dberr.NewDBInternalError("list_history", err)
	}
	defer rows.Close()

	var records []*domain.HistoryRecord
	for rows.Next() {
		var rec domain.HistoryRecord
		var fileID sql.NullInt64
		var status string

		err := rows.Scan(
			&rec.ID, &rec.Name, &fileID, &rec.MimeType,
			&rec.CreatedAt, &rec.UpdatedAt, &rec.CreatedBy, &rec.UpdatedBy, &status,
		)
		if err != nil {
			return nil, dberr.NewDBInternalError("list_history", err)
		}

		if fileID.Valid {
			rec.FileID = fileID.Int64
		}
		rec.Status = status
		records = append(records, &rec)
	}

	hasNext := false
	if int64(len(records)) > size {
		hasNext = true
		records = records[:size]
	}

	return &domain.HistoryResponse{
		Next:  hasNext,
		Data:  records,
		Total: int64(len(records)) + offset, // Note: This is an estimation. For exact total, a separate COUNT query is needed.
	}, nil
}

// --- Mutations ---

func (m *Pdf) InsertPdfExportHistory(opts *options.CreateOptions, input *domain.NewExportHistory) (int64, error) {
	db, err := m.storage.Database()
	if err != nil {
		return 0, dberr.NewDBInternalError("insert_pdf_export_history", err)
	}

	query := `
       INSERT INTO media_exporter.pdf_export_history
          (name, file_id, mime, uploaded_at, updated_at, uploaded_by, status, agent_id, call_id, dc)
       VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8, $9)
       RETURNING id
    `

	var id int64

	// Перевірка на 0 для file_id
	var fileID sql.NullInt64
	if input.FileID != 0 {
		fileID = sql.NullInt64{Int64: input.FileID, Valid: true}
	}

	var agentID sql.NullInt64
	if input.AgentID != 0 {
		agentID = sql.NullInt64{Int64: input.AgentID, Valid: true}
	}

	var callID sql.NullString
	if input.CallID != "" {
		callID = sql.NullString{String: input.CallID, Valid: true}
	}

	err = db.QueryRow(
		context.Background(),
		query,
		input.Name,
		fileID,
		input.Mime,
		input.UploadedAt,
		input.UploadedBy,
		input.Status,
		agentID,
		callID,
		opts.Auth.GetDomainId(),
	).Scan(&id)
	if err != nil {
		return 0, m.handlePgError("insert_export_history", err)
	}

	return id, nil
}

func (m *Pdf) UpdatePdfExportStatus(input *domain.UpdateExportStatus) error {
	db, err := m.storage.Database()
	if err != nil {
		return dberr.NewDBInternalError("update_pdf_export_status", err)
	}

	query := `
    UPDATE media_exporter.pdf_export_history
    SET status = $1,
        updated_at = $2,
        updated_by = $3,
        file_id = COALESCE(NULLIF($4, 0), file_id)
    WHERE id = $5
`
	cmd, err := db.Exec(
		context.Background(),
		query,
		input.Status,
		time.Now().UnixMilli(),
		input.UpdatedBy,
		input.FileID,
		input.ID,
	)
	if err != nil {
		return dberr.NewDBInternalError("update_export_status", err)
	}

	if cmd.RowsAffected() == 0 {
		return dberr.NewDBNotFoundError("update_export_status", fmt.Sprintf("id=%d", input.ID))
	}

	return nil
}

func (m *Pdf) DeletePdfExportRecord(opts *options.DeleteOptions, recordID int64) error {
	db, err := m.storage.Database()
	if err != nil {
		return dberr.NewDBInternalError("delete_pdf_export_record", err)
	}

	query := `DELETE FROM media_exporter.pdf_export_history WHERE id = $1 AND dc = $2`

	cmd, err := db.Exec(opts.Context, query, recordID, opts.Auth.GetDomainId())
	if err != nil {
		return dberr.NewDBInternalError("delete_pdf_export_record", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("no export history record found for id=%d", recordID)
	}
	return nil
}

func (m *Pdf) handlePgError(op string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return &dberr.DBUniqueViolationError{DBError: *dberr.NewDBError(op, pgErr.Message), Column: pgErr.ConstraintName}
		case "23503":
			return &dberr.DBForeignKeyViolationError{DBError: *dberr.NewDBError(op, pgErr.Message), ForeignKeyTable: pgErr.TableName}
		}
	}
	return dberr.NewDBInternalError(op, err)
}

func parseSort(sort string) string {
	defaultColumn := "h.updated_at"
	defaultDirection := "DESC"

	if sort == "" {
		return defaultColumn + " " + defaultDirection
	}

	allowed := map[string]string{
		"created_at": "h.uploaded_at",
		"updated_at": "h.updated_at",
		"created_by": "h.uploaded_by",
		"name":       "h.name",
		"status":     "h.status",
	}

	direction := defaultDirection
	columnKey := sort

	switch sort[0] {
	case '+':
		direction = "ASC"
		columnKey = sort[1:]
	case '-':
		direction = "DESC"
		columnKey = sort[1:]
	default:
		return defaultColumn + " " + defaultDirection
	}

	columnKey = strings.ToLower(strings.TrimSpace(columnKey))

	column, ok := allowed[columnKey]
	if !ok {
		return defaultColumn + " " + defaultDirection
	}

	return column + " " + direction
}

func NewPdfStore(store *Store) (store.PdfStore, error) {
	if store == nil {
		return nil, dberr.NewDBInternalError("new_store", errors.New("store is nil"))
	}
	return &Pdf{storage: store}, nil
}
