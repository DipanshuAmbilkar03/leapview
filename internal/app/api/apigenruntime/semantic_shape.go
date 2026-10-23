package apigenruntime

import (
	"encoding/json"
	"fmt"
)

// Shape limits for semantic query filters. The array-length caps on the request
// are enforced separately; these bound what a single filter can fan out to so
// a request inside the byte limit cannot plan an arbitrarily large predicate.
const (
	// maxSemanticFilterValues caps one filter's value list (an IN list).
	maxSemanticFilterValues = 1000
	// maxSemanticFilterDepth caps filter-group nesting; top-level filters are depth 1.
	maxSemanticFilterDepth = 4
	// maxSemanticFilterNodes caps filters across the whole tree.
	maxSemanticFilterNodes = 500
)

type semanticFilterShape struct {
	Values []json.RawMessage `json:"values"`
	Groups []struct {
		Filters []semanticFilterShape `json:"filters"`
	} `json:"groups"`
}

// validateSemanticFilterShape checks the filters array of a semantic request.
// It returns the offending field path and a client-facing detail. Bodies it
// cannot decode are accepted here because the domain decoder owns type errors.
func validateSemanticFilterShape(raw json.RawMessage) (field, detail string, ok bool) {
	var filters []semanticFilterShape
	if err := json.Unmarshal(raw, &filters); err != nil {
		return "", "", true
	}
	nodes := 0
	return walkSemanticFilters(filters, "filters", 1, &nodes)
}

func walkSemanticFilters(filters []semanticFilterShape, path string, depth int, nodes *int) (string, string, bool) {
	for index, filter := range filters {
		filterPath := fmt.Sprintf("%s[%d]", path, index)
		if *nodes++; *nodes > maxSemanticFilterNodes {
			return "filters", fmt.Sprintf("must contain at most %d filters including nested groups", maxSemanticFilterNodes), false
		}
		if len(filter.Values) > maxSemanticFilterValues {
			return filterPath + ".values", fmt.Sprintf("must contain at most %d values", maxSemanticFilterValues), false
		}
		if len(filter.Groups) > 0 && depth >= maxSemanticFilterDepth {
			return filterPath + ".groups", fmt.Sprintf("filter groups may be nested at most %d levels deep", maxSemanticFilterDepth), false
		}
		for groupIndex, group := range filter.Groups {
			groupPath := fmt.Sprintf("%s.groups[%d].filters", filterPath, groupIndex)
			if field, detail, ok := walkSemanticFilters(group.Filters, groupPath, depth+1, nodes); !ok {
				return field, detail, false
			}
		}
	}
	return "", "", true
}
