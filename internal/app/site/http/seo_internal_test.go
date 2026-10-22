package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSitePagesDeclareSocialPreviewImage(t *testing.T) {
	baseURL, err := url.Parse("https://leapview.dev")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHandlerWithOptions(Options{BaseURL: baseURL}))
	defer server.Close()

	for _, path := range []string{"/", "/download", "/visuals", "/docs", "/docs/introduction", "/docs/search", "/no-such-page"} {
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		body := readBody(t, response)
		response.Body.Close()
		for _, want := range []string{
			`<meta property="og:image" content="https://leapview.dev/static/product-dashboard-light.png">`,
			`<meta name="twitter:image" content="https://leapview.dev/static/product-dashboard-light.png">`,
			`<meta name="twitter:card" content="summary_large_image">`,
			`<meta property="og:image:alt" content="`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing %q", path, want)
			}
		}
	}
}

func TestSitemapLastModifiedComesFromBuildTime(t *testing.T) {
	for _, test := range []struct {
		name      string
		buildTime string
		want      string
	}{
		{name: "commit time", buildTime: "2026-10-21T23:30:00-05:00", want: "<lastmod>2026-10-22</lastmod>"},
		{name: "unset", buildTime: ""},
		{name: "malformed", buildTime: "yesterday"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(NewHandlerWithOptions(Options{BuildTime: test.buildTime}))
			defer server.Close()
			response, err := server.Client().Get(server.URL + "/sitemap.xml")
			if err != nil {
				t.Fatal(err)
			}
			sitemap := readBody(t, response)
			response.Body.Close()
			if test.want == "" {
				if strings.Contains(sitemap, "<lastmod>") {
					t.Fatalf("sitemap invented a lastmod:\n%s", sitemap)
				}
				return
			}
			if got, want := strings.Count(sitemap, test.want), strings.Count(sitemap, "<url>"); got != want || want == 0 {
				t.Fatalf("lastmod on %d of %d urls:\n%s", got, want, sitemap)
			}
		})
	}
}

func TestRobotsAllowsCrawlersToReadSearchNoindex(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	robots := readBody(t, response)
	response.Body.Close()
	for _, line := range strings.Split(robots, "\n") {
		if strings.TrimSpace(line) == "Disallow: /docs/search" {
			t.Fatalf("robots.txt blocks /docs/search, which hides its noindex tag:\n%s", robots)
		}
	}
	if !strings.Contains(robots, "Disallow: /docs/search/") {
		t.Fatalf("robots.txt should keep crawlers off the search fragment endpoint:\n%s", robots)
	}
}

func TestDocumentationMCPLinkOpensDocsInBrowsers(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || !strings.HasPrefix(response.Header.Get("Location"), "/docs/guides/integrate/agent-tools") {
		t.Fatalf("browser GET /mcp = %d %q, want redirect to the agent tools guide", response.StatusCode, response.Header.Get("Location"))
	}

	request, err = http.NewRequest(http.MethodGet, server.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusSeeOther {
		t.Fatalf("MCP client GET /mcp was redirected")
	}
}

func TestDownloadStatusUsesElementsThatMayCarryAlertRoles(t *testing.T) {
	for _, status := range []string{"preparing", "withdrawn"} {
		var page strings.Builder
		if err := desktopReleaseStatus(desktopReleaseManifest{Status: status}).Render(&page); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(page.String(), "<aside") {
			t.Errorf("%s status renders an aside with a live-region role:\n%s", status, page.String())
		}
		if !strings.Contains(page.String(), `role="`) {
			t.Errorf("%s status lost its live-region role", status)
		}
	}
}

func TestSiteOffersTheHostedDemo(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, response)
	response.Body.Close()
	if got := strings.Count(body, `href="https://demo.leapview.dev"`); got < 2 {
		t.Fatalf("home page links the demo %d times, want header and footer entries", got)
	}
	if !strings.Contains(body, "Try the demo") {
		t.Fatal("home page does not name the demo entry")
	}
}
