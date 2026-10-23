// Package docs owns authored and generated Markdown for the public documentation site.
package docs

import "embed"

// Files contains the unified catalog, search index, authored articles, generated
// references, downloadable schemas, and API contract. Patterns are explicit so
// docs/internal (audit evidence and screenshots) and Go sources stay out of the
// leapview and leapview-site binaries; embed_test.go enforces that boundary.
//
//go:embed *.md *.json *.txt *.yaml api articles guides reference visuals
var Files embed.FS
