package page

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/platform/web/staticasset"
	g "maragu.dev/gomponents"
)

func TestRenderAppliesInjectedLayoutWithoutOwningItsSignal(t *testing.T) {
	layout := Layout{
		Presentation: Presentation{ProductName: "Product", FaviconPath: "/brand.svg"},
		Assets:       staticasset.New(staticasset.Config{Version: "release-123"}),
		Signal: struct {
			Name string `json:"name"`
		}{Name: "chrome"},
		Scripts: []string{"/shell.js"},
		Mount: func(content g.Node, attrs ...g.Node) g.Node {
			return g.El("product-shell", append(attrs, content)...)
		},
		ColorMode: "light_colorblind",
	}
	var output bytes.Buffer
	if err := Render(layout, Spec{
		UpdatesURL: "/updates", Scripts: []string{"/route.js"},
		Content: g.El("route-page"), ContentAttrs: []g.Node{g.Attr("slot", "page")},
	}).Render(&output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{"<title>Product</title>", `data-color-mode="light"`, `data-light-theme="light_colorblind"`, `data-theme-preference="light_colorblind"`, `href="/brand.svg?v=release-123"`, `src="/shell.js?v=release-123"`, `src="/route.js?v=release-123"`, `<product-shell slot="page"><route-page>`} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered document missing %q", want)
		}
	}
	signals := WithSignal(layout, map[string]any{"page": "route"})
	if signals["chrome"] != layout.Signal {
		t.Fatal("layout signal was not merged")
	}
}

func TestDocumentTitleIncludesCustomProductIdentity(t *testing.T) {
	if got := documentTitle("Admin - General", "Northstar Analytics"); got != "Admin - General · Northstar Analytics" {
		t.Fatalf("document title = %q", got)
	}
	if got := documentTitle("LeapView Dashboards", "Northstar Analytics"); got != "Northstar Analytics Dashboards" {
		t.Fatalf("branded document title = %q", got)
	}
}

func TestRenderIncludesInlineInitialStreamRecovery(t *testing.T) {
	layout := Layout{Assets: staticasset.New(staticasset.Config{Version: "test"})}
	var output bytes.Buffer
	if err := Render(layout, Spec{UpdatesURL: "/updates?route=data", Content: g.El("route-page")}).Render(&output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{
		`data-page-stream-recovery-root`,
		// The stream lifecycle is delegated to command.js, which is loaded on
		// every page, so every error type and status reaches the same logic.
		`data-on:datastar-fetch="evt.detail.el === el &amp;&amp; ($pageStreamRecovery = window.LeapViewCommand?.pageStreamRecovery?.(evt, $pageStreamRecovery)`,
		// Events that beat command.js still redirect an expired session and open the card.
		`evt.detail.argsRaw?.status === &#39;401&#39; ? (window.location.replace(&#39;/login?error=session_expired&#39;), false)`,
		`evt.detail.type === &#39;error&#39; || evt.detail.type === &#39;retries-failed&#39; ? true`,
		`data-page-stream-recovery`,
		`lv-page-stream-recovery`,
		`data-show="$pageStreamRecovery"`,
		`style="display:none"`,
		`role="alert"`,
		`aria-live="assertive"`,
		`aria-labelledby="page-stream-recovery-title"`,
		`data-page-stream-recovery-title`,
		`data-page-stream-recovery-message`,
		"Unable to load this page",
		">Retry</button>",
		`<script type="module" src="/static/command.js?v=test"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{
		`evt.detail.type === &#39;retrying&#39;`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("rendered page must not inline %q:\n%s", unwanted, body)
		}
	}
}

func TestRenderPlacesRecoveryCardBeforeRouteContent(t *testing.T) {
	layout := Layout{Assets: staticasset.New(staticasset.Config{Version: "test"})}
	var output bytes.Buffer
	if err := Render(layout, Spec{UpdatesURL: "/updates", Content: g.El("route-page")}).Render(&output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	if strings.Count(body, `data-page-stream-recovery-root=""`) != 1 || strings.Count(body, `data-page-stream-recovery=""`) != 1 {
		t.Fatalf("expected exactly one stream root and one recovery card:\n%s", body)
	}
	if strings.Index(body, `data-page-stream-recovery=""`) > strings.Index(body, "<route-page>") {
		t.Fatalf("recovery card must precede route content so it is independent of component upgrade:\n%s", body)
	}
}

func TestRenderForwardsHiddenStreamPolicy(t *testing.T) {
	layout := Layout{Assets: staticasset.New(staticasset.Config{Version: "test"})}
	for _, pause := range []bool{false, true} {
		name := "persistent"
		if pause {
			name = "pause when hidden"
		}
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if err := Render(layout, Spec{UpdatesURL: "/updates", PauseWhenHidden: pause}).Render(&output); err != nil {
				t.Fatal(err)
			}
			body := output.String()
			for _, lifecycle := range []string{"data-on:visibilitychange__document", "data-on:pagehide__window", "data-on:pageshow__window", "requestCancellation: el._pagestreamAbort"} {
				if strings.Contains(body, lifecycle) != pause {
					t.Errorf("lifecycle %q present = %v, want %v", lifecycle, strings.Contains(body, lifecycle), pause)
				}
			}
			if !pause && !strings.Contains(body, `data-init="@get(&#39;/updates&#39;, {openWhenHidden: true})"`) {
				t.Fatalf("default stream initialization changed:\n%s", body)
			}
		})
	}
}
