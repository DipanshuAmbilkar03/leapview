package runtimefactory

import (
	"context"
	"database/sql"
	"testing"

	adminstorage "github.com/flidai/leapview/internal/admin/storage"
	"github.com/flidai/leapview/internal/analytics/catalogstats"
	semanticmodel "github.com/flidai/leapview/internal/analytics/model"
	semanticquery "github.com/flidai/leapview/internal/analytics/query"
	analyticscontract "github.com/flidai/leapview/internal/analytics/runtime"
	dashboardanalyticsruntime "github.com/flidai/leapview/internal/dashboard/analyticsruntime"
	dashboardruntime "github.com/flidai/leapview/internal/dashboard/runtime"
	dashboardruntimefactory "github.com/flidai/leapview/internal/dashboard/runtimefactory"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	projectruntime "github.com/flidai/leapview/internal/project/runtime"
)

func TestAdminStorageDetailReadsThroughLeasedProductionRuntimeAdapters(t *testing.T) {
	model := compiledPlannerTestModel()
	planner, err := semanticquery.NewCompiledPlanner(model)
	if err != nil {
		t.Fatal(err)
	}
	identity := projectgraph.ServingIdentity{ProjectID: "project:demo", Environment: "prod", GenerationID: "state-1"}
	definition, err := dashboardruntime.NewProjectDefinition(identity.ProjectID, "", "", map[projectgraph.ResourceID]*semanticmodel.Model{"semantic-model:sales": model}, nil)
	if err != nil {
		t.Fatal(err)
	}
	detail := &catalogstats.StorageTable{
		Table: catalogstats.Table{Schema: "model", Name: "orders", RowCount: 700, FileCount: 1, SizeBytes: 2048},
		UUID:  "table-uuid", Path: "orders/", BeginSnapshot: 7,
		Files: []catalogstats.StorageFile{{ID: 9, Path: "part-1.parquet", RecordCount: 700, SizeBytes: 2048}},
	}
	project := storageDetailProject{planner: planner, detail: detail}
	builder := dashboardanalyticsruntime.NewRuntimeBuilder(dashboardanalyticsruntime.RuntimeFactoryConfig{
		Projects: analyticscontract.ProjectFactoryFunc(func(context.Context, analyticscontract.ProjectRequest) (analyticscontract.Project, error) {
			return project, nil
		}),
	})
	service, err := builder(t.Context(), dashboardruntimefactory.Input{Identity: identity, Definition: definition, TargetID: "target-1", SnapshotSealID: "seal-1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	// Production adds these wrappers around the dashboard Service before its
	// runtime is exposed through a generation lease to admin Storage.
	prepared := &postgresPreparedRuntime{dashboardRuntimeWithGraph: &dashboardRuntimeWithGraph{Service: service}}
	lease := &storageDetailLease{runtime: prepared, identity: identity}
	table, err := (adminstorage.Service{Runtime: storageDetailProvider{lease: lease}}).Table(t.Context(), "model", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if table.TableUUID != detail.UUID || table.DuckLakePath != detail.Path || table.RowCount != 700 || table.FileCount != 1 || len(table.Files) != 1 || table.Files[0].ID != 9 || table.Files[0].Path != "part-1.parquet" || !lease.released {
		t.Fatalf("leased storage detail = %#v, files = %#v, released = %t", table, table.Files, lease.released)
	}
}

type storageDetailProject struct {
	analyticscontract.Project
	planner *semanticquery.Planner
	detail  *catalogstats.StorageTable
}

func (storageDetailProject) Close() error                                    { return nil }
func (p storageDetailProject) Planner(string) (*semanticquery.Planner, bool) { return p.planner, true }
func (p storageDetailProject) CatalogTableStatistics(context.Context) ([]catalogstats.Table, error) {
	return []catalogstats.Table{p.detail.Table}, nil
}
func (p storageDetailProject) CatalogTableStorage(_ context.Context, schema, name string) (*catalogstats.StorageTable, error) {
	if schema != p.detail.Schema || name != p.detail.Name {
		return nil, sql.ErrNoRows
	}
	return p.detail, nil
}

type storageDetailProvider struct{ lease *storageDetailLease }

func (p storageDetailProvider) Acquire(context.Context) (projectruntime.Lease, error) {
	return p.lease, nil
}

type storageDetailLease struct {
	runtime  projectruntime.Runtime
	identity projectgraph.ServingIdentity
	released bool
}

func (l *storageDetailLease) Runtime() projectruntime.Runtime        { return l.runtime }
func (l *storageDetailLease) Identity() projectgraph.ServingIdentity { return l.identity }
func (l *storageDetailLease) Release()                               { l.released = true }
