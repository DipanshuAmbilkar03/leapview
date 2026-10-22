package query

import (
	"strings"
	"testing"
)

func TestPlanAggregateRejectsMaskedDimensionFilterAndSort(t *testing.T) {
	statusMask := []ColumnMask{{Field: "orders.status", Mask: "redact"}}
	tests := []struct {
		name    string
		request Request
		want    string
	}{
		{
			name:    "physical dimension",
			request: Request{Dataset: "orders", Dimensions: []Field{{Field: "orders.status"}}, Metrics: []Field{{Field: "order_count"}}, ColumnMasks: statusMask},
			want:    `dimension "orders.status"`,
		},
		{
			name:    "mask field matches case-insensitively",
			request: Request{Dataset: "orders", Dimensions: []Field{{Field: "orders.status"}}, Metrics: []Field{{Field: "order_count"}}, ColumnMasks: []ColumnMask{{Field: " Orders.Status ", Mask: "null"}}},
			want:    `dimension "orders.status"`,
		},
		{
			name: "prefix oracle filter on unselected masked field",
			request: Request{
				Dataset: "orders", Metrics: []Field{{Field: "order_count"}},
				Filters:     []Filter{{Field: "orders.status", Dataset: "orders", Operator: "starts_with", Values: []any{"sh"}}},
				ColumnMasks: statusMask,
			},
			want: `filter field "orders.status"`,
		},
		{
			name: "prefix oracle filter beside masked dimension",
			request: Request{
				Dataset: "orders", Dimensions: []Field{{Field: "orders.status"}}, Metrics: []Field{{Field: "order_count"}},
				Filters:     []Filter{{Field: "orders.status", Operator: "starts_with", Values: []any{"sh"}}},
				ColumnMasks: statusMask,
			},
			want: "masked",
		},
		{
			name: "filter nested in a boolean group",
			request: Request{
				Dataset: "orders", Metrics: []Field{{Field: "order_count"}},
				Filters: []Filter{{Groups: []FilterGroup{
					{Filters: []Filter{{Field: "orders.customer_id", Operator: "equals", Values: []any{1}}}},
					{Filters: []Filter{{Field: "orders.status", Operator: "starts_with", Values: []any{"sh"}}}},
				}}},
				ColumnMasks: statusMask,
			},
			want: `filter field "orders.status"`,
		},
		{
			name: "spatial filter on masked coordinate",
			request: Request{
				Dataset: "orders", Metrics: []Field{{Field: "order_count"}},
				Filters:     []Filter{{Spatial: &SpatialFilter{Kind: "bounds", LatitudeField: "orders.latitude", LongitudeField: "orders.longitude", West: -1, South: -1, East: 1, North: 1}}},
				ColumnMasks: []ColumnMask{{Field: "orders.latitude", Mask: "zero"}},
			},
			want: `filter field "orders.latitude"`,
		},
		{
			name: "sort on masked dimension alias",
			request: Request{
				Dataset: "orders", Dimensions: []Field{{Field: "orders.status", Alias: "state_label"}}, Metrics: []Field{{Field: "order_count"}},
				Sort:        []Sort{{Field: "state_label", Direction: "asc"}},
				ColumnMasks: statusMask,
			},
			want: `dimension "orders.status"`,
		},
		{
			name: "semantic dimension bound to masked physical field",
			request: Request{
				Dataset: "orders", Dimensions: []Field{{Field: "customer_state"}}, Metrics: []Field{{Field: "order_count"}},
				ColumnMasks: []ColumnMask{{Field: "customers.state", Mask: "redact"}},
			},
			want: `dimension "customer_state"`,
		},
		{
			name: "semantic filter bound to masked physical field",
			request: Request{
				Dataset: "orders", Metrics: []Field{{Field: "order_count"}},
				Filters:     []Filter{{Field: "customer_state", Operator: "starts_with", Values: []any{"C"}}},
				ColumnMasks: []ColumnMask{{Field: "customers.state", Mask: "redact"}},
			},
			want: `filter field "customer_state"`,
		},
		{
			name: "masked time dimension",
			request: Request{
				Dataset: "orders", Dimensions: []Field{{Field: "activity_date", Grain: "day"}}, Metrics: []Field{{Field: "order_count"}},
				ColumnMasks: []ColumnMask{{Field: "orders.ordered_at", Mask: "null"}},
			},
			want: `dimension "activity_date"`,
		},
		{
			name: "multi-dataset conformed dimension masked in one branch",
			request: Request{
				Dimensions: []Field{{Field: "activity_date", Grain: "day"}}, Metrics: []Field{{Field: "order_count"}, {Field: "tag_count"}},
				ColumnMasks: []ColumnMask{{Field: "tags.tagged_at", Mask: "null"}},
			},
			want: `dimension "activity_date"`,
		},
		{
			name: "multi-dataset conformed filter masked in one branch",
			request: Request{
				Metrics:     []Field{{Field: "order_count"}, {Field: "tag_count"}},
				Filters:     []Filter{{Field: "customer_state", Operator: "starts_with", Values: []any{"C"}}},
				ColumnMasks: []ColumnMask{{Field: "customers.state", Mask: "redact"}},
			},
			want: `filter field "customer_state"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := mustNewCompiledPlanner(t, testModel()).Plan(test.request)
			if err == nil {
				t.Fatal("masked aggregate input was planned")
			}
			if !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "masked") {
				t.Fatalf("error = %v, want masked rejection containing %q", err, test.want)
			}
		})
	}
}

func TestPlanAggregateKeepsUnmaskedFieldsPlannable(t *testing.T) {
	plan, err := mustNewCompiledPlanner(t, testModel()).Plan(Request{
		Dataset: "orders", Dimensions: []Field{{Field: "orders.status"}}, Metrics: []Field{{Field: "order_count"}},
		Filters:     []Filter{{Field: "orders.customer_id", Operator: "equals", Values: []any{1}}},
		Sort:        []Sort{{Field: "status", Direction: "asc"}},
		ColumnMasks: []ColumnMask{{Field: "orders.email", Mask: "redact"}, {Field: "customers.state", Mask: "redact"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, `"status"`) {
		t.Fatalf("plan SQL = %s", plan.SQL)
	}
}

func TestPlanSpatialRejectsMaskedCoordinatesAndFilters(t *testing.T) {
	base := SpatialTileRequest{
		Dataset: "orders", Metrics: []Field{{Field: "revenue", Alias: "revenue"}},
		Latitude: Field{Field: "orders.latitude", Alias: "latitude"}, Longitude: Field{Field: "orders.longitude", Alias: "longitude"},
		Zoom: 4, TargetZoom: 6, MetatileX: 4, MetatileY: 8, MetatileSize: 4, CellPixels: 48, Buffer: 768,
	}
	planner := mustNewCompiledPlanner(t, testModel())

	masked := base
	masked.ColumnMasks = []ColumnMask{{Field: "orders.latitude", Mask: "zero"}}
	if _, err := planner.PlanSpatialTileAggregate(masked); err == nil || !strings.Contains(err.Error(), "masked") {
		t.Fatalf("masked coordinate aggregate tile error = %v", err)
	}

	filtered := base
	filtered.Filters = []Filter{{Field: "orders.status", Operator: "starts_with", Values: []any{"sh"}}}
	filtered.ColumnMasks = []ColumnMask{{Field: "orders.status", Mask: "redact"}}
	if _, err := planner.PlanSpatialTileAggregate(filtered); err == nil || !strings.Contains(err.Error(), "masked") {
		t.Fatalf("masked filter aggregate tile error = %v", err)
	}

	metadata := SpatialMetadataRequest{
		Dataset: "orders", Latitude: base.Latitude, Longitude: base.Longitude, Metrics: base.Metrics,
		Filters:    filtered.Filters,
		FeatureCap: 1, RawMinimumZoom: 0, MaximumZoom: 2, ColumnMasks: filtered.ColumnMasks,
	}
	if _, err := planner.PlanSpatialMetadata(metadata); err == nil || !strings.Contains(err.Error(), "masked") {
		t.Fatalf("masked filter metadata error = %v", err)
	}
}

func TestPlanBundleRejectsMaskedBranchBeforeSharing(t *testing.T) {
	_, err := mustNewCompiledPlanner(t, testModel()).PlanBundle([]BundleRequest{{
		ID: "masked",
		Request: Request{
			Dataset: "orders", Metrics: []Field{{Field: "order_count"}},
			Filters:     []Filter{{Field: "orders.status", Operator: "starts_with", Values: []any{"sh"}}},
			ColumnMasks: []ColumnMask{{Field: "orders.status", Mask: "redact"}},
		},
	}})
	if err == nil {
		t.Fatal("masked bundle branch was planned")
	}
}
