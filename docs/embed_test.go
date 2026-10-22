package docs

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// The embedded tree is compiled into both the leapview and leapview-site binaries,
// so it must hold only publishable content: internal audit evidence and Go sources
// stay in the repository but out of the executables.
func TestEmbeddedFilesExcludeInternalAndSourceFiles(t *testing.T) {
	t.Parallel()

	var total int64
	err := fs.WalkDir(Files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if name == "internal" || strings.HasPrefix(name, "internal/") {
			t.Errorf("%s: internal documentation must not be embedded", name)
		}
		if path.Ext(name) == ".go" {
			t.Errorf("%s: Go sources must not be embedded", name)
		}
		switch path.Ext(name) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
			t.Errorf("%s: raster images must not be embedded; publish them from the site assets", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const budget = 16 << 20
	if total > budget {
		t.Fatalf("embedded documentation is %d bytes, over the %d byte budget", total, budget)
	}
}

func TestEmbeddedFilesIncludePublicContent(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"public-release.json", "desktop-release.json", "llms.txt", "navigation.yaml", "api/openapi.yaml", "getting-started.md", "visuals/index.md", "reference/agent-tools/index.md"} {
		if _, err := fs.Stat(Files, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	entries, err := fs.Glob(Files, "articles/*/*.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Error("articles are not embedded")
	}
}
