package postgres

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	conf "github.com/webitel/media-exporter/config"
	"github.com/webitel/media-exporter/internal/errors"
	"github.com/webitel/media-exporter/internal/store"
	otelpgx "github.com/webitel/webitel-go-kit/infra/otel/instrumentation/pgx"
)

// Store is the struct implementing the Store interface.
type Store struct {
	pdfStore store.PdfStore
	config   *conf.DatabaseConfig
	conn     *pgxpool.Pool
}

// New creates a new Store instance.
func New(config *conf.DatabaseConfig) *Store {
	return &Store{config: config}
}

func (s *Store) Pdf() store.PdfStore {
	if s.pdfStore == nil {
		cs, err := NewPdfStore(s)
		if err != nil {
			return nil
		}
		s.pdfStore = cs
	}
	return s.pdfStore
}

// Database returns the database connection or a custom error if it is not opened.
func (s *Store) Database() (*pgxpool.Pool, error) { // Return custom DB error
	if s.conn == nil {
		return nil, errors.New("database connection is not opened")
	}
	return s.conn, nil
}

// Open establishes a connection to the database and returns a custom error if it fails.
func (s *Store) Open() error {
	config, err := pgxpool.ParseConfig(s.config.Url)
	if err != nil {
		return err
	}

	// Attach the OpenTelemetry tracer for pgx
	config.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName())

	conn, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return err
	}
	s.conn = conn
	slog.Debug("cases.store.connection_opened", slog.String("message", "postgres: connection opened"))
	return nil
}

// Close closes the database connection and returns a custom error if it fails.
func (s *Store) Close() error {
	if s.conn != nil {
		s.conn.Close()
		slog.Debug("cases.store.connection_closed", slog.String("message", "postgres: connection closed"))
		s.conn = nil
	}
	return nil
}
