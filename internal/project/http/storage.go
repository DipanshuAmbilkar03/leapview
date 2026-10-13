package http

import (
	"fmt"
	stdhttp "net/http"
	"strings"

	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

// StorageAsset exposes authorized catalog metadata without physical statistics,
// connection configuration, or storage locations.
type StorageAsset struct {
	Name string
	Kind string
	Href string
}

func (h *BrowserHandler) StorageAssets(r *stdhttp.Request) ([]StorageAsset, error) {
	if h == nil || h.CurrentUser == nil || r == nil {
		return nil, fmt.Errorf("storage catalog is unavailable")
	}
	principal, ok := h.CurrentUser(r)
	if !ok || strings.TrimSpace(principal.ID) == "" {
		return nil, fmt.Errorf("storage requires an authenticated principal")
	}
	projectID, err := h.boundProject(r.Context())
	if err != nil {
		return nil, fmt.Errorf("storage catalog is unavailable")
	}
	page, err := listCatalogAll(r.Context(), h.Catalog, principal.ID, principal.DevBypass,
		[]projectgraph.Kind{projectgraph.KindSource, projectgraph.KindModel, projectgraph.KindSemanticModel},
		h.currentCredential(r), projectID)
	if err != nil {
		return nil, fmt.Errorf("storage catalog is unavailable")
	}
	assets := make([]StorageAsset, 0, len(page.Items))
	for _, item := range page.Items {
		kind := "Source"
		switch item.Ref.Kind {
		case projectgraph.KindModel:
			kind = "Model"
		case projectgraph.KindSemanticModel:
			kind = "Semantic model"
		case projectgraph.KindSource:
		default:
			continue
		}
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = item.Name
		}
		assets = append(assets, StorageAsset{Name: name, Kind: kind, Href: productSearchHref(item)})
	}
	return assets, nil
}
