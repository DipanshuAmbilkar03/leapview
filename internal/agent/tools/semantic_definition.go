package tools

import semanticmodel "github.com/flidai/leapview/internal/analytics/model"

// Protected definitions expose authorized semantic members without the raw
// expressions and physical source bindings that can reveal protected fields.
func semanticModelDefinitionWithAccess(model *semanticmodel.Model, visibility SemanticDefinitionAccess) map[string]any {
	definition := map[string]any{"name": model.Name, "title": model.Title, "description": model.Description}
	datasets := map[string]any{}
	for name, dataset := range model.Datasets {
		if !visibility.Datasets[name] {
			continue
		}
		value := compactSemanticDataset(dataset)
		delete(value, "model")
		if !visibility.Dimensions[dataset.DefaultTimeDimension] {
			delete(value, "defaultTimeDimension")
		}
		datasets[name] = value
	}
	dimensions := map[string]any{}
	for name, dimension := range model.Dimensions {
		if visibility.Dimensions[name] {
			dimensions[name] = compactSemanticDimension(dimension)
		}
	}
	metrics := map[string]any{}
	for name, metric := range model.Metrics {
		if metric.Hidden || !visibility.Metrics[name] {
			continue
		}
		value := compactSemanticMetric(metric)
		delete(value, "input")
		delete(value, "expression")
		delete(value, "where")
		for key, dependency := range map[string]string{"numerator": metric.Numerator, "denominator": metric.Denominator} {
			member, found := model.Metrics[dependency]
			if !found || member.Hidden || !visibility.Metrics[dependency] {
				delete(value, key)
			}
		}
		if !visibility.Datasets[metric.Dataset] {
			delete(value, "dataset")
		}
		if !visibility.Dimensions[metric.TimeDimension] {
			delete(value, "timeDimension")
		}
		value["definitionRedacted"] = true
		metrics[name] = value
	}
	if len(datasets) > 0 {
		definition["datasets"] = datasets
	}
	if len(dimensions) > 0 {
		definition["dimensions"] = dimensions
	}
	if len(metrics) > 0 {
		definition["metrics"] = metrics
	}
	return definition
}

func conservativeProtectedDefinition(model *semanticmodel.Model) map[string]any {
	definition := semanticModelDefinition(model)
	visibility := SemanticDefinitionAccess{Datasets: map[string]bool{}, Dimensions: map[string]bool{}, Metrics: map[string]bool{}}
	for kind, names := range map[string]map[string]bool{
		"datasets": visibility.Datasets, "dimensions": visibility.Dimensions, "metrics": visibility.Metrics,
	} {
		members, _ := definition[kind].(map[string]any)
		for name := range members {
			names[name] = true
		}
	}
	return semanticModelDefinitionWithAccess(model, visibility)
}
