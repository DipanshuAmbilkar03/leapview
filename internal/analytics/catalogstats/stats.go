// Package catalogstats defines the physical, generation-bound metadata that
// an analytical runtime may expose to catalog presentation surfaces.
package catalogstats

import (
	"context"
	"time"

	semanticmodel "github.com/flidai/leapview/internal/analytics/model"
)

// Table describes one physical DuckLake table at the runtime's serving
// snapshot. It deliberately excludes file paths and storage credentials.
type Table struct {
	Schema      string
	Name        string
	RowCount    int64
	ColumnCount int64
	FileCount   int64
	SizeBytes   int64
	SnapshotID  int64
	SnapshotAt  time.Time
	Columns     []semanticmodel.ColumnSchema
}

// Reader is implemented by serving runtimes that can inspect their own
// immutable physical catalog.
type Reader interface {
	CatalogTableStatistics(context.Context) ([]Table, error)
}

// StorageReader exposes physical file metadata for an administrative table
// detail. It is separate from Reader so overview queries do not load files.
type StorageReader interface {
	CatalogTableStorage(context.Context, string, string) (*StorageTable, error)
}

// StorageTable includes physical identity and the complete bounded file list.
// Path preserves DuckLake's stored table path, which can be schema-relative.
type StorageTable struct {
	Table
	ID            int64
	UUID          string
	Path          string
	BeginSnapshot int64
	EndSnapshot   int64
	Files         []StorageFile
}

// StorageFile preserves DuckLake's stored file path, which may be relative
// to the table's data path. Storage credentials are never included.
type StorageFile struct {
	ID            int64
	Path          string
	Format        string
	RecordCount   int64
	SizeBytes     int64
	BeginSnapshot int64
	EndSnapshot   int64
}
