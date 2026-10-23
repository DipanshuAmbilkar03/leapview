package mcpoauth

import (
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

// ClientRegistration describes how an OAuth client came to exist. Neither
// route involves an operator vetting the client, so the consent page must
// not let a self-chosen client name stand in for identity.
type ClientRegistration string

const (
	// RegistrationDynamic is an RFC 7591 client registered anonymously through
	// /oauth/register. Its name and redirect URIs are entirely self-asserted.
	RegistrationDynamic ClientRegistration = "dynamic"
	// RegistrationMetadataDocument is a client identified by an HTTPS client
	// metadata document URL. The document host is attributable, but the
	// document is still self-asserted.
	RegistrationMetadataDocument ClientRegistration = "metadata_document"
)

// Consent is what the signed-in user is asked to approve.
type Consent struct {
	ClientID     string
	ClientName   string
	Scopes       []string
	Resource     string
	Registration ClientRegistration
	// MetadataHost is the host serving the client metadata document, set only
	// for RegistrationMetadataDocument clients.
	MetadataHost string
	// RedirectURI is the exact callback the authorization code will be sent to.
	RedirectURI string
	// RedirectHost is the callback's host (with port when present) in its
	// ASCII form, so look-alike internationalized names are visible.
	RedirectHost string
}

// Verified reports whether an operator has vouched for the client. No current
// registration route does, so every client is presented as unverified.
func (c Consent) Verified() bool { return false }

func buildConsent(clientID, clientName, redirectURI string, scopes []string, resource string) Consent {
	consent := Consent{
		ClientID: clientID, ClientName: clientName,
		Scopes:   append([]string(nil), scopes...),
		Resource: resource, Registration: RegistrationDynamic,
		RedirectURI: redirectURI, RedirectHost: asciiHost(redirectURI),
	}
	if strings.HasPrefix(clientID, "https://") {
		consent.Registration = RegistrationMetadataDocument
		consent.MetadataHost = asciiHost(clientID)
	}
	return consent
}

// asciiHost returns the lower-case host (including any port) of a URL with
// internationalized labels converted to punycode, or "" when it has none.
func asciiHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	hostname := strings.ToLower(parsed.Hostname())
	if ascii, err := idna.Lookup.ToASCII(hostname); err == nil {
		hostname = ascii
	}
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	if port := parsed.Port(); port != "" {
		return hostname + ":" + port
	}
	return hostname
}
