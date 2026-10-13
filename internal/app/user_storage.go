package app

import (
	"fmt"
	"net/http"

	accessmodule "github.com/flidai/leapview/internal/access/module"
	adminview "github.com/flidai/leapview/internal/admin/view"
	projecthttp "github.com/flidai/leapview/internal/project/http"
)

func storageAdministrator(access *accessmodule.Module) func(*http.Request) (bool, error) {
	return func(r *http.Request) (bool, error) {
		if access == nil {
			return false, fmt.Errorf("storage authorization is unavailable")
		}
		principal, ok := access.CurrentPrincipal(r)
		if !ok {
			return false, nil
		}
		return access.RequestPlatformAdmin(r.Context(), r, principal.ID)
	}
}

func userStorageAssets(browser *projecthttp.BrowserHandler) func(*http.Request) ([]adminview.StorageAsset, error) {
	return func(r *http.Request) ([]adminview.StorageAsset, error) {
		if browser == nil {
			return nil, fmt.Errorf("storage catalog is unavailable")
		}
		assets, err := browser.StorageAssets(r)
		if err != nil {
			return nil, err
		}
		rows := make([]adminview.StorageAsset, 0, len(assets))
		for _, asset := range assets {
			rows = append(rows, adminview.StorageAsset{Name: asset.Name, Kind: asset.Kind, Href: asset.Href})
		}
		return rows, nil
	}
}
