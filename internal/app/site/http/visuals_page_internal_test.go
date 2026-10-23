package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVisualsPageShipsPlaceholdersForTheStreamedShowcase(t *testing.T) {
	charts, tables := visualShowcaseShape()
	if got, want := charts+len(tables), len(visualDocuments); got != want {
		t.Fatalf("shape accounts for %d cards, catalog documents %d", got, want)
	}
	if strings.Join(tables, " ") != "table matrix pivot" {
		t.Fatalf("tabular cards = %v, want table, matrix and pivot in catalog order", tables)
	}

	server := httptest.NewServer(NewHandler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/visuals")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, response)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	// The cards stream in after first paint. Without placeholders occupying their
	// final layout the footer jumps when they arrive (CLS 0.28 measured).
	if got, want := strings.Count(body, `class="chart skeleton"`), charts; got != want {
		t.Errorf("chart placeholders = %d, want %d", got, want)
	}
	for _, class := range []string{"table-card centered narrow skeleton", "table-card compact centered skeleton", "table-card compact wide skeleton"} {
		if got := strings.Count(body, `class="`+class+`"`); got != 1 {
			t.Errorf("%q placeholders = %d, want 1", class, got)
		}
	}
	if !strings.Contains(body, `<lv-site-visual-showcase><div class="site-showcase-placeholder" aria-busy="true">`) {
		t.Error("placeholders are not the showcase element's light DOM")
	}
	if got := strings.Count(body, `aria-hidden="true"><div class="visual-frame">`); got < charts+len(tables) {
		t.Errorf("placeholder cards must be hidden from assistive technology, got %d aria-hidden frames", got)
	}
}
