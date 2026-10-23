package http

import (
	"regexp"
	"strings"
	"testing"
)

func TestDocumentationTablesScrollInsideNamedFocusableRegions(t *testing.T) {
	rendered, err := renderDocumentationMarkdown("articles/example.md", `# Example

## Flags

| Flag | Meaning |
| --- | --- |
| `+"`--a`"+` | First |

| Flag | Meaning |
| --- | --- |
| `+"`--b`"+` | Second |

## Fields & `+"`types`"+`

| Field | Meaning |
| --- | --- |
| id | Identifier |
`)
	if err != nil {
		t.Fatal(err)
	}

	regions := regexp.MustCompile(`<div class="site-docs-table-scroll" role="region" aria-label="([^"]*)" tabindex="0">\n<table>`).FindAllStringSubmatch(rendered, -1)
	if got := strings.Count(rendered, "<table"); got != 3 || len(regions) != 3 {
		t.Fatalf("%d tables in %d focusable regions, want every table wrapped:\n%s", got, len(regions), rendered)
	}
	if got, want := strings.Count(rendered, "</table>\n</div>"), 3; got != want {
		t.Errorf("closed %d wrappers, want %d", got, want)
	}
	seen := map[string]bool{}
	for _, region := range regions {
		if seen[region[1]] {
			t.Errorf("region label %q is not unique; screen-reader landmark lists would repeat it", region[1])
		}
		seen[region[1]] = true
	}
	for _, want := range []string{"Flags table 1 of 2", "Flags table 2 of 2", "Fields &amp; types table"} {
		if !seen[want] {
			t.Errorf("missing region label %q; got %v", want, seen)
		}
	}
}

func TestDocumentationTableWithoutHeadingStillGetsAName(t *testing.T) {
	rendered, err := renderDocumentationMarkdown("articles/example.md", "| A |\n| --- |\n| 1 |\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, `aria-label="Table"`) {
		t.Errorf("unnamed region:\n%s", rendered)
	}
}

func TestDocumentationLinksLeavingDocsPublishAsGitHubURLs(t *testing.T) {
	rendered, err := renderDocumentationMarkdown("articles/architecture/registry.md", "[ADR](../../../adr/0017-x.md) [page](sibling) [site](/docs/cli/build) [web](https://example.com/a.md)\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="https://github.com/flidai/leapview/blob/main/adr/0017-x.md"`,
		`href="sibling"`,
		`href="/docs/cli/build"`,
		`href="https://example.com/a.md"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered output missing %s:\n%s", want, rendered)
		}
	}
}

func TestGeneratedVisualAPIReferenceTableIsAFocusableRegion(t *testing.T) {
	reference, ok := visualReferenceForDocument("visuals/line")
	if !ok {
		t.Fatal("line visual has no generated API reference")
	}
	rendered := renderVisualAPIReference(reference)
	const wrapper = `<div class="site-docs-table-scroll" role="region" aria-label="API reference table" tabindex="0"><table`
	if !strings.Contains(rendered, wrapper) {
		t.Errorf("API reference table is not wrapped in a focusable scroll region:\n%.600s", rendered)
	}
}
