package mapassets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/dashboard/visualization/mapasset"
	"github.com/stretchr/testify/require"
)

func installedPackage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	style, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "static", "map-assets", "leapview-streets", "style.json"))
	require.NoError(t, err)
	for _, file := range mapasset.ExpectedFiles() {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		content := []byte("stub " + file.Path)
		switch {
		case strings.HasSuffix(file.Path, "/style.json"):
			content = style
		case strings.HasSuffix(file.Path, ".pmtiles"):
			content = []byte("PMTiles stub archive")
		}
		require.NoError(t, os.WriteFile(target, content, 0o644))
	}
	return root
}

func TestVerifyAcceptsCompleteDelivery(t *testing.T) {
	server := httptest.NewServer(servePackage(installedPackage(t)))
	defer server.Close()
	require.NoError(t, Verify(context.Background(), server.Client(), server.URL))
}

func TestVerifyReportsEveryUnservedFile(t *testing.T) {
	root := installedPackage(t)
	asset, err := mapasset.Resolve("streets")
	require.NoError(t, err)
	for _, served := range []string{asset.ArchiveURL, asset.SpriteURL + ".json", asset.SpriteURL + ".png"} {
		require.NoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(served, "/map-assets/")))))
	}
	server := httptest.NewServer(servePackage(root))
	defer server.Close()
	err = Verify(context.Background(), server.Client(), server.URL)
	require.Error(t, err)
	for _, missing := range []string{"basemap.pmtiles", "sprites/leapview.json", "sprites/leapview.png"} {
		require.ErrorContains(t, err, missing)
	}
	require.ErrorContains(t, err, "HTTP 404")
}

func TestVerifyRejectsWrongContentTypeAndStyleDigest(t *testing.T) {
	root := installedPackage(t)
	asset, err := mapasset.Resolve("streets")
	require.NoError(t, err)
	style := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(asset.StyleURL, "/map-assets/")))
	require.NoError(t, os.WriteFile(style, []byte(`{"version":8}`), 0o644))
	files := servePackage(root)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".png") {
			w.Header().Set("Content-Type", "text/html")
			return
		}
		files.ServeHTTP(w, r)
	}))
	defer server.Close()
	err = Verify(context.Background(), server.Client(), server.URL)
	require.ErrorContains(t, err, "Content-Type")
	require.ErrorContains(t, err, "style digest")
}

func TestVerifyRequiresRangeableArchive(t *testing.T) {
	files := servePackage(installedPackage(t))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("Range")
		files.ServeHTTP(w, r)
	}))
	defer server.Close()
	require.ErrorContains(t, Verify(context.Background(), server.Client(), server.URL), "HTTP 206")
}

func TestVerifyRejectsInvalidBaseURL(t *testing.T) {
	require.Error(t, Verify(context.Background(), nil, "leapview.dev"))
	require.Error(t, Verify(context.Background(), nil, "https://leapview.dev/docs"))
}

func TestPageReferencedURLsAreInPackageInventory(t *testing.T) {
	inventory := map[string]bool{}
	for _, file := range mapasset.ExpectedFiles() {
		inventory["/map-assets/"+file.Path] = true
	}
	asset, err := mapasset.Resolve("streets")
	require.NoError(t, err)
	for _, referenced := range []string{
		asset.StyleURL,
		asset.ArchiveURL,
		strings.NewReplacer("{fontstack}", "Noto%20Sans%20Regular", "{range}", "0-255").Replace(asset.GlyphsURL),
		asset.SpriteURL + ".json", asset.SpriteURL + ".png", asset.SpriteURL + "@2x.json", asset.SpriteURL + "@2x.png",
	} {
		require.True(t, mapasset.IsContentAddressedURLPath(referenced), referenced)
		require.True(t, inventory[strings.ReplaceAll(referenced, "%20", " ")], referenced)
	}
}

func TestSiteImageRunsFromTheDirectoryHoldingMapAssets(t *testing.T) {
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "Dockerfile.site"))
	require.NoError(t, err)
	stages := regexp.MustCompile(`(?m)^FROM .* AS runtime$`).Split(string(dockerfile), -1)
	require.Len(t, stages, 2, "Dockerfile.site must end in one runtime stage")
	runtime := stages[1]
	require.Contains(t, runtime, "COPY --from=sourcegen /src/.data/map-assets /.data/map-assets")
	require.Regexp(t, `(?m)^WORKDIR /$`, runtime, "the distroless nonroot base starts in /home/nonroot, where .data/map-assets does not exist")
	require.Less(t, strings.Index(runtime, "WORKDIR /"), strings.Index(runtime, "ENTRYPOINT"))
}
