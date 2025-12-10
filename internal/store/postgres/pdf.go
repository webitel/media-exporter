package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func (m *Pdf) GetPdfExportHistory(req *domain.PdfHistoryRequestOptions) (*domain.HistoryResponse, error) {
	db, err := m.storage.Database()
	if err != nil {
		return nil, dberr.NewDBInternalError("get_pdf_export_history", err)
	}

	// Total & size
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
		Where(sq.Eq{"agent_id": req.AgentID}).
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

	// Проміжний зріз для сканування (відповідає стовпцям БД)
	var scannedRecords []*domain.ExportHistory
	for rows.Next() {
		var record domain.ExportHistory
		var fileID sql.NullInt64

		err := rows.Scan(
			&record.ID,
			&record.Name,
			&fileID,
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

		if fileID.Valid {
			record.FileID = fileID.Int64
		} else {
			record.FileID = 0
		}

		scannedRecords = append(scannedRecords, &record)
	}

	if err = rows.Err(); err != nil {
		return nil, dberr.NewDBInternalError("get_pdf_export_history", err)
	}

	hasNext := false
	if int64(len(scannedRecords)) > size {
		hasNext = true
		scannedRecords = scannedRecords[:len(scannedRecords)-1] // drop the extra record
	}

	finalRecords := make([]*domain.HistoryRecord, len(scannedRecords))
	for i, rec := range scannedRecords {
		finalRecords[i] = &domain.HistoryRecord{
			ID:        rec.ID,
			Name:      rec.Name,
			FileID:    rec.FileID,
			MimeType:  rec.Mime,
			CreatedAt: rec.UploadedAt,
			UpdatedAt: rec.UpdatedAt,
			CreatedBy: rec.UploadedBy,
			UpdatedBy: rec.UpdatedBy,
			Status:    string(rec.Status),
		}
	}

	return &domain.HistoryResponse{
		Next:  hasNext,
		Data:  finalRecords,
		Total: int64(len(finalRecords)) + offset,
	}, nil
}

func (m *Pdf) InsertPdfExportHistory(opts *options.CreateOptions, input *domain.NewExportHistory) (int64, error) {
	db, err := m.storage.Database()
	if err != nil {
		return 0, dberr.NewDBInternalError("insert_pdf_export_history", err)
	}

	query := `
		INSERT INTO media_exporter.pdf_export_history
			(name, file_id, mime, uploaded_at, updated_at, uploaded_by, status, agent_id, dc)
		VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8)
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
		opts.Auth.GetDomainId(),
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

func NewPdfStore(store *Store) (store.PdfStore, error) {
	if store == nil {
		return nil, dberr.NewDBInternalError("new_store", errors.New("store is nil"))
	}
	return &Pdf{storage: store}, nil
}
