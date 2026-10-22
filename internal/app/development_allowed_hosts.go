package app

import (
	"net"
	"net/url"
	"strings"

	"github.com/flidai/leapview/internal/app/config"
)

// developmentAllowedHosts is the Host allowlist used outside production when
// LEAPVIEW_ALLOWED_HOSTS is empty. An empty allowlist disables the middleware,
// and a development server with the auth bypass is a full-admin API, so DNS
// rebinding would let a hostile page reach it. Loopback names, the public URL
// host, and an explicit non-wildcard listen host stay reachable; anything else
// is configured with LEAPVIEW_ALLOWED_HOSTS.
func developmentAllowedHosts(cfg config.Config) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if parsed, err := url.Parse(strings.TrimSpace(cfg.PublicURL)); err == nil && parsed.Hostname() != "" {
		hosts = append(hosts, parsed.Hostname())
	}
	if listen, err := config.ParseListenAddr(cfg.ListenAddr()); err == nil {
		if ip := net.ParseIP(listen.Host); listen.Host != "" && (ip == nil || !ip.IsUnspecified()) {
			hosts = append(hosts, listen.Host)
		}
	}
	return hosts
}
