package exploration

import "testing"

func TestExplorationSpecHasSelection(t *testing.T) {
	tests := []struct {
		name string
		spec *ExplorationSpec
		want bool
	}{
		{"nil", nil, false},
		{"empty", &ExplorationSpec{Dimensions: []ExplorationDimensionRef{}, Metrics: []ExplorationMetricRef{}}, false},
		{"filters and sort only", &ExplorationSpec{Filters: []ExplorationFilter{{Field: "orders.status"}}, Sort: []ExplorationSort{{Field: "x"}}}, false},
		{"dimension", &ExplorationSpec{Dimensions: []ExplorationDimensionRef{{Field: "orders.status"}}}, true},
		{"metric", &ExplorationSpec{Metrics: []ExplorationMetricRef{{Field: "revenue"}}}, true},
		{"time", &ExplorationSpec{Time: &ExplorationTimeSelection{Field: "orders.created_at", Grain: "day"}}, true},
	}
	for _, test := range tests {
		if got := test.spec.HasSelection(); got != test.want {
			t.Errorf("%s: HasSelection() = %t, want %t", test.name, got, test.want)
		}
	}
}
