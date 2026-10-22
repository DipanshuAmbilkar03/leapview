package mcpoauth

import "testing"

func TestBuildConsentIdentifiesDynamicallyRegisteredClientRedirectHost(t *testing.T) {
	consent := buildConsent("mcp_client_abc", "Claude", "https://Evil.Example:8443/callback?x=1", []string{ScopeMCPUse}, "https://leapview.test/mcp")

	if consent.Registration != RegistrationDynamic {
		t.Fatalf("Registration = %q, want %q", consent.Registration, RegistrationDynamic)
	}
	if consent.Verified() {
		t.Fatal("dynamically registered client must not be verified")
	}
	if consent.RedirectHost != "evil.example:8443" {
		t.Fatalf("RedirectHost = %q", consent.RedirectHost)
	}
	if consent.RedirectURI != "https://Evil.Example:8443/callback?x=1" {
		t.Fatalf("RedirectURI = %q", consent.RedirectURI)
	}
	if consent.ClientName != "Claude" || consent.ClientID != "mcp_client_abc" {
		t.Fatalf("consent = %#v", consent)
	}
}

func TestBuildConsentRecordsMetadataDocumentHost(t *testing.T) {
	consent := buildConsent("https://client.example/oauth/client.json", "CIMD Client", "http://127.0.0.1:33418/cb", []string{ScopeMCPUse}, "https://leapview.test/mcp")

	if consent.Registration != RegistrationMetadataDocument {
		t.Fatalf("Registration = %q, want %q", consent.Registration, RegistrationMetadataDocument)
	}
	if consent.MetadataHost != "client.example" {
		t.Fatalf("MetadataHost = %q", consent.MetadataHost)
	}
	if consent.RedirectHost != "127.0.0.1:33418" {
		t.Fatalf("RedirectHost = %q", consent.RedirectHost)
	}
	if consent.Verified() {
		t.Fatal("a self-asserted client metadata document is not operator verification")
	}
}

func TestBuildConsentExposesPunycodeForInternationalizedRedirectHosts(t *testing.T) {
	consent := buildConsent("mcp_client_abc", "Claude", "https://clаude.example/cb", nil, "https://leapview.test/mcp")

	if consent.RedirectHost != "xn--clude-5ve.example" {
		t.Fatalf("RedirectHost = %q, want punycode form", consent.RedirectHost)
	}
}
