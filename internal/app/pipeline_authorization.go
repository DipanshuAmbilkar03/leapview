package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/flidai/leapview/internal/access"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

func pipelineBrowserAuthorizer(accessModule canonicalAccessModule, runtimeHost canonicalRuntimeHost, projectIDForRequest func(context.Context) (projectgraph.ResourceID, error)) func(*http.Request, string, access.Capability) (bool, error) {
	return func(r *http.Request, pipelineID string, capability access.Capability) (bool, error) {
		var action access.Action
		switch capability {
		case access.CapabilityResourceRead:
			action = access.ActionPipelineRead
		case access.CapabilityResourceUse:
			action = access.ActionPipelineRun
		default:
			return false, fmt.Errorf("unsupported pipeline browser capability %q", capability)
		}
		principal, ok := accessModule.CurrentPrincipal(r)
		if !ok {
			return false, nil
		}
		projectID, err := projectIDForRequest(r.Context())
		if err != nil {
			return false, err
		}
		resource, err := access.NewResourceRef(projectgraph.ResourceID(pipelineID), projectgraph.KindPipeline)
		if err != nil {
			return false, err
		}
		allowed, authErr := authorizeProjectResources(r.Context(), accessModule, runtimeHost, principal.ID, projectID, []access.ResourceRef{resource}, action)
		return allowed, authErr
	}
}
