package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgconn"
	pdfapi "github.com/webitel/media-exporter/api/pdf"
	dberr "github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/model"
	"github.com/webitel/media-exporter/internal/model/options"
	"github.com/webitel/media-exporter/internal/store"
)

type Pdf struct {
	storage *Store
}

func (m *Pdf) GetPdfExportHistory(opts *options.SearchOptions, req *pdfapi.PdfHistoryRequest) (*pdfapi.PdfHistoryResponse, error) {
	db, err := m.storage.Database()
	if err != nil {
		return nil, dberr.NewDBInternalError("get_pdf_export_history", err)
	}

	// Page & size
	page := int64(req.Page)
	if page < 1 {
		page = 1
	}
	size := int64(req.Size)
	if size <= 0 {
		size = 20
	}

	offset := (page - 1) * size
	limit := size + 1 // fetch one extra to check has_next

	// Build query with Squirrel
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	query := psql.
		Select(
			"id",
			"name",
			"file_id",
			"mime",
			"uploaded_at",
			"updated_at",
			"uploaded_by",
			"updated_by",
			"status",
		).
		From("media_exporter.pdf_export_history").
		Where(sq.Eq{"agent_id": req.AgentId}).
		Where(sq.Eq{"uploaded_by": opts.Auth.GetUserId()}).
		OrderBy("uploaded_at DESC").
		Offset(uint64(offset)).
		Limit(uint64(limit))

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return nil, dberr.NewDBInternalError("get_pdf_export_history", err)
	}

	rows, err := db.Query(context.Background(), sqlStr, args...)
	if err != nil {
		return nil, dberr.NewDBInternalError("get_pdf_export_history", err)
	}
	defer rows.Close()

	var records []*pdfapi.PdfHistoryRecord
	for rows.Next() {
		var record model.ExportHistory
		err := rows.Scan(
			&record.ID,
			&record.Name,
			&record.FileID,
			&record.Mime,
			&record.UploadedAt,
			&record.UpdatedAt,
			&record.UploadedBy,
			&record.UpdatedBy,
			&record.Status,
		)
		if err != nil {
			return nil, dberr.NewDBInternalError("get_pdf_export_history", err)
		}

		records = append(records, &pdfapi.PdfHistoryRecord{
			Id:        record.ID,
			Name:      record.Name,
			FileId:    record.FileID,
			MimeType:  record.Mime,
			CreatedAt: record.UploadedAt,
			UpdatedAt: record.UpdatedAt,
			CreatedBy: record.UploadedBy,
			UpdatedBy: record.UpdatedBy,
			Status:    mapStatusToProto(string(record.Status)),
		})
	}

	if err = rows.Err(); err != nil {
		return nil, dberr.NewDBInternalError("get_pdf_export_history", err)
	}

	// Check has_next
	hasNext := false
	if int64(len(records)) > size {
		hasNext = true
		records = records[:len(records)-1] // drop the extra record
	}

	return &pdfapi.PdfHistoryResponse{
		Page: int32(page),
		Next: hasNext,
		Data: records,
	}, nil
}

func mapStatusToProto(status string) pdfapi.PdfExportStatus {
	switch status {
	case "pending":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_PENDING
	case "processing":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_PROCESSING
	case "done":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_DONE
	case "failed":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_FAILED
	default:
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_UNSPECIFIED
	}
}

func (m *Pdf) InsertPdfExportHistory(input *model.NewExportHistory) (int64, error) {
	db, err := m.storage.Database()
	if err != nil {
		return 0, dberr.NewDBInternalError("insert_pdf_export_history", err)
	}

	query := `
		INSERT INTO media_exporter.pdf_export_history
			(name, file_id, mime, uploaded_at, updated_at, uploaded_by, status, agent_id)
		VALUES ($1, $2, $3, $4, $4, $5, $6, $7)
		RETURNING id
	`

	var id int64
	err = db.QueryRow(
		context.Background(),
		query,
		input.Name,
		input.FileID,
		input.Mime,
		input.UploadedAt,
		input.UploadedBy,
		input.Status,
		input.AgentID,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505": // unique_violation
				return 0, &dberr.DBUniqueViolationError{
					DBError: *dberr.NewDBError("insert_export_history", pgErr.Message),
					Column:  pgErr.ConstraintName,
				}
			case "23503": // foreign_key_violation
				return 0, &dberr.DBForeignKeyViolationError{
					DBError:         *dberr.NewDBError("insert_export_history", pgErr.Message),
					ForeignKeyTable: pgErr.TableName,
				}
			default:
				return 0, dberr.NewDBInternalError("insert_export_history", err)
			}
		}
		return 0, dberr.NewDBInternalError("insert_export_history", err)
	}

	return id, nil
}

func (m *Pdf) UpdatePdfExportStatus(input *model.UpdateExportStatus) error {
	db, err := m.storage.Database()
	if err != nil {
		return dberr.NewDBInternalError("update_pdf_export_status", err)
	}

	query := `
    UPDATE media_exporter.pdf_export_history
    SET status = $1,
        updated_at = $2,
        updated_by = $3,
        file_id = COALESCE($4, file_id)
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
		return dberr.NewDBNotFoundError("update_export_status",
			fmt.Sprintf("no export history record found for id=%d", input.ID))
	}

	return nil
}

func NewPdfStore(store *Store) (store.PdfStore, error) {
	if store == nil {
		return nil, dberr.NewDBInternalError("new_store", errors.New("store is nil"))
	}
	return &Pdf{storage: store}, nil
}
