package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/analytics/catalogstats"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	projectruntime "github.com/flidai/leapview/internal/project/runtime"
)

func TestServiceReadsActiveRuntimeCatalogForProduction(t *testing.T) {
	tables := []catalogstats.Table{{
		Schema: "model", Name: "orders", RowCount: 42, ColumnCount: 3,
		FileCount: 2, SizeBytes: 4096, SnapshotID: 17,
	}}
	provider := &storageRuntimeProvider{runtime: &storageRuntimeStub{tables: tables}}
	service := Service{Runtime: provider}

	data := service.Data(context.Background())
	if data.Status != "" {
		t.Fatalf("Data() status = %q, want empty", data.Status)
	}
	if data.TableCount != 1 || data.DataFileCount != 2 || data.TotalDataSizeBytes != 4096 {
		t.Fatalf("Data() summary = %#v", data)
	}
	if len(data.Tables) != 1 || data.Tables[0].Schema != "model" || data.Tables[0].Name != "orders" || data.Tables[0].BeginSnapshot != 17 {
		t.Fatalf("Data() tables = %#v", data.Tables)
	}

	table, err := service.Table(context.Background(), "model", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if table.RowCount != 42 || table.FileCount != 2 || table.SizeBytes != 4096 {
		t.Fatalf("Table() = %#v", table)
	}
	if _, err := service.Table(context.Background(), "model", "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing Table() error = %v, want sql.ErrNoRows", err)
	}
}

func TestServiceTableLoadsPhysicalIdentityAndActiveFiles(t *testing.T) {
	detail := &catalogstats.StorageTable{
		Table: catalogstats.Table{Schema: "model", Name: "orders", RowCount: 700, ColumnCount: 3, FileCount: 1, SizeBytes: 2048, SnapshotID: 17},
		ID:    4, UUID: "table-uuid", Path: "model/orders/", BeginSnapshot: 7,
		Files: []catalogstats.StorageFile{{ID: 9, Path: "part-1.parquet", Format: "parquet", RecordCount: 700, SizeBytes: 2048, BeginSnapshot: 12}},
	}
	provider := &storageRuntimeProvider{runtime: &storageRuntimeStub{tables: []catalogstats.Table{detail.Table}, detail: detail}}
	table, err := (Service{Runtime: provider}).Table(t.Context(), "model", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if table.TableID != 4 || table.TableUUID != "table-uuid" || table.DuckLakePath != "model/orders/" || table.BeginSnapshot != 7 {
		t.Fatalf("Table() physical identity = %#v", table)
	}
	if len(table.Files) != 1 || table.Files[0].ID != 9 || table.Files[0].Path != "part-1.parquet" || table.Files[0].Format != "parquet" || table.Files[0].RecordCountLabel != "700" || table.Files[0].SizeLabel != "2.0 KiB" || table.Files[0].BeginSnapshot != 12 {
		t.Fatalf("Table() active files = %#v", table.Files)
	}
}

func TestServiceDoesNotFallBackWithoutActiveRuntime(t *testing.T) {
	service := Service{}

	data := service.Data(context.Background())
	if data.Status != storageInactiveStatus {
		t.Fatalf("Data() status = %q, want no-active-runtime status", data.Status)
	}
	if data.TableCount != 0 || data.DataFileCount != 0 || data.TotalDataSizeBytes != 0 || len(data.Tables) != 0 {
		t.Fatalf("Data() without runtime = %#v, want empty data", data)
	}

	if _, err := service.Table(context.Background(), "model", "orders"); !errors.Is(err, errNoActiveRuntime) {
		t.Fatalf("Table() error = %v, want errNoActiveRuntime", err)
	}
}

func TestServiceDoesNotExposeCatalogErrorsInStatus(t *testing.T) {
	const technical = `Catalog Error: Table with name ducklake_table does not exist (SELECT * FROM secret_relation)`
	provider := &storageRuntimeProvider{runtime: &storageRuntimeErrorStub{err: errors.New(technical)}}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	service := Service{Runtime: provider, Logger: logger}

	data := service.Data(context.Background())
	if data.Status != storageUnavailableStatus {
		t.Fatalf("Data() status = %q, want bounded unavailable status", data.Status)
	}
	if strings.Contains(data.Status, "ducklake_table") || strings.Contains(data.Status, "SELECT") {
		t.Fatalf("Data() leaked technical catalog details: %q", data.Status)
	}
	if !strings.Contains(logs.String(), "ducklake_table") || !strings.Contains(logs.String(), "read_catalog") {
		t.Fatalf("technical catalog diagnostic was not logged: %q", logs.String())
	}
}

type storageRuntimeProvider struct {
	runtime projectruntime.Runtime
}

func (p *storageRuntimeProvider) Acquire(context.Context) (projectruntime.Lease, error) {
	return &storageRuntimeLease{runtime: p.runtime}, nil
}

type storageRuntimeLease struct {
	runtime projectruntime.Runtime
}

func (l *storageRuntimeLease) Runtime() projectruntime.Runtime { return l.runtime }
func (*storageRuntimeLease) Identity() projectgraph.ServingIdentity {
	return projectgraph.ServingIdentity{}
}
func (*storageRuntimeLease) Release() {}

type storageRuntimeStub struct {
	tables []catalogstats.Table
	detail *catalogstats.StorageTable
}

type storageRuntimeErrorStub struct{ err error }

func (s *storageRuntimeErrorStub) Close() error { return nil }
func (s *storageRuntimeErrorStub) CatalogTableStatistics(context.Context) ([]catalogstats.Table, error) {
	return nil, s.err
}

func (s *storageRuntimeStub) Close() error { return nil }
func (s *storageRuntimeStub) CatalogTableStatistics(context.Context) ([]catalogstats.Table, error) {
	return s.tables, nil
}

func (s *storageRuntimeStub) CatalogTableStorage(_ context.Context, schema, name string) (*catalogstats.StorageTable, error) {
	if s.detail != nil && s.detail.Schema == schema && s.detail.Name == name {
		return s.detail, nil
	}
	for _, table := range s.tables {
		if table.Schema == schema && table.Name == name {
			return &catalogstats.StorageTable{Table: table}, nil
		}
	}
	return nil, sql.ErrNoRows
}
