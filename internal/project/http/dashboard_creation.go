package http

import stdhttp "net/http"

// CanCreateDashboard shares the request-bound project permission used by
// dashboard creation routes with other browser affordances.
func (h *BrowserHandler) CanCreateDashboard(r *stdhttp.Request) bool {
	return h.dashboardCreationAllowed(r)
}
