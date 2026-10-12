package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/flidai/leapview/internal/analytics/catalogstats"
	semanticquery "github.com/flidai/leapview/internal/analytics/query"
)

const maxCatalogStorageFiles = 10000

// CatalogTableStorage reads a single table's identity and active data files
// from the serving snapshot. The SQL limit bounds materialized metadata; an
// overflowing result is rejected rather than presented as a complete list.
func (r *ProjectRuntime) CatalogTableStorage(ctx context.Context, schema, name string) (*catalogstats.StorageTable, error) {
	queryer, metadataNamespace, err := r.catalogMetadataQueryer()
	if err != nil {
		return nil, err
	}
	snapshotID := r.DuckLakeSnapshotID()
	rows, err := queryer.Query(ctx, semanticquery.Plan{
		SQL:  strings.ReplaceAll(catalogStorageSQL, "__ducklake_metadata_lake", metadataNamespace),
		Args: []any{snapshotID, snapshotID, "lake", schema, name, maxCatalogStorageFiles + 1},
		Columns: []string{
			"schema_name", "table_name", "table_id", "table_uuid", "table_path", "table_begin_snapshot", "table_end_snapshot",
			"snapshot_id", "column_count", "file_count", "row_count", "byte_count",
			"file_id", "file_path", "file_format", "record_count", "file_size_bytes", "file_begin_snapshot", "file_end_snapshot",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("inspect serving DuckLake table storage: %w", err)
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	if len(rows) > maxCatalogStorageFiles {
		return nil, fmt.Errorf("DuckLake table contains more than %d active data files", maxCatalogStorageFiles)
	}
	row := rows[0]
	table := &catalogstats.StorageTable{
		Table: catalogstats.Table{
			Schema: catalogStatisticString(row["schema_name"]), Name: catalogStatisticString(row["table_name"]),
			SnapshotID: catalogStatisticInt64(row["snapshot_id"]), ColumnCount: catalogStatisticInt64(row["column_count"]),
			FileCount: catalogStatisticInt64(row["file_count"]), RowCount: catalogStatisticInt64(row["row_count"]), SizeBytes: catalogStatisticInt64(row["byte_count"]),
		},
		ID: catalogStatisticInt64(row["table_id"]), UUID: catalogStatisticString(row["table_uuid"]), Path: catalogStatisticString(row["table_path"]),
		BeginSnapshot: catalogStatisticInt64(row["table_begin_snapshot"]), EndSnapshot: catalogStatisticInt64(row["table_end_snapshot"]),
		Files: make([]catalogstats.StorageFile, 0, len(rows)),
	}
	for _, row := range rows {
		// An empty table still has one LEFT JOIN row, with no file identity.
		if row["file_id"] == nil {
			continue
		}
		table.Files = append(table.Files, catalogstats.StorageFile{
			ID: catalogStatisticInt64(row["file_id"]), Path: catalogStatisticString(row["file_path"]), Format: catalogStatisticString(row["file_format"]),
			RecordCount: catalogStatisticInt64(row["record_count"]), SizeBytes: catalogStatisticInt64(row["file_size_bytes"]),
			BeginSnapshot: catalogStatisticInt64(row["file_begin_snapshot"]), EndSnapshot: catalogStatisticInt64(row["file_end_snapshot"]),
		})
	}
	return table, nil
}

const catalogStorageSQL = `
WITH selected_snapshot AS (
	SELECT CASE WHEN ? > 0 THEN ? ELSE id END AS id
	FROM ducklake_current_snapshot(?)
), active_table AS (
	SELECT s.schema_name, t.table_name, t.table_id, t.table_uuid, t.path,
	       t.begin_snapshot, t.end_snapshot
	FROM __ducklake_metadata_lake.ducklake_table t
	JOIN __ducklake_metadata_lake.ducklake_schema s ON s.schema_id = t.schema_id
	CROSS JOIN selected_snapshot selected
	WHERE s.schema_name = ? AND t.table_name = ?
	  AND t.begin_snapshot <= selected.id
	  AND (t.end_snapshot IS NULL OR selected.id < t.end_snapshot)
	  AND s.begin_snapshot <= selected.id
	  AND (s.end_snapshot IS NULL OR selected.id < s.end_snapshot)
), active_files AS (
	SELECT f.table_id, f.data_file_id, f.path, f.file_format,
	       f.record_count, f.file_size_bytes, f.begin_snapshot, f.end_snapshot
	FROM __ducklake_metadata_lake.ducklake_data_file f
	JOIN active_table a ON a.table_id = f.table_id
	CROSS JOIN selected_snapshot selected
	WHERE f.begin_snapshot <= selected.id
	  AND (f.end_snapshot IS NULL OR selected.id < f.end_snapshot)
)
SELECT a.schema_name, a.table_name, a.table_id, CAST(a.table_uuid AS VARCHAR), a.path,
       a.begin_snapshot, a.end_snapshot, selected.id,
       (SELECT count(*) FROM __ducklake_metadata_lake.ducklake_column c
        WHERE c.table_id = a.table_id AND c.parent_column IS NULL
          AND c.begin_snapshot <= selected.id
          AND (c.end_snapshot IS NULL OR selected.id < c.end_snapshot)),
       count(f.data_file_id) OVER (),
       coalesce(sum(f.record_count) OVER (), 0),
       coalesce(sum(f.file_size_bytes) OVER (), 0),
       f.data_file_id, f.path, f.file_format, f.record_count, f.file_size_bytes,
       f.begin_snapshot, f.end_snapshot
FROM active_table a
CROSS JOIN selected_snapshot selected
LEFT JOIN active_files f ON f.table_id = a.table_id
ORDER BY f.data_file_id
LIMIT ?`
