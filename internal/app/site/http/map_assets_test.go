package http

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/app/site/mapassets"
	"github.com/flidai/leapview/internal/dashboard/visualization/mapasset"
	"github.com/stretchr/testify/require"
)

func TestSiteServesCompleteMapAssetPackageThroughHandler(t *testing.T) {
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
	server := httptest.NewServer(NewHandlerWithOptions(Options{MapAssetRoot: root}))
	defer server.Close()
	require.NoError(t, mapassets.Verify(context.Background(), server.Client(), server.URL))
}
