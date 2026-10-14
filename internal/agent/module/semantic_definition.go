package module

import (
	"context"

	agenttools "github.com/flidai/leapview/internal/agent/tools"
	semanticmodel "github.com/flidai/leapview/internal/analytics/model"
	semanticquery "github.com/flidai/leapview/internal/analytics/query"
)

func (m *Module) semanticDefinitionAccess(ctx context.Context, scope agenttools.Scope, ref agenttools.CatalogRef, model *semanticmodel.Model) (agenttools.SemanticDefinitionAccess, bool) {
	if m == nil || m.dashboardMetrics == nil || model == nil {
		return agenttools.SemanticDefinitionAccess{}, false
	}
	metrics, ok := m.dashboardMetrics(scope.ProjectID)
	if !ok || metrics == nil {
		return agenttools.SemanticDefinitionAccess{}, false
	}
	port, ok := metrics.(interface {
		SemanticConsumer(context.Context, string) (*semanticquery.SemanticAccessConsumer, error)
	})
	if !ok {
		return agenttools.SemanticDefinitionAccess{}, false
	}
	consumer, err := port.SemanticConsumer(ctx, ref.ID)
	if err != nil || consumer == nil || consumer.PrincipalID() != scope.PrincipalID || consumer.ModelID() != ref.ID ||
		consumer.Planner() == nil || consumer.Planner().CompiledModel() == nil || !consumer.Planner().CompiledModel().MatchesModel(model) {
		return agenttools.SemanticDefinitionAccess{}, false
	}
	assets, err := consumer.Assets()
	if err != nil {
		return agenttools.SemanticDefinitionAccess{}, false
	}
	visibility := agenttools.SemanticDefinitionAccess{
		Datasets: map[string]bool{}, Dimensions: map[string]bool{}, Metrics: map[string]bool{},
	}
	for _, asset := range assets {
		switch asset.Kind {
		case "dataset":
			visibility.Datasets[asset.Name] = true
		case "dimension":
			visibility.Dimensions[asset.Name] = true
		case "metric":
			visibility.Metrics[asset.Name] = true
		}
	}
	return visibility, true
}
