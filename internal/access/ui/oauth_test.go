package ui

import (
	"html"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access/http/mcpoauth"
	webpage "github.com/flidai/leapview/internal/platform/web/page"
	"github.com/flidai/leapview/internal/platform/web/staticasset"
)

func TestOAuthConsentPageUsesProductBranding(t *testing.T) {
	var output strings.Builder
	if err := OAuthConsentPage(mcpoauth.Consent{ClientName: "Agent", Resource: "https://example.test"}, nil, "csrf",
		webpage.Presentation{ProductName: "LeapView", FaviconPath: "/static/favicon.svg"}, staticasset.Resolver{}).Render(&output); err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(output.String())
	for _, expected := range []string{
		"<title>Authorize MCP access · LeapView</title>",
		`<link rel="icon" href="/static/favicon.svg?v=dev" type="image/svg+xml">`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("document does not contain %q", expected)
		}
	}
}

func renderConsent(t *testing.T, consent mcpoauth.Consent) string {
	t.Helper()
	var output strings.Builder
	if err := OAuthConsentPage(consent, nil, "csrf",
		webpage.Presentation{ProductName: "LeapView", FaviconPath: "/static/favicon.svg"}, staticasset.Resolver{}).Render(&output); err != nil {
		t.Fatal(err)
	}
	return html.UnescapeString(output.String())
}

func TestOAuthConsentPageShowsRedirectHostAndUnverifiedWarningForDynamicClients(t *testing.T) {
	rendered := renderConsent(t, mcpoauth.Consent{
		ClientID: "mcp_client_abc", ClientName: "Claude", Resource: "https://leapview.test/mcp",
		Registration: mcpoauth.RegistrationDynamic,
		RedirectURI:  "https://evil.example/callback", RedirectHost: "evil.example",
	})
	for _, expected := range []string{
		"Unverified application",
		"evil.example",
		"https://evil.example/callback",
		`role="group"`,
		`aria-labelledby="consent-warning-title"`,
		`id="consent-warning-title"`,
		`aria-describedby="consent-warning-body"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("document does not contain %q\n%s", expected, rendered)
		}
	}
	if strings.Index(rendered, "Unverified application") > strings.Index(rendered, `name="decision"`) {
		t.Error("the unverified warning must precede the decision buttons")
	}
}

func TestOAuthConsentPageNamesMetadataHostForMetadataDocumentClients(t *testing.T) {
	rendered := renderConsent(t, mcpoauth.Consent{
		ClientID: "https://client.example/oauth/client.json", ClientName: "CIMD Client", Resource: "https://leapview.test/mcp",
		Registration: mcpoauth.RegistrationMetadataDocument, MetadataHost: "client.example",
		RedirectURI: "http://127.0.0.1:33418/cb", RedirectHost: "127.0.0.1:33418",
	})
	for _, expected := range []string{"Unverified application", "client.example", "127.0.0.1:33418"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("document does not contain %q", expected)
		}
	}
}

func TestOAuthConsentPageEscapesClientControlledText(t *testing.T) {
	var output strings.Builder
	consent := mcpoauth.Consent{
		ClientName: "<script>alert(1)</script>", Registration: mcpoauth.RegistrationDynamic,
		RedirectURI: "https://evil.example/cb?q=<b>", RedirectHost: "evil.example",
	}
	if err := OAuthConsentPage(consent, nil, "csrf", webpage.Presentation{ProductName: "LeapView"}, staticasset.Resolver{}).Render(&output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "<script>alert(1)") || strings.Contains(output.String(), "<b>") {
		t.Fatalf("client-controlled text was not escaped: %s", output.String())
	}
}
