package ui

import (
	"fmt"

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

func storagePageSignal(page uisignals.AdminPageSignal, data AdminData) uisignals.AdminPageSignal {
	page.HeaderTitle = "Storage"
	if data.UserStorageAssets != nil {
		page.HeaderDetail = "Browse the data assets you can access."
		page.Sections = uisignals.Pointer([]uisignals.AdminContentSectionSignal{{
			Title: "Data assets", Table: uisignals.Pointer(userStorageAssetsGrid(*data.UserStorageAssets)),
		}})
		return page
	}
	page.HeaderDetail = "Browse tables and views across schemas."
	page.Storage = uisignals.Pointer(AdminStorageSignalFromData(data.Storage))
	page.Metrics = uisignals.OptionalSlice([]uisignals.AdminMetricSignal{{
		Label: "Total data size", Value: data.Storage.TotalDataSizeLabel,
		Detail: uisignals.Optional(fmt.Sprintf("%d active files", data.Storage.DataFileCount)),
	}})
	return page
}

func storageDetailPageSignal(page uisignals.AdminPageSignal, data AdminData) uisignals.AdminPageSignal {
	page.HeaderTitle = "Storage"
	page.HeaderDetail = "Physical storage and active data files."
	if len(data.Storage.Tables) != 1 {
		page.Empty = uisignals.Pointer("Storage table not found.")
		return page
	}
	table := data.Storage.Tables[0]
	page.HeaderTitle = table.Name
	page.Metrics = uisignals.OptionalSlice([]uisignals.AdminMetricSignal{
		{Label: "Data size", Value: table.SizeLabel},
		{Label: "Active files", Value: fmt.Sprint(table.FileCount)},
		{Label: "Stored rows", Value: table.RowCountLabel},
		{Label: "Begin snapshot", Value: fmt.Sprint(table.BeginSnapshot)},
	})
	page.Sections = uisignals.OptionalSlice([]uisignals.AdminContentSectionSignal{
		{Title: "Storage", Facts: uisignals.Pointer([]uisignals.DefinitionFactSignal{
			{Label: "Schema", Value: table.Schema},
			{Label: "Object type", Value: table.Type},
			{Label: "DuckLake path", Value: table.DuckLakePath},
			{Label: "Table UUID", Value: table.TableUUID},
		})},
		{Title: "Active files", Table: uisignals.Pointer(adminStorageFilesGrid(table.Files))},
	})
	return page
}
