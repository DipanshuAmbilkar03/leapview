package module

import (
	"net/http"
	"strings"

	apitransport "github.com/flidai/leapview/internal/platform/http/transport"
)

// writeAuthorizationStatus reports an authentication or authorization outcome
// that carries no body of its own. Public API paths receive the shared
// problem+json document so clients can parse every /api/v1 failure the same
// way; browser and command paths keep their plain status text. The cause is
// never part of the response.
func writeAuthorizationStatus(w http.ResponseWriter, r *http.Request, status int) {
	if r == nil || r.URL == nil || !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		http.Error(w, http.StatusText(status), status)
		return
	}
	code := "AUTHORIZATION_DENIED"
	switch {
	case status == http.StatusUnauthorized:
		code = "AUTHENTICATION_REQUIRED"
	case status == http.StatusNotFound:
		code = "RESOURCE_NOT_FOUND"
	case status >= http.StatusInternalServerError:
		code = "AUTHORIZATION_UNAVAILABLE"
	}
	apitransport.WriteProblem(w, r, status, code, http.StatusText(status), nil)
}
