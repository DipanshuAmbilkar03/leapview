package http

import nethttp "net/http"

func (h Handler) Appearance(w nethttp.ResponseWriter, r *nethttp.Request) {
	if h.rejectAuthoringCredential(w, r) {
		return
	}
	h.renderPage(w, r, "appearance")
}
