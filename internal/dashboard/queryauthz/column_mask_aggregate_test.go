package authz

import (
	"context"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access"
	accesspolicy "github.com/flidai/leapview/internal/access/policy"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	"github.com/flidai/leapview/internal/analytics/dataquery"
	semanticquery "github.com/flidai/leapview/internal/analytics/query"
)

// A column-mask policy must reach the aggregate planner whenever the masked
// field is grouped or filtered by, and the planner must refuse the query: the
// masked column would otherwise be recoverable through group keys or a filter
// prefix oracle even though it is never projected.
func TestGovernedAggregateForwardsMaskAndPlannerRejectsMaskedGroupingAndFilters(t *testing.T) {
	_, _, semantic, physical, _ := canonicalGraph(t)
	expression := `{"field":"orders.region","mask":"redact"}`
	mask, err := accesspolicy.Compile("mask", "column_mask", expression)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := canonicalSnapshot(t, []struct {
		id         string
		resource   access.ResourceRef
		capability access.Capability
	}{{"semantic", semantic, access.CapabilityResourceUse}}, []accesssnapshot.DataPolicy{
		{ID: "mask", Resource: physical, PolicyType: "column_mask", ExpressionJSON: expression, Compiled: mask},
	})
	metrics := canonicalMetricsWithSnapshot(t, snapshot, nil)
	concrete, ok := metrics.concretePlanner("semantic_sales")
	if !ok {
		t.Fatal("semantic planner is unavailable")
	}

	prefix := dataquery.Filter{Field: "region", Operator: "starts_with", Values: []any{"E"}}
	tests := []struct {
		name    string
		fields  []dataquery.Field
		filters []dataquery.Filter
	}{
		{name: "grouped by masked dimension", fields: []dataquery.Field{{Field: "region"}}},
		{name: "filtered by masked dimension", filters: []dataquery.Filter{prefix}},
		{name: "grouped and filtered by masked dimension", fields: []dataquery.Field{{Field: "region"}}, filters: []dataquery.Filter{prefix}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := dataquery.SemanticAggregate("semantic_sales", "", test.fields, []dataquery.Field{{Field: "order_count"}}, test.filters, nil, 0, 0)
			request.ProjectID = canonicalProject
			request.Surface, request.Operation = dataquery.SurfaceDashboard, dataquery.OperationDashboardRows
			governed, _, err := metrics.GovernDataQuery(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			forwarded := false
			planned := semanticquery.Request{Metrics: []semanticquery.Field{{Field: "order_count"}}}
			for _, mask := range governed.ColumnMasks {
				forwarded = forwarded || (mask.Field == "orders.region" && mask.Mask == "redact")
				planned.ColumnMasks = append(planned.ColumnMasks, semanticquery.ColumnMask{Field: mask.Field, Mask: mask.Mask})
			}
			if !forwarded {
				t.Fatalf("governed masks = %#v, want orders.region redact", governed.ColumnMasks)
			}
			for _, field := range governed.Fields {
				planned.Dimensions = append(planned.Dimensions, semanticquery.Field{Field: field.Field, Alias: field.Alias})
			}
			for _, filter := range governed.Filters {
				if filter.Operator == "starts_with" {
					planned.Filters = append(planned.Filters, semanticquery.Filter{Field: filter.Field, Operator: filter.Operator, Values: filter.Values})
				}
			}
			if _, err := concrete.Plan(planned); err == nil || !strings.Contains(err.Error(), "masked") {
				t.Fatalf("governed masked aggregate planned, error = %v", err)
			}
		})
	}
}
