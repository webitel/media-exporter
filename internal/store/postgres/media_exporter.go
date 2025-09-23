package postgres

import (
	"context"
	"errors"
	"time"

	dberr "github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/model"
	"github.com/webitel/media-exporter/internal/store"

	"github.com/jackc/pgconn"
)

type MediaExporter struct {
	storage *Store
}

func (m *MediaExporter) InsertExportHistory(input *model.NewExportHistory) (int64, error) {
	db, err := m.storage.Database()
	if err != nil {
		return 0, dberr.NewDBInternalError("insert_export_history", err)
	}

	query := `
		INSERT INTO media_exporter.export_history
			(name, file_id, mime, uploaded_at, updated_at, uploaded_by, status)
		VALUES ($1, $2, $3, $4, $4, $5, $6)
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
	).Scan(&id)
	if err != nil {
		// розбір Postgres помилки
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

func (m *MediaExporter) UpdateExportStatus(input *model.UpdateExportStatus) error {
	db, err := m.storage.Database()
	if err != nil {
		return dberr.NewDBInternalError("update_export_status", err)
	}

	query := `
		UPDATE media_exporter.export_history
		SET status = $1,
		    updated_at = $2,
		    updated_by = $3
		WHERE id = $4
	`

	cmd, err := db.Exec(
		context.Background(),
		query,
		input.Status,
		time.Now().UnixMilli(),
		input.UpdatedBy,
		input.ID,
	)
	if err != nil {
		return dberr.NewDBInternalError("update_export_status", err)
	}

	if cmd.RowsAffected() == 0 {
		return dberr.NewDBNotFoundError("update_export_status", "no export history record found")
	}

	return nil
}

func NewMediaExporterStore(store *Store) (store.MediaExporterStore, error) {
	if store == nil {
		return nil, dberr.NewDBInternalError("new_store", errors.New("store is nil"))
	}
	return &MediaExporter{storage: store}, nil
}
