package ui

import (
	"html"
	"strings"
	"testing"

	projectview "github.com/flidai/leapview/internal/project"
	catalog "github.com/flidai/leapview/internal/project/navigation"
	uisignals "github.com/flidai/leapview/internal/project/ui/signals"
	g "maragu.dev/gomponents"
)

func TestAssetPagePausesOnlyResumableDataSections(t *testing.T) {
	for _, test := range []struct {
		assetType projectview.AssetType
		section   string
		pause     bool
	}{
		{projectview.AssetTypeModel, "data", true},
		{projectview.AssetTypeSemanticModel, "data", true},
		{projectview.AssetTypeModel, "details", false},
		{projectview.AssetTypeModel, "definition", false},
		{projectview.AssetTypeModel, "versions", false},
		{projectview.AssetTypeSemanticModel, "details", false},
		{projectview.AssetTypeSemanticModel, "definition", false},
		{projectview.AssetTypeSource, "data", false},
		{projectview.AssetTypeSource, "details", false},
		{projectview.AssetTypeDashboard, "details", false},
	} {
		t.Run(string(test.assetType)+"/"+test.section, func(t *testing.T) {
			asset := projectview.DevelopAssetView{ID: "asset-1", Type: string(test.assetType), Key: "orders", Title: "Orders"}
			node := ProjectAssetPageWithRefreshAndVersionsForEnvironment(catalog.Catalog{}, projectview.DevelopView{ID: "project-1"}, asset, []projectview.DevelopAssetView{asset}, nil, test.section, "production", "", AssetRefreshState{}, AssetVersionsState{}, "csrf")
			assertProjectStreamLifecycle(t, node, "section="+test.section, test.pause)
		})
	}
}

func TestCollectionConnectionAndPipelinePagesKeepPersistentStreams(t *testing.T) {
	nav := catalog.Catalog{}
	project := projectview.DevelopView{ID: "project-1", Title: "Project"}
	connection := projectview.DevelopAssetView{ID: "connection-1", Type: string(projectview.AssetTypeConnection), Key: "warehouse", Title: "Warehouse"}
	for _, test := range []struct {
		name  string
		node  g.Node
		route string
	}{
		{"catalog", CatalogPageForQuery(nav, "initial query"), "route=catalog"},
		{"models", ProjectAreaPageWithContext(nav, project, nil, nil, nil, "models", "model", "initial query", "production", "", "csrf"), "route=data"},
		{"sources", ProjectAreaPageWithContext(nav, project, nil, nil, nil, "sources", "source", "initial query", "production", "", "csrf"), "route=data"},
		{"semantic models", ProjectAreaPageWithContext(nav, project, nil, nil, nil, "semantic-models", "semantic_model", "initial query", "production", "", "csrf"), "route=data"},
		{"connections", ConnectionsPageWithAdministrationForEnvironment(nav, project.ID, nil, nil, "initial query", "production", "", "csrf", ConnectionAdministrationView{}, ConnectionCommandBindings{}), "route=connections"},
		{"connection details", ConnectionAssetPageWithAdministrationForEnvironment(nav, project, connection, nil, nil, "details", "production", "", AssetVersionsState{}, ConnectionAdministrationView{}, ConnectionCommandBindings{}, "csrf", nil), "route=connection_asset"},
		{"pipelines", PipelinesPage(nav, PipelineMonitorState{}, "pipelines", ""), "route=pipelines"},
		{"pipeline run", PipelineRunDetailDocument(nav, uisignals.PipelineRunDetailPageSignal{PipelineID: "pipeline-1", RunID: "run-1", ActiveTab: "execution"}, "csrf"), "route=pipeline_run_detail"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertProjectStreamLifecycle(t, test.node, test.route, false)
		})
	}
}

func assertProjectStreamLifecycle(t *testing.T, node g.Node, route string, pause bool) {
	t.Helper()
	var output strings.Builder
	if err := node.Render(&output); err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(output.String())
	if !strings.Contains(rendered, route) {
		t.Fatalf("rendered page missing updates selector %q", route)
	}
	for _, attribute := range []string{"data-on:visibilitychange__document=", "data-on:pagehide__window=", "data-on:pageshow__window="} {
		if strings.Contains(rendered, attribute) != pause {
			t.Errorf("%s presence = %v, want %v", attribute, strings.Contains(rendered, attribute), pause)
		}
	}
	if pause {
		if !strings.Contains(rendered, `data-init="el._pagestreamAbort?.abort(); if (!document.hidden)`) {
			t.Fatal("data page did not use resumable initialization")
		}
	} else if !strings.Contains(rendered, `data-init="@get('/updates?`) || !strings.Contains(rendered, `, {openWhenHidden: true})"`) {
		t.Fatal("stateful page did not retain persistent initialization")
	}
}
