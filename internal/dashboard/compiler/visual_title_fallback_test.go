package compiler

import (
	"testing"

	visualizationir "github.com/flidai/leapview/internal/dashboard/visualization/ir"
)

func TestFallbackVisualTitle(t *testing.T) {
	t.Parallel()

	metric := func(id, label string) visualizationir.VisualizationField {
		return visualizationir.VisualizationField{ID: id, Role: visualizationir.VisualizationFieldRoleMetric, Label: label}
	}
	dimension := func(id, label string) visualizationir.VisualizationField {
		return visualizationir.VisualizationField{ID: id, Role: visualizationir.VisualizationFieldRoleDimension, Label: label}
	}
	authored := " Revenue versus baseline "
	blank := "  "
	tests := []struct {
		name     string
		title    *string
		id       string
		fields   []visualizationir.VisualizationField
		expected string
	}{
		{name: "authored title wins", title: &authored, id: "revenue_kpi", fields: []visualizationir.VisualizationField{metric("revenue", "Revenue")}, expected: " Revenue versus baseline "},
		{name: "single metric uses its display label", id: "total_orders", fields: []visualizationir.VisualizationField{metric("order_count", "Orders")}, expected: "Orders"},
		{name: "blank title is untitled", title: &blank, id: "total_orders", fields: []visualizationir.VisualizationField{metric("order_count", "Orders")}, expected: "Orders"},
		{name: "metric label equal to its raw id is not a label", id: "review_kpi", fields: []visualizationir.VisualizationField{metric("review_score", "review_score")}, expected: "Review kpi"},
		{name: "several fields fall back to a readable id", id: "orders-by-state", fields: []visualizationir.VisualizationField{dimension("state", "State"), metric("orders", "Orders")}, expected: "Orders by state"},
		{name: "no fields", id: "aov_kpi", expected: "Aov kpi"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := fallbackVisualTitle(test.title, test.id, test.fields); got != test.expected {
				t.Fatalf("fallbackVisualTitle() = %q, want %q", got, test.expected)
			}
		})
	}
}
