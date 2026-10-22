package transport

import "net/http"

// HeadFromGet answers HEAD with exactly the status and headers the GET handler
// produces, and no body. It is meant for read-only document routes that
// crawlers, uptime probes and link checkers commonly probe with HEAD; it must
// not wrap streaming handlers, because the wrapped writer deliberately does not
// implement http.Flusher.
//
// The handler runs as a GET so its authorization, redirects and cache policy
// stay the single source of truth. Every other method is passed through.
func HeadFromGet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		get := r.Clone(r.Context())
		get.Method = http.MethodGet
		next.ServeHTTP(headResponseWriter{ResponseWriter: w}, get)
	})
}

// headResponseWriter keeps headers and status but discards the body.
type headResponseWriter struct {
	http.ResponseWriter
}

func (headResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
