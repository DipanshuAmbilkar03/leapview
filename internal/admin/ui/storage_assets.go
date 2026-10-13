package ui

import (
	uisignals "github.com/flidai/leapview/internal/admin/ui/signals"
	adminview "github.com/flidai/leapview/internal/admin/view"
)

func userStorageAssetsGrid(assets []adminview.StorageAsset) adminRecordTable {
	rows := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		rows = append(rows, map[string]any{"name": asset.Name, "name_href": asset.Href, "kind": asset.Kind})
	}
	return adminRecordTable{
		Columns: []adminRecordTableColumn{
			{ID: "name", Header: "Name", Kind: uisignals.Pointer("link"), HrefKey: uisignals.Pointer("name_href")},
			{ID: "kind", Header: "Type"},
		},
		Rows: rows, Empty: "No data assets are available to you.",
	}
}

// Explicit nulls clear the previous projection when a reconnect changes the
// request's administrative authority.
func storagePagePayload(page uisignals.AdminPageSignal) any {
	return struct {
		uisignals.AdminPageSignal
		Storage  *uisignals.AdminStorageSignal          `json:"storage"`
		Metrics  *[]uisignals.AdminMetricSignal         `json:"metrics"`
		Sections *[]uisignals.AdminContentSectionSignal `json:"sections"`
	}{page, page.Storage, page.Metrics, page.Sections}
}
