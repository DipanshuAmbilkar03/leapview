package ui

import (
	"net/url"
	"sort"
	"strings"

	"github.com/flidai/leapview/internal/access/http/mcpoauth"
	webpage "github.com/flidai/leapview/internal/platform/web/page"
	"github.com/flidai/leapview/internal/platform/web/staticasset"
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func OAuthConsentPage(consent mcpoauth.Consent, values url.Values, csrfToken string, presentation webpage.Presentation, assets staticasset.Resolver) g.Node {
	if strings.TrimSpace(presentation.ProductName) == "" {
		presentation.ProductName = "Application"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hidden := g.Group{}
	for _, key := range keys {
		if key == "decision" || key == "gorilla.csrf.Token" {
			continue
		}
		for _, value := range values[key] {
			hidden = append(hidden, h.Input(h.Type("hidden"), h.Name(key), h.Value(value)))
		}
	}
	hidden = append(hidden, h.Input(h.Type("hidden"), h.Name("gorilla.csrf.Token"), h.Value(csrfToken)))
	return c.HTML5(c.HTML5Props{
		Title: "Authorize MCP access · " + presentation.ProductName, Language: "en",
		Head: g.Group{
			h.Link(h.Rel("icon"), h.Href(assets.URL(presentation.FaviconPath)), h.Type("image/svg+xml")),
			h.Link(h.Rel("stylesheet"), h.Href(assets.URL("/static/app.css"))),
		},
		Body: g.Group{
			h.Main(h.Class("min-h-svh bg-app text-fg-default flex items-center justify-center p-6"),
				h.Section(h.Class("w-full max-w-lg rounded-xl border border-border-default bg-canvas-default p-6 shadow-lg"),
					h.H1(h.Class("text-xl font-semibold"), g.Text("Authorize MCP access")),
					h.P(h.Class("mt-3 text-sm text-fg-muted"), consentIntro(consent, presentation.ProductName)),
					consentWarning(consent, presentation.ProductName),
					consentDetails(consent),
					h.Form(h.Method("post"), h.Action("/oauth/authorize"), h.Class("mt-6 flex justify-end gap-3"),
						hidden,
						h.Button(h.Type("submit"), h.Name("decision"), h.Value("deny"), h.Class("btn"), g.Text("Cancel")),
						h.Button(h.Type("submit"), h.Name("decision"), h.Value("approve"), h.Class("btn btn-accent"), g.Text("Authorize")),
					),
				),
			),
		},
	})
}

func consentIntro(consent mcpoauth.Consent, productName string) g.Node {
	request := " is requesting permission to use " + productName + " tools as your signed-in account."
	if consent.Verified() {
		return g.Text(consent.ClientName + request)
	}
	// The name is the client's own claim, so say so instead of presenting it
	// as an identity.
	return g.Text("An application calling itself “" + consent.ClientName + "”" + request)
}

// consentWarning explains, before the decision buttons, that the requesting
// application is self-described. Colour is never the only signal: the
// warning is a labelled group with its own heading and description.
func consentWarning(consent mcpoauth.Consent, productName string) g.Node {
	if consent.Verified() {
		return nil
	}
	destination := consent.RedirectHost
	if destination == "" {
		destination = "the application"
	}
	origin := "Anyone can register an application under any name, including one you recognize. "
	if consent.Registration == mcpoauth.RegistrationMetadataDocument && consent.MetadataHost != "" {
		origin = "This application identifies itself with a document hosted at " + consent.MetadataHost + ". "
	}
	return h.Div(h.Role("group"),
		g.Attr("aria-labelledby", "consent-warning-title"), g.Attr("aria-describedby", "consent-warning-body"),
		h.Class("mt-5 rounded-md border border-control-border-warning bg-panel-muted p-4"),
		h.H2(h.ID("consent-warning-title"), h.Class("text-sm font-semibold text-fg-default"), g.Text("Unverified application")),
		h.P(h.ID("consent-warning-body"), h.Class("mt-1 text-sm text-fg-default"),
			g.Text(origin+productName+" has not verified it. After you authorize, the result is sent to "),
			h.Strong(h.Class("font-mono break-all"), g.Text(destination)),
			g.Text(". Continue only if you started this connection from that site or app."),
		),
	)
}

func consentDetails(consent mcpoauth.Consent) g.Node {
	row := func(term string, value ...g.Node) g.Node {
		return h.Div(h.Class("mt-3 first:mt-0"),
			h.Dt(h.Class("text-xs font-medium text-fg-muted"), g.Text(term)),
			h.Dd(append([]g.Node{h.Class("mt-0.5 text-sm text-fg-default break-words")}, value...)...),
		)
	}
	rows := g.Group{row("Application", g.Text(consent.ClientName))}
	if consent.RedirectHost != "" {
		rows = append(rows, row("Returns to",
			h.Span(h.Class("font-mono font-semibold break-all"), g.Text(consent.RedirectHost)),
			h.P(h.Class("mt-0.5 text-xs text-fg-muted font-mono break-all"), g.Text(consent.RedirectURI)),
		))
	}
	rows = append(rows,
		row("Permission", g.Text("Use governed read-only BI tools. Resource capabilities are checked for every call.")),
		row("Resource", h.Span(h.Class("font-mono break-all"), g.Text(consent.Resource))),
	)
	return h.Dl(h.Class("mt-5 rounded-md border border-line-muted bg-panel-muted p-4"), rows)
}
