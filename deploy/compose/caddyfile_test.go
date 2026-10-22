package compose

import (
	"strings"
	"testing"
)

func TestShippedCaddyfilesDoNotForwardClientSuppliedAddressHeaders(t *testing.T) {
	for _, name := range []string{"Caddyfile", "Caddyfile.first-install-bootstrap"} {
		t.Run(name, func(t *testing.T) {
			lines := map[string]bool{}
			for _, line := range strings.Split(read(t, name), "\n") {
				lines[strings.TrimSpace(line)] = true
			}
			for _, required := range []string{
				"request_header -True-Client-IP",
				"request_header X-Real-IP {remote_host}",
				"request_header X-Forwarded-For {remote_host}",
				"reverse_proxy leapview:8080",
			} {
				if !lines[required] {
					t.Fatalf("%s is missing %q", name, required)
				}
			}
		})
	}
}
