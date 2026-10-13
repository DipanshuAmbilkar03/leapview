package http

import (
	"fmt"
	"net/http"

	"github.com/flidai/leapview/internal/admin/ui"
)

func (m ReadModel) StorageData(r *http.Request) (ui.AdminData, error) {
	data := m.baseData(r)
	principal, ok := m.currentPrincipal(r)
	if !ok || principal.ID == "" {
		return data, fmt.Errorf("storage requires an authenticated principal")
	}
	administrator := principal.DevBypass
	if !administrator && m.StorageAdministrator != nil {
		var err error
		administrator, err = m.StorageAdministrator(r)
		if err != nil {
			return data, fmt.Errorf("storage is unavailable")
		}
	}
	if administrator {
		data.Storage = m.StorageService.Data(r.Context())
		return data, nil
	}
	if m.UserStorageAssets == nil {
		return data, fmt.Errorf("storage catalog is unavailable")
	}
	assets, err := m.UserStorageAssets(r)
	if err != nil {
		return data, err
	}
	data.UserStorageAssets = &assets
	return data, nil
}
