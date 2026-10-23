package mapassets

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/flidai/leapview/internal/dashboard/visualization/mapasset"
	"github.com/stretchr/testify/require"
)

func TestRootDoesNotDependOnWorkingDirectory(t *testing.T) {
	installation := t.TempDir()
	installed := installedPackage(t)
	require.NoError(t, os.MkdirAll(filepath.Join(installation, ".data"), 0o755))
	require.NoError(t, os.Rename(installed, filepath.Join(installation, ".data", "map-assets")))
	executable := filepath.Join(installation, "leapview-site")

	t.Chdir(t.TempDir())
	require.Equal(t, filepath.Join(installation, ".data", "map-assets"), resolveRoot("", executable))
	require.Equal(t, "/explicit", resolveRoot("/explicit", executable))
	require.Equal(t, defaultRoot, resolveRoot("", filepath.Join(t.TempDir(), "leapview-site")))

	t.Chdir(installation)
	require.Equal(t, defaultRoot, resolveRoot("", filepath.Join(t.TempDir(), "leapview-site")))
}

func TestMissingListsEveryAbsentFile(t *testing.T) {
	root := installedPackage(t)
	require.Empty(t, Missing(root))
	asset, err := mapasset.Resolve("streets")
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, "leapview-streets", "archives", mapasset.ArchiveSHA256, "basemap.pmtiles")))
	require.Equal(t, []string{"leapview-streets/archives/" + mapasset.ArchiveSHA256 + "/basemap.pmtiles"}, Missing(root))
	require.Len(t, Missing(t.TempDir()), len(mapasset.ExpectedFiles()))
	require.NotEmpty(t, asset.ArchiveURL)
}

func TestHandlerDoesNotInventArchivesThatAreNotInstalled(t *testing.T) {
	recorder := httptest.NewRecorder()
	servePackage(t.TempDir()).ServeHTTP(recorder, httptest.NewRequest("GET", "/map-assets/leapview-streets/archives/"+mapasset.ArchiveSHA256+"/basemap.pmtiles", nil))
	require.Equal(t, 404, recorder.Code)
}
