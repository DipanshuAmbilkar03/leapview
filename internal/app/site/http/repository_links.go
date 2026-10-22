package http

import (
	"github.com/flidai/leapview/internal/app/site/doclinks"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// documentationSourceKey carries the docs/-relative path of the Markdown file
// being rendered, which relative links are resolved against.
var documentationSourceKey = parser.NewContextKey()

// repositoryLinkTransformer publishes links that leave docs/ (for example to
// an ADR) as GitHub URLs. Resolved against the site they would be 404s. The
// catalog generator rejects links that cannot be published, so a failure to
// classify a link here leaves it untouched.
type repositoryLinkTransformer struct{}

func (repositoryLinkTransformer) Transform(document *ast.Document, _ text.Reader, context parser.Context) {
	source, _ := context.Get(documentationSourceKey).(string)
	if source == "" {
		return
	}
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if link, ok := node.(*ast.Link); ok && entering {
			if published, err := doclinks.Classify(source, string(link.Destination)); err == nil && published.Kind == doclinks.Repository {
				link.Destination = []byte(published.Destination)
			}
		}
		return ast.WalkContinue, nil
	})
}
