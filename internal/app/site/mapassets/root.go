package mapassets

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/flidai/leapview/internal/dashboard/visualization/mapasset"
	siteassets "github.com/flidai/leapview/site"
)

const defaultRoot = ".data/map-assets"

// Root locates the installed basemap package without depending on the working
// directory the container runtime chose: an explicit root wins, then the
// development layout under the working directory, then the layout shipped
// beside the executable.
func Root(configured string) string {
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}
	return resolveRoot(strings.TrimSpace(configured), executable)
}

func resolveRoot(configured, executable string) string {
	if configured != "" {
		return configured
	}
	if isDirectory(defaultRoot) {
		return defaultRoot
	}
	if executable != "" {
		if beside := filepath.Join(filepath.Dir(executable), defaultRoot); isDirectory(beside) {
			return beside
		}
	}
	return defaultRoot
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Missing lists the package files absent from root. The embedded style keeps
// the site rendering without them, so a missing archive or sprite is otherwise
// invisible until browsers request it.
func Missing(root string) []string {
	var missing []string
	for _, file := range mapasset.ExpectedFiles() {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(file.Path))); err != nil || !info.Mode().IsRegular() {
			missing = append(missing, file.Path)
		}
	}
	return missing
}

// Handler serves paths relative to /map-assets/ from the installed package at
// root, falling back to the embedded fail-closed style.
func Handler(root string) http.Handler {
	installed := os.DirFS(root)
	disk := http.FileServer(http.FS(installed))
	embedded := http.FileServer(http.FS(siteassets.MapAssets()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" && fs.ValidPath(path) {
			if _, err := fs.Stat(installed, path); err == nil {
				disk.ServeHTTP(w, r)
				return
			}
		}
		embedded.ServeHTTP(w, r)
	})
}
