// Package doclinks decides how a link written in authored documentation
// Markdown is published on the public site.
//
// Authors write links that work where the Markdown is read in the repository,
// such as ../../../adr/0017-example.md. The public site serves only /docs
// pages, so a link that leaves docs/ must become a GitHub URL, and a link to
// another Markdown source must be written as a /docs page link. The runtime
// renderer applies the rewrite; the catalog generator uses the same decision
// to fail the build when a link cannot resolve.
package doclinks

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// RepositoryURL is the public source repository.
const RepositoryURL = "https://github.com/flidai/leapview"

// Kind says how a documentation link is published.
type Kind int

const (
	// Unchanged links are external, absolute site paths, fragments, or
	// URL-relative links between documentation pages.
	Unchanged Kind = iota
	// Repository links point at a file or directory outside docs/ and are
	// published as GitHub URLs.
	Repository
)

// Link is the publication decision for one link destination.
type Link struct {
	Kind Kind
	// RepoPath is the slash-separated repository path of a Repository link.
	RepoPath string
	// Destination is the href to publish.
	Destination string
}

// Classify resolves destination as written in the Markdown file source, which
// is a slash-separated path relative to docs/. It returns an error for a link
// that would be a 404 on the public site.
func Classify(source, destination string) (Link, error) {
	unchanged := Link{Kind: Unchanged, Destination: destination}
	if destination == "" || strings.HasPrefix(destination, "#") || strings.HasPrefix(destination, "/") {
		return unchanged, nil
	}
	parsed, err := url.Parse(destination)
	if err != nil {
		return Link{}, fmt.Errorf("invalid link %q: %w", destination, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.Path == "" {
		return unchanged, nil
	}

	resolved := path.Join(path.Dir("docs/"+source), parsed.Path)
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return Link{}, fmt.Errorf("link %q leaves the repository", destination)
	}
	if resolved == "docs" || strings.HasPrefix(resolved, "docs/") {
		if path.Ext(resolved) == ".md" {
			return Link{}, fmt.Errorf("link %q targets a Markdown source file, which is not published; link the /docs page instead", destination)
		}
		return unchanged, nil
	}

	view := "blob"
	if strings.HasSuffix(parsed.Path, "/") || path.Ext(resolved) == "" {
		view = "tree"
	}
	published := RepositoryURL + "/" + view + "/main/" + (&url.URL{Path: resolved}).EscapedPath()
	if parsed.RawQuery != "" {
		published += "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		published += "#" + parsed.EscapedFragment()
	}
	return Link{Kind: Repository, RepoPath: resolved, Destination: published}, nil
}
