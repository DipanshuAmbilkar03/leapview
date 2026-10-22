package doclinks

import "testing"

func TestClassifyRewritesRepositoryRelativeLinksToGitHub(t *testing.T) {
	const source = "articles/architecture/semantic-attribute-registry.md"
	for _, test := range []struct {
		name        string
		destination string
		want        Link
	}{
		{
			name:        "decision record",
			destination: "../../../adr/0017-adopt-a-looker-aligned-semantic-access-contract.md",
			want: Link{
				Kind:        Repository,
				RepoPath:    "adr/0017-adopt-a-looker-aligned-semantic-access-contract.md",
				Destination: "https://github.com/flidai/leapview/blob/main/adr/0017-adopt-a-looker-aligned-semantic-access-contract.md",
			},
		},
		{
			name:        "fragment is preserved",
			destination: "../../../internal/app/cli/README.md#flags",
			want: Link{
				Kind:        Repository,
				RepoPath:    "internal/app/cli/README.md",
				Destination: "https://github.com/flidai/leapview/blob/main/internal/app/cli/README.md#flags",
			},
		},
		{
			name:        "directory",
			destination: "../../../adr/specifications/",
			want: Link{
				Kind:        Repository,
				RepoPath:    "adr/specifications",
				Destination: "https://github.com/flidai/leapview/tree/main/adr/specifications",
			},
		},
		{name: "sibling page stays URL-relative", destination: "plan-build-publish", want: Link{Kind: Unchanged, Destination: "plan-build-publish"}},
		{name: "absolute site path", destination: "/docs/cli/build", want: Link{Kind: Unchanged, Destination: "/docs/cli/build"}},
		{name: "external", destination: "https://example.com/a.md", want: Link{Kind: Unchanged, Destination: "https://example.com/a.md"}},
		{name: "mail", destination: "mailto:security@example.com", want: Link{Kind: Unchanged, Destination: "mailto:security@example.com"}},
		{name: "same-page fragment", destination: "#install", want: Link{Kind: Unchanged, Destination: "#install"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Classify(source, test.destination)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("Classify(%q) = %+v, want %+v", test.destination, got, test.want)
			}
		})
	}
}

func TestClassifyRejectsLinksThatCannotResolveOnThePublicSite(t *testing.T) {
	for _, test := range []struct {
		source      string
		destination string
	}{
		{"articles/a.md", "../../../../outside-the-repository.md"},
		{"articles/a.md", "b.md"},
		{"articles/a.md", "../guides/cli/targets.md"},
	} {
		if got, err := Classify(test.source, test.destination); err == nil {
			t.Errorf("Classify(%q, %q) = %+v, want an error", test.source, test.destination, got)
		}
	}
}
