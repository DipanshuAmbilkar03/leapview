package hostinstall

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// The installed configuration must still be the immutable predecessor payload.
// The application healthcheck may evolve to the current product command. The
// Caddyfile and bundled PostgreSQL templates may follow the exact reviewed
// lineages below;
// all other topology remains equal before any capture or migration begins.
func validateMaintenancePayloadTransition(installed, predecessor, candidate map[string][]byte) error {
	if err := validateMaintenanceBasePayloadTransition(installed, predecessor, candidate); err != nil {
		return err
	}
	changed := ""
	for _, name := range []string{
		"compose.postgres.yaml", "postgres/bundled-entrypoint.sh", "postgres/bundled-init.sh",
	} {
		if len(predecessor[name]) == 0 || len(candidate[name]) == 0 || !bytes.Equal(installed[name], predecessor[name]) {
			return fmt.Errorf("installed deployment differs from predecessor payload: %s", name)
		}
		if changed == "" && !bytes.Equal(predecessor[name], candidate[name]) {
			changed = name
		}
	}
	if changed != "" && !reviewedBundledPostgresTemplateTransition(predecessor, candidate) {
		return fmt.Errorf("deployment topology changed: %s", changed)
	}
	return nil
}

// Admit only the reviewed forward lineage: #978 adds KILL and NOLOGIN
// readonly/backup roles; the next template replaces the AMD64-only image pin
// with the PostgreSQL 18.6 native AMD64/ARM64 index. Whole-file tuple hashes
// retain the exact historical payloads and reject mixed or reverse changes.
// This does not replace the separately managed PostgreSQL container or relax
// its profile identity; native maintenance resumes that existing container.
func reviewedBundledPostgresTemplateTransition(predecessor, candidate map[string][]byte) bool {
	const entrypoint = "3135934604ed71add4b116e8cab53dcb95c369ef0b732f813fa5456bb0e60d43"
	const rolesInitializer = "6654a03defecce45df62eb4701020aedf3f11ae6eddc24053bd5cb48807d5905"
	legacy := [3]string{"1447caf2bc58c6a2e59f6d836a1c9e3b183618b70a9fd5070361512fdbabb685", entrypoint, "81cdf13f0e2a69d2a5674a21a7db298c7eafd98d57be767764c5f73b075bb3c8"}
	roles := [3]string{"f52715c337985c5948a02b2daa8a78596f3bcefb13b0c434d8aba51dd5ee5ae8", entrypoint, rolesInitializer}
	native := [3]string{"64d212275ab3f00b2bdae23fc9261896a94dc880b0e25fbb2f650a406c62b486", entrypoint, rolesInitializer}
	var before, after [3]string
	for i, name := range [...]string{"compose.postgres.yaml", "postgres/bundled-entrypoint.sh", "postgres/bundled-init.sh"} {
		before[i] = fmt.Sprintf("%x", sha256.Sum256(predecessor[name]))
		after[i] = fmt.Sprintf("%x", sha256.Sum256(candidate[name]))
	}
	for _, transition := range [...][2][3]string{{legacy, roles}, {legacy, native}, {roles, native}} {
		if before == transition[0] && after == transition[1] {
			return true
		}
	}
	return false
}

// Admit only the reviewed forward proxy lineage: #1048 (LV-02) makes Caddy,
// the only trusted hop, drop client-supplied True-Client-IP and overwrite
// X-Real-IP and X-Forwarded-For with the connecting address, so the login rate
// limiter cannot be keyed on forged headers. Every predecessor that native
// maintenance supports (the pinned revision 019 and legacy public images, and
// later releases with the bundled PostgreSQL adapter) ships the ef29c362 file
// before #1048; the earlier ca113dd3 file predates all of them. The change only
// narrows which request headers reach the application; it does not alter the
// site address, TLS, ports or upstream. Whole-file hashes reject reverse,
// partial or further proxy changes. Cutover activates the candidate file
// through the current generation link and recreates the public proxy.
func reviewedCaddyfileTransition(predecessor, candidate []byte) bool {
	const clientAddressHeaders = "ef29c362e9312da034c4420683051e19e252063709db932a1b29bc06c47c9114"
	const trustedClientAddress = "24ccb5e51eeb6755428710f227ed87a71ccd1898f9fe47ded299022c5f8ca8cc"
	return fmt.Sprintf("%x", sha256.Sum256(predecessor)) == clientAddressHeaders &&
		fmt.Sprintf("%x", sha256.Sum256(candidate)) == trustedClientAddress
}

func validateMaintenanceBasePayloadTransition(installed, predecessor, candidate map[string][]byte) error {
	for _, name := range []string{
		"compose.yaml", "compose.https.yaml", "Caddyfile", "deployment.env.example",
	} {
		if len(predecessor[name]) == 0 || len(candidate[name]) == 0 || !bytes.Equal(installed[name], predecessor[name]) {
			return fmt.Errorf("installed deployment differs from predecessor payload: %s", name)
		}
		if name == "compose.yaml" {
			if err := validateMaintenanceCompose(predecessor[name], candidate[name]); err != nil {
				return fmt.Errorf("deployment topology changed: %s: %w", name, err)
			}
		} else if name == "Caddyfile" {
			if !bytes.Equal(predecessor[name], candidate[name]) && !reviewedCaddyfileTransition(predecessor[name], candidate[name]) {
				return fmt.Errorf("deployment topology changed: %s", name)
			}
		} else if !bytes.Equal(predecessor[name], candidate[name]) {
			return fmt.Errorf("deployment topology changed: %s", name)
		}
	}
	return nil
}

// validateLegacyMaintenancePayloadTransition is limited to the exact
// pinned historical images that predate the optional bundled PostgreSQL
// payload. It verifies the old installed topology and admits the adapter files
// as new immutable candidate payload only on those predecessor transitions.
func validateLegacyMaintenancePayloadTransition(installed, predecessor, candidate map[string][]byte) error {
	for _, name := range []string{"compose.postgres.yaml", "postgres/bundled-entrypoint.sh", "postgres/bundled-init.sh"} {
		if len(installed[name]) != 0 || len(predecessor[name]) != 0 || len(candidate[name]) == 0 {
			return fmt.Errorf("historical PostgreSQL adapter payload is invalid: %s", name)
		}
	}
	return validateMaintenanceBasePayloadTransition(
		installed,
		predecessor,
		candidate,
	)
}

func validateMaintenanceCompose(predecessor, candidate []byte) error {
	var before, after map[string]any
	if err := yaml.Unmarshal(predecessor, &before); err != nil {
		return fmt.Errorf("invalid predecessor Compose: %w", err)
	}
	if err := yaml.Unmarshal(candidate, &after); err != nil {
		return fmt.Errorf("invalid candidate Compose: %w", err)
	}
	healthcheck := func(document map[string]any) (map[string]any, error) {
		for _, key := range []string{"services", "leapview", "healthcheck"} {
			child, ok := document[key].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("missing application healthcheck mapping: %s", key)
			}
			document = child
		}
		return document, nil
	}
	oldHealth, err := healthcheck(before)
	if err != nil {
		return err
	}
	newHealth, err := healthcheck(after)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(newHealth["test"], []any{"CMD", "/usr/local/bin/leapview", "healthcheck"}) {
		return fmt.Errorf("candidate must use the canonical application healthcheck")
	}
	delete(oldHealth, "test")
	delete(newHealth, "test")
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("configuration differs outside the application healthcheck command")
	}
	return nil
}
