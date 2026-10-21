package explorehandoff

import (
	"strings"

	"github.com/flidai/leapview/internal/analytics/exploration"
	semanticmodel "github.com/flidai/leapview/internal/analytics/model"
	visualizationdefinition "github.com/flidai/leapview/internal/dashboard/visualization/definition"
)

// recordsSpecForVisual carries detail rows through the canonical contract so
// repeated field values never become a grouped aggregate during exploration.
func recordsSpecForVisual(definition visualizationdefinition.Definition, model *semanticmodel.Model) (exploration.ExplorationSpec, bool) {
	query := definition.Query
	if query.Kind != visualizationdefinition.QueryDetail || query.Detail == nil || model == nil || len(definition.SecondaryQueries) != 0 {
		return exploration.ExplorationSpec{}, false
	}
	detail := query.Detail
	datasetID, modelID := strings.TrimSpace(detail.TableID), strings.TrimSpace(query.ModelID)
	if datasetID == "" || modelID == "" || detail.Limit < 1 || detail.Limit > 1000 {
		return exploration.ExplorationSpec{}, false
	}
	mode := exploration.ExplorationQueryModeRecords
	spec := exploration.ExplorationSpec{
		SchemaVersion: 1, ModelID: modelID, DatasetID: &datasetID, Mode: &mode,
		Dimensions: make([]exploration.ExplorationDimensionRef, 0, len(detail.Fields)),
		Metrics:    []exploration.ExplorationMetricRef{}, Filters: []exploration.ExplorationFilter{},
		Sort: make([]exploration.ExplorationSort, 0, len(detail.DefaultSort)), Limit: int32(detail.Limit),
	}
	targets := map[string]string{}
	ambiguous := map[string]bool{}
	for _, field := range detail.Fields {
		if !validFieldBinding(field) || strings.TrimSpace(field.Grain) != "" {
			return exploration.ExplorationSpec{}, false
		}
		canonical, ok := canonicalExplorerDimensionID(model, datasetID, field.FieldID)
		if !ok {
			return exploration.ExplorationSpec{}, false
		}
		spec.Dimensions = append(spec.Dimensions, exploration.ExplorationDimensionRef{Field: canonical, Alias: optionalAlias(field.Alias)})
		for _, source := range []string{field.FieldID, field.Alias, canonical} {
			source = strings.TrimSpace(source)
			if previous, exists := targets[source]; exists && previous != canonical {
				ambiguous[source] = true
			}
			targets[source] = canonical
		}
	}
	for _, sort := range detail.DefaultSort {
		field := strings.TrimSpace(sort.FieldID)
		canonical, ok := targets[field]
		if !ok || ambiguous[field] {
			return exploration.ExplorationSpec{}, false
		}
		spec.Sort = append(spec.Sort, exploration.ExplorationSort{Field: canonical, Direction: exploration.ExplorationSortDirection(sort.Direction)})
	}
	if err := exploration.ValidateAgainstModel(model, &spec); err != nil {
		return exploration.ExplorationSpec{}, false
	}
	return spec, true
}

// WithSort accepts only a selected query field or its unambiguous output alias.
// The resulting spec still passes the normal governed handoff authorization.
func WithSort(spec exploration.ExplorationSpec, field, direction string) (exploration.ExplorationSpec, bool) {
	if direction != "asc" && direction != "desc" {
		return spec, false
	}
	matches := map[string]bool{}
	for _, dimension := range spec.Dimensions {
		if dimension.Field == field || (dimension.Alias != nil && *dimension.Alias == field) {
			matches[dimension.Field] = true
		}
	}
	for _, metric := range spec.Metrics {
		if metric.Field == field || (metric.Alias != nil && *metric.Alias == field) {
			matches[metric.Field] = true
		}
	}
	if len(matches) != 1 {
		return spec, false
	}
	for canonical := range matches {
		spec.Sort = []exploration.ExplorationSort{{Field: canonical, Direction: exploration.ExplorationSortDirection(direction)}}
	}
	return spec, true
}
