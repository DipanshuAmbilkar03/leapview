package http

import (
	stdhttp "net/http"

	"github.com/flidai/leapview/internal/agent"
)

func (h *Handler) canCreateDashboard(r *stdhttp.Request) bool {
	if h.options.CanCreateDashboard == nil || h.chatScope(r).Credential.Restricted {
		return false
	}
	return h.options.CanCreateDashboard(r)
}

func (h *Handler) authorizedBuilderToolScope(r *stdhttp.Request, scope agent.Scope, turnContext *agent.TurnContext) agent.Scope {
	resolved := builderToolScope(scope, turnContext)
	if resolved.MainChatAuthoring {
		resolved.MainChatAuthoring = h.canCreateDashboard(r)
	}
	return resolved
}
