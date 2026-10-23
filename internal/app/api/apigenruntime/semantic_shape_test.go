package apigenruntime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func semanticValuesJSON(count int) string {
	values := make([]string, count)
	for i := range values {
		values[i] = fmt.Sprintf(`"v%d"`, i)
	}
	return "[" + strings.Join(values, ",") + "]"
}

// nestedGroupsJSON returns one filter whose groups nest levels deep.
func nestedGroupsJSON(levels int) string {
	filter := `{"field":"leaf","operator":"equals","values":["x"]}`
	for range levels {
		filter = `{"groups":[{"filters":[` + filter + `]}]}`
	}
	return filter
}

func TestSemanticBodyFilterShapeLimits(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantField string // empty means accepted
	}{
		{"typical", `{"filters":[{"field":"region","operator":"in","values":["a","b"]}]}`, ""},
		{"in list at the cap", `{"filters":[{"field":"id","operator":"in","values":` + semanticValuesJSON(maxSemanticFilterValues) + `}]}`, ""},
		{"in list over the cap", `{"filters":[{"field":"a"},{"field":"id","operator":"in","values":` + semanticValuesJSON(maxSemanticFilterValues+1) + `}]}`, "filters[1].values"},
		{"nesting at the cap", `{"filters":[` + nestedGroupsJSON(maxSemanticFilterDepth-1) + `]}`, ""},
		{"nesting over the cap", `{"filters":[` + nestedGroupsJSON(maxSemanticFilterDepth) + `]}`, "filters[0].groups[0].filters[0].groups[0].filters[0].groups[0].filters[0].groups"},
		{"nested in list over the cap", `{"filters":[{"groups":[{"filters":[{"field":"id","operator":"in","values":` + semanticValuesJSON(maxSemanticFilterValues+1) + `}]}]}]}`, "filters[0].groups[0].filters[0].values"},
		{"null filters", `{"filters":null}`, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/semantic-models/sales/query", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			ok := validateSemanticBody(response, request)
			if test.wantField == "" {
				if !ok || response.Code != http.StatusOK {
					t.Fatalf("rejected valid body: %d %s", response.Code, response.Body.String())
				}
				return
			}
			if ok || response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.wantField) {
				t.Fatalf("ok=%v status=%d body=%s, want rejection naming %q", ok, response.Code, response.Body.String(), test.wantField)
			}
		})
	}
}

func TestSemanticBodyFilterNodeBudgetCoversNestedGroups(t *testing.T) {
	leaf := `{"field":"f","values":["x"]}`
	group := `{"groups":[{"filters":[` + strings.Repeat(leaf+",", 90) + leaf + `]}]}`
	body := `{"filters":[` + strings.Repeat(group+",", 5) + group + `]}` // 6 groups x 92 nodes, over the node cap
	request := httptest.NewRequest(http.MethodPost, "/api/v1/semantic-models/sales/query", strings.NewReader(body))
	response := httptest.NewRecorder()
	if validateSemanticBody(response, request) || response.Code != http.StatusBadRequest {
		t.Fatalf("filter tree over the node budget was accepted: %d", response.Code)
	}
}
