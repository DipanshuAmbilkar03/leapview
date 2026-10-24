package hostinstall

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestMaintenancePayloadTransition(t *testing.T) {
	compose, err := os.ReadFile("../../../../deploy/compose/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	current := string(compose)
	previous := strings.Replace(current, "[CMD, /usr/local/bin/leapview, healthcheck]", "[CMD, leapview, healthcheck]", 1)
	if previous == current {
		t.Fatal("fixture did not change the healthcheck command")
	}
	payload := func(compose string) map[string][]byte {
		return map[string][]byte{
			"compose.yaml": []byte(compose), "compose.https.yaml": []byte("proxy"),
			"compose.postgres.yaml":          []byte("postgres overlay"),
			"postgres/bundled-entrypoint.sh": []byte("entrypoint"), "postgres/bundled-init.sh": []byte("initializer"),
			"Caddyfile": []byte("caddy"), "deployment.env.example": []byte("settings"),
		}
	}
	tests := []struct {
		name      string
		change    func(map[string][]byte, map[string][]byte, map[string][]byte)
		wantError bool
	}{
		{"canonical healthcheck transition", nil, false},
		{"unchanged current payload", func(i, p, c map[string][]byte) {
			i["compose.yaml"] = c["compose.yaml"]
			p["compose.yaml"] = c["compose.yaml"]
		}, false},
		{"installed drift", func(i, p, c map[string][]byte) { i["compose.yaml"] = c["compose.yaml"] }, true},
		{"changed public binding", func(i, p, c map[string][]byte) {
			c["compose.yaml"] = []byte(strings.Replace(current, "127.0.0.1:8080", "0.0.0.0:8080", 1))
		}, true},
		{"changed volume", func(i, p, c map[string][]byte) {
			c["compose.yaml"] = []byte(strings.Replace(current, "leapview-state:/var/lib/leapview", "other-state:/var/lib/leapview", 1))
		}, true},
		{"changed health interval", func(i, p, c map[string][]byte) {
			c["compose.yaml"] = []byte(strings.Replace(current, "interval: 30s", "interval: 300s", 1))
		}, true},
		{"shell healthcheck", func(i, p, c map[string][]byte) {
			c["compose.yaml"] = []byte(strings.Replace(current, "[CMD, /usr/local/bin/leapview, healthcheck]", "[CMD-SHELL, 'true']", 1))
		}, true},
		{"missing healthcheck", func(i, p, c map[string][]byte) {
			c["compose.yaml"] = []byte(strings.Replace(current, "healthcheck:", "other:", 1))
		}, true},
		{"duplicate services", func(i, p, c map[string][]byte) { c["compose.yaml"] = []byte(current + "\nservices: {}\n") }, true},
		{"malformed YAML", func(i, p, c map[string][]byte) { c["compose.yaml"] = []byte("services: [") }, true},
		{"proxy changes", func(i, p, c map[string][]byte) { c["Caddyfile"] = []byte("changed") }, true},
		{"missing predecessor evidence", func(i, p, c map[string][]byte) { delete(p, "compose.yaml") }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i, p, c := payload(previous), payload(previous), payload(current)
			if tc.change != nil {
				tc.change(i, p, c)
			}
			err := validateMaintenancePayloadTransition(i, p, c)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

func TestRevision019MaintenancePayloadTransitionAdmitsBundledAdapterOnce(t *testing.T) {
	compose, err := os.ReadFile("../../../../deploy/compose/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	current := string(compose)
	previous := strings.Replace(current, "[CMD, /usr/local/bin/leapview, healthcheck]", "[CMD, leapview, healthcheck]", 1)
	if previous == current {
		t.Fatal("fixture did not model the exact predecessor healthcheck")
	}
	payload := func(compose string) map[string][]byte {
		return map[string][]byte{
			"compose.yaml": []byte(compose), "compose.https.yaml": []byte("proxy"),
			"Caddyfile": []byte("caddy"), "deployment.env.example": []byte("settings"),
		}
	}
	installed, predecessor, candidate := payload(previous), payload(previous), payload(current)
	for name, contents := range map[string][]byte{
		"compose.postgres.yaml":          []byte("bundled topology"),
		"postgres/bundled-entrypoint.sh": []byte("entrypoint"),
		"postgres/bundled-init.sh":       []byte("initializer"),
	} {
		candidate[name] = contents
	}
	if err := validateLegacyMaintenancePayloadTransition(installed, predecessor, candidate); err != nil {
		t.Fatalf("exact revision 019 transition rejected: %v", err)
	}
	delete(candidate, "postgres/bundled-init.sh")
	if err := validateLegacyMaintenancePayloadTransition(installed, predecessor, candidate); err == nil {
		t.Fatal("revision 019 transition accepted an incomplete bundled adapter payload")
	}
}

// reviewedCaddyfileFixtures pins the shipped proxy configuration and derives
// its exact predecessor by removing the #1048 (LV-02) client-address hardening.
func reviewedCaddyfileFixtures(t *testing.T) (predecessor, candidate []byte) {
	t.Helper()
	candidate, err := os.ReadFile("../../../../deploy/compose/Caddyfile")
	if err != nil {
		t.Fatal(err)
	}
	hardening := "  # Caddy is the only trusted proxy hop. LeapView keys rate limits on the\n" +
		"  # rightmost X-Forwarded-For entry, so never pass client-supplied address\n" +
		"  # headers through.\n" +
		"  request_header -True-Client-IP\n" +
		"  request_header X-Real-IP {remote_host}\n" +
		"  request_header X-Forwarded-For {remote_host}\n"
	predecessor = []byte(strings.Replace(string(candidate), hardening, "", 1))
	for _, fixture := range []struct {
		contents []byte
		want     string
	}{
		{predecessor, "ef29c362e9312da034c4420683051e19e252063709db932a1b29bc06c47c9114"},
		{candidate, "24ccb5e51eeb6755428710f227ed87a71ccd1898f9fe47ded299022c5f8ca8cc"},
	} {
		if got := fmt.Sprintf("%x", sha256.Sum256(fixture.contents)); got != fixture.want {
			t.Fatalf("Caddyfile fixture sha256 = %s, want %s", got, fixture.want)
		}
	}
	return predecessor, candidate
}

func TestMaintenancePayloadTransitionAdmitsOnlyReviewedCaddyfileLineage(t *testing.T) {
	predecessorCaddy, candidateCaddy := reviewedCaddyfileFixtures(t)
	compose, err := os.ReadFile("../../../../deploy/compose/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	payload := func(caddy []byte) map[string][]byte {
		return map[string][]byte{
			"compose.yaml": compose, "compose.https.yaml": []byte("proxy"),
			"compose.postgres.yaml":          []byte("postgres overlay"),
			"postgres/bundled-entrypoint.sh": []byte("entrypoint"), "postgres/bundled-init.sh": []byte("initializer"),
			"Caddyfile": caddy, "deployment.env.example": []byte("settings"),
		}
	}
	tests := []struct {
		name      string
		change    func(i, p, c map[string][]byte)
		wantError string
	}{
		{"reviewed client-address hardening", nil, ""},
		{"already hardened", func(i, p, c map[string][]byte) {
			i["Caddyfile"], p["Caddyfile"] = candidateCaddy, candidateCaddy
		}, ""},
		{"reverse transition", func(i, p, c map[string][]byte) {
			i["Caddyfile"], p["Caddyfile"], c["Caddyfile"] = candidateCaddy, candidateCaddy, predecessorCaddy
		}, "deployment topology changed: Caddyfile"},
		{"other candidate proxy change", func(i, p, c map[string][]byte) {
			c["Caddyfile"] = append(append([]byte{}, candidateCaddy...), "# extra\n"...)
		}, "deployment topology changed: Caddyfile"},
		{"unreviewed predecessor", func(i, p, c map[string][]byte) {
			i["Caddyfile"] = append(append([]byte{}, predecessorCaddy...), "# extra\n"...)
			p["Caddyfile"] = i["Caddyfile"]
		}, "deployment topology changed: Caddyfile"},
		{"installed drift", func(i, p, c map[string][]byte) {
			i["Caddyfile"] = candidateCaddy
		}, "installed deployment differs from predecessor payload: Caddyfile"},
		{"mixed proxy overlay change", func(i, p, c map[string][]byte) {
			c["compose.https.yaml"] = []byte("changed proxy")
		}, "deployment topology changed: compose.https.yaml"},
		{"mixed settings change", func(i, p, c map[string][]byte) {
			c["deployment.env.example"] = []byte("changed settings")
		}, "deployment topology changed: deployment.env.example"},
		{"mixed compose change", func(i, p, c map[string][]byte) {
			c["compose.yaml"] = []byte(strings.Replace(string(compose), "127.0.0.1:8080", "0.0.0.0:8080", 1))
		}, "deployment topology changed: compose.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i, p, c := payload(predecessorCaddy), payload(predecessorCaddy), payload(candidateCaddy)
			if tc.change != nil {
				tc.change(i, p, c)
			}
			err := validateMaintenancePayloadTransition(i, p, c)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("reviewed transition rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestLegacyMaintenancePayloadTransitionAdmitsReviewedCaddyfileLineage(t *testing.T) {
	predecessorCaddy, candidateCaddy := reviewedCaddyfileFixtures(t)
	payload := func(caddy []byte) map[string][]byte {
		return map[string][]byte{
			"compose.yaml":       []byte("services:\n  leapview:\n    healthcheck:\n      test: [CMD, /usr/local/bin/leapview, healthcheck]\n"),
			"compose.https.yaml": []byte("proxy"), "Caddyfile": caddy, "deployment.env.example": []byte("settings"),
		}
	}
	installed, predecessor, candidate := payload(predecessorCaddy), payload(predecessorCaddy), payload(candidateCaddy)
	for name, contents := range map[string][]byte{
		"compose.postgres.yaml":          []byte("bundled topology"),
		"postgres/bundled-entrypoint.sh": []byte("entrypoint"),
		"postgres/bundled-init.sh":       []byte("initializer"),
	} {
		candidate[name] = contents
	}
	if err := validateLegacyMaintenancePayloadTransition(installed, predecessor, candidate); err != nil {
		t.Fatalf("pinned historical predecessor rejected the reviewed Caddyfile: %v", err)
	}
	candidate["Caddyfile"] = []byte("changed")
	if err := validateLegacyMaintenancePayloadTransition(installed, predecessor, candidate); err == nil {
		t.Fatal("pinned historical predecessor accepted an unreviewed Caddyfile")
	}
}
