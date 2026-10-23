package mapassets

import (
	"net/http"

	mapassethttp "github.com/flidai/leapview/internal/dashboard/visualization/mapasset/http"
)

// servePackage serves root through the production map-asset boundary, exactly
// as the public site mounts it.
func servePackage(root string) http.Handler {
	return mapassethttp.CacheHandler(http.StripPrefix("/map-assets/", Handler(root)))
}
