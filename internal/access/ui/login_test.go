package ui

import (
	"html"
	"strings"
	"testing"
)

func TestLoginPageUsesProductBranding(t *testing.T) {
	var output strings.Builder
	if err := LoginPage().Render(&output); err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(output.String())
	for _, expected := range []string{
		"<title>LeapView Login</title>",
		`<link rel="icon" href="/static/favicon.svg?v=dev" type="image/svg+xml">`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("document does not contain %q", expected)
		}
	}
}

func TestLoginBootstrapUsesProductName(t *testing.T) {
	page := LoginBootstrapSignalsForOptions(LoginPageOptions{})["page"].(LoginPageSignal)
	if page.Title != "LeapView" || page.Kind != "login" {
		t.Fatalf("login page signal = %#v", page)
	}
}

func TestLoginBootstrapExposesExplicitDevelopmentLogin(t *testing.T) {
	page := LoginBootstrapSignalsForOptions(LoginPageOptions{DevelopmentLogin: true})["page"].(LoginPageSignal)
	if !page.DevelopmentLogin {
		t.Fatalf("development login signal = %#v", page)
	}
}

func TestLoginPageCarriesOnlyClosedErrorCodesIntoUpdates(t *testing.T) {
	for _, test := range []struct {
		name      string
		errorCode string
		want      string
	}{
		{name: "invalid credentials", errorCode: "invalid_credentials", want: "error=invalid_credentials"},
		{name: "throttled", errorCode: "too_many_attempts", want: "error=too_many_attempts"},
		{name: "unknown", errorCode: "<script>alert(1)</script>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			if err := LoginPage(LoginPageOptions{ErrorCode: test.errorCode}).Render(&output); err != nil {
				t.Fatal(err)
			}
			rendered := html.UnescapeString(output.String())
			if test.want != "" && !strings.Contains(rendered, test.want) {
				t.Fatalf("document does not contain %q", test.want)
			}
			if strings.Contains(rendered, "script%3Ealert") || strings.Contains(rendered, "<script>alert") {
				t.Fatalf("document propagated an unknown login error: %s", rendered)
			}
		})
	}
}

func TestLoginPageDeclaresDescriptionWithoutCredentials(t *testing.T) {
	var output strings.Builder
	if err := LoginPage(LoginPageOptions{CSRFToken: "csrf-secret-token"}).Render(&output); err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(output.String())
	if got := strings.Count(rendered, `<meta name="description"`); got != 1 {
		t.Fatalf("document has %d description metas, want exactly 1: %s", got, rendered)
	}
	if !strings.Contains(rendered, `<meta name="description" content="Sign in to LeapView`) {
		t.Fatalf("description does not name the product sign-in: %s", rendered)
	}
	start := strings.Index(rendered, `<meta name="description"`)
	tag := rendered[start : start+strings.Index(rendered[start:], ">")]
	if strings.Contains(tag, "csrf-secret-token") {
		t.Fatalf("description leaks the CSRF token: %s", tag)
	}
}

func TestLoginPageDescriptionUsesConfiguredProductName(t *testing.T) {
	var output strings.Builder
	options := LoginPageOptions{}
	options.Presentation.ProductName = "Acme Insights"
	if err := LoginPage(options).Render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.UnescapeString(output.String()), `<meta name="description" content="Sign in to Acme Insights`) {
		t.Fatalf("description does not use configured product name: %s", output.String())
	}
}
