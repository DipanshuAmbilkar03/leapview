package duckdb

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	semanticquery "github.com/flidai/leapview/internal/analytics/query"
)

func TestProjectRuntimeCatalogTableStorageUsesServingSnapshotAndBoundedQuery(t *testing.T) {
	database := &catalogStatisticsDatabase{metadataSchema: "leapview_catalog_pool", rows: semanticquery.Rows{{
		"schema_name": "model", "table_name": "orders", "table_id": int64(4), "table_uuid": "table-uuid", "table_path": "model/orders/",
		"table_begin_snapshot": int64(7), "snapshot_id": int64(17), "column_count": int64(3),
		"file_count": int64(1), "row_count": int64(700), "byte_count": int64(2048),
		"file_id": int64(9), "file_path": "part-1.parquet", "file_format": "parquet", "record_count": int64(700), "file_size_bytes": int64(2048), "file_begin_snapshot": int64(12),
	}}}
	runtime := &ProjectRuntime{db: database, lastSnapshotID: 17}
	table, err := runtime.CatalogTableStorage(t.Context(), "model", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if table.ID != 4 || table.UUID != "table-uuid" || table.Path != "model/orders/" || table.BeginSnapshot != 7 || table.FileCount != 1 || len(table.Files) != 1 || table.Files[0].ID != 9 || table.Files[0].RecordCount != 700 || table.Files[0].SizeBytes != 2048 {
		t.Fatalf("CatalogTableStorage() = %#v, files = %#v", table, table.Files)
	}
	args := database.plan.Args
	if len(args) != 6 || args[0] != int64(17) || args[1] != int64(17) || args[2] != "lake" || args[3] != "model" || args[4] != "orders" || args[5] != maxCatalogStorageFiles+1 {
		t.Fatalf("query args = %#v", args)
	}
	if !strings.Contains(database.plan.SQL, `"__ducklake_metadata_lake"."leapview_catalog_pool".ducklake_data_file`) {
		t.Fatalf("storage query does not qualify the PostgreSQL metadata schema:\n%s", database.plan.SQL)
	}
	database.rows = nil
	if _, err := runtime.CatalogTableStorage(t.Context(), "model", "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing table error = %v", err)
	}
	database.rows = make(semanticquery.Rows, maxCatalogStorageFiles+1)
	if table, err := runtime.CatalogTableStorage(t.Context(), "model", "orders"); err == nil || table != nil {
		t.Fatalf("overflowing metadata returned partial detail: table=%#v err=%v", table, err)
	}
}

func TestProjectRuntimeCatalogTableStorageReadsRealFilesAtServingSnapshot(t *testing.T) {
	ctx, environment, runtime := openObservationWriterRuntime(t)
	for _, statement := range []string{
		"CREATE SCHEMA lake.storage_test",
		"CREATE TABLE lake.storage_test.orders AS SELECT range AS id FROM range(700)",
		"CREATE TABLE lake.storage_test.empty_orders (id BIGINT)",
	} {
		if err := environment.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	snapshots, err := environment.SnapshotIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime.lastSnapshotID = snapshots[len(snapshots)-1]
	before, err := runtime.CatalogTableStorage(ctx, "storage_test", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if before.UUID == "" || before.Path == "" || before.BeginSnapshot <= 0 || before.ColumnCount != 1 || before.RowCount != 700 || before.FileCount != 1 || len(before.Files) != 1 {
		t.Fatalf("storage detail = %#v, files = %#v", before, before.Files)
	}
	file := before.Files[0]
	if file.Path == "" || file.Format != "parquet" || file.RecordCount != 700 || file.SizeBytes <= 0 || file.SizeBytes != before.SizeBytes || file.BeginSnapshot <= 0 {
		t.Fatalf("active file = %#v", file)
	}
	empty, err := runtime.CatalogTableStorage(ctx, "storage_test", "empty_orders")
	if err != nil {
		t.Fatal(err)
	}
	if empty.UUID == "" || empty.Path == "" || empty.FileCount != 0 || empty.RowCount != 0 || empty.SizeBytes != 0 || len(empty.Files) != 0 {
		t.Fatalf("empty storage detail = %#v", empty)
	}
	// Replace the table in a later snapshot. The old table and its now-ended
	// file must remain visible to the original serving generation.
	if err := environment.Exec(ctx, "CREATE OR REPLACE TABLE lake.storage_test.orders AS SELECT 1 AS id"); err != nil {
		t.Fatal(err)
	}
	sealed, err := runtime.CatalogTableStorage(ctx, "storage_test", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.UUID != before.UUID || sealed.RowCount != 700 || len(sealed.Files) != 1 || sealed.Files[0].ID != file.ID || sealed.Files[0].EndSnapshot <= sealed.SnapshotID {
		t.Fatalf("sealed detail changed after catalog replacement: %#v, files = %#v", sealed, sealed.Files)
	}
	runtime.lastSnapshotID = 0 // Explicitly exercise the current-snapshot fallback.
	current, err := runtime.CatalogTableStorage(ctx, "storage_test", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if current.UUID == before.UUID || current.RowCount != 1 || current.FileCount != 1 || len(current.Files) != 1 || current.Files[0].ID == file.ID {
		t.Fatalf("current detail = %#v, files = %#v", current, current.Files)
	}
}
