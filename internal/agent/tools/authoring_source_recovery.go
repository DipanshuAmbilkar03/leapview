package tools

import (
	"context"
	"errors"

	dashboardauthoring "github.com/flidai/leapview/internal/dashboard/authoring"
	authoringapplication "github.com/flidai/leapview/internal/dashboard/authoring/application"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	agentcore "github.com/flidai/leapview/pkg/agent"
)

const sourceEditRecoveryInstructions = "This source edit was rejected; its newText was not committed. Use currentSource when present, otherwise call read_dashboard_source again. Copy oldText exactly from the saved YAML, including indentation, and use its draftId and full revision token. Consult docs_search/docs_read for the dashboard YAML schema and correct the reported diagnostic field paths before retrying. Do not use rejected newText as oldText or create another dashboard. If bulk edits repeatedly fail, use add_dashboard_visual and assign_dashboard_field for charts, then make smaller source edits for filters and layout. Finish with a successful preview_dashboard_draft for the requested page."

// Source edit failures can be corrected within the same agent turn. Reading
// through the application rechecks current access and revision authority; an
// error never implies that the rejected candidate became the persisted draft.
func (p DashboardAuthoringProvider) sourceEditError(ctx context.Context, scope Scope, project projectgraph.ResourceID, id dashboardauthoring.DashboardID, err error) agentcore.ToolResult {
	result := authoringToolError(err)
	if !errors.Is(err, dashboardauthoring.ErrInvalidPayload) && !errors.Is(err, dashboardauthoring.ErrInvalidAuthoring) && !errors.Is(err, dashboardauthoring.ErrStaleRevision) {
		return result
	}
	recovery := struct {
		Instructions  string                           `json:"instructions"`
		CurrentSource *authoringapplication.SourceRead `json:"currentSource,omitempty"`
	}{Instructions: sourceEditRecoveryInstructions}
	source, readErr := p.Application.ReadSource(ctx, authoringapplication.DraftRequest{ProjectID: project, ActorID: scope.PrincipalID, DashboardID: id})
	// Large source documents retain the ordinary source-read path instead of
	// expanding a validation error beyond the model's bounded tool-result size.
	if readErr == nil && len(source.YAML) <= 32<<10 {
		recovery.CurrentSource = &source
	}
	result.ModelContent = map[string]any{"error": result.Content.(map[string]any)["error"], "recovery": recovery}
	return result
}
