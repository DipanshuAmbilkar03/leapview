//go:build linux

package hostinstall

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/platform/ociref"
	"github.com/stretchr/testify/require"
)

// These fixtures reconstruct the exact #978 delta. Independent hashes bind
// them to live source 62ad1a4 and reviewed candidate source a96e7f9.
func postgresTemplatePayloads(t *testing.T) (map[string][]byte, map[string][]byte) {
	t.Helper()
	before, after := map[string][]byte{}, map[string][]byte{}
	for _, name := range []string{"compose.yaml", "compose.https.yaml", "Caddyfile", "deployment.env.example", "compose.postgres.yaml", "postgres/bundled-entrypoint.sh", "postgres/bundled-init.sh"} {
		data, err := os.ReadFile(filepath.Join("../../../../deploy/compose", name))
		require.NoError(t, err)
		before[name], after[name] = data, data
	}
	// Keep the historical #978 fixture exact when the current template moves
	// to the native AMD64/ARM64 index. The independent hashes below still bind
	// both historical payloads, not a reconstruction with today's image pin.
	after["compose.postgres.yaml"] = []byte(strings.Replace(string(after["compose.postgres.yaml"]),
		"sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873",
		"sha256:63bdc97d67b5133bf0e5ebd500bec6d046fa851dc81340d838f0347e616107e8", 1))
	before["compose.postgres.yaml"] = []byte(strings.Replace(string(after["compose.postgres.yaml"]), "    # The root init process must forward stop signals after PostgreSQL drops UID.\n    cap_add: [CHOWN, DAC_OVERRIDE, KILL, SETGID, SETUID]", "    cap_add: [CHOWN, DAC_OVERRIDE, SETGID, SETUID]", 1))
	init := string(after["postgres/bundled-init.sh"])
	for _, role := range []string{"leapview_control_readonly", "leapview_control_backup"} {
		init = strings.Replace(init, fmt.Sprintf("    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '%s') THEN\n        CREATE ROLE %s NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;\n    END IF;\n", role, role), "", 1)
		init = strings.Replace(init, fmt.Sprintf("ALTER ROLE %s NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;\n", role), "", 1)
	}
	before["postgres/bundled-init.sh"] = []byte(init)
	for _, pin := range []struct{ name, before, after string }{
		{"compose.postgres.yaml", "1447caf2bc58c6a2e59f6d836a1c9e3b183618b70a9fd5070361512fdbabb685", "f52715c337985c5948a02b2daa8a78596f3bcefb13b0c434d8aba51dd5ee5ae8"},
		{"postgres/bundled-entrypoint.sh", "3135934604ed71add4b116e8cab53dcb95c369ef0b732f813fa5456bb0e60d43", "3135934604ed71add4b116e8cab53dcb95c369ef0b732f813fa5456bb0e60d43"},
		{"postgres/bundled-init.sh", "81cdf13f0e2a69d2a5674a21a7db298c7eafd98d57be767764c5f73b075bb3c8", "6654a03defecce45df62eb4701020aedf3f11ae6eddc24053bd5cb48807d5905"},
	} {
		require.Equal(t, pin.before, fmt.Sprintf("%x", sha256.Sum256(before[pin.name])), "predecessor fixture: %s", pin.name)
		require.Equal(t, pin.after, fmt.Sprintf("%x", sha256.Sum256(after[pin.name])), "candidate fixture: %s", pin.name)
	}
	return before, after
}

func TestNativeUpgradeStagesReviewedPostgresTemplateTransition(t *testing.T) {
	for _, scenario := range []string{"reviewed tuple", "unchanged tuple", "native index from legacy", "native index from roles", "native index unchanged", "native index reverse", "native index legacy reverse", "native index hybrid", "unreviewed image", "reverse", "old compose hybrid", "old init hybrid", "capability", "volume", "SQL", "entrypoint", "installed drift", "base drift"} {
		t.Run(scenario, func(t *testing.T) {
			before, after := postgresTemplatePayloads(t)
			native := maps.Clone(after)
			currentCompose, err := os.ReadFile("../../../../deploy/compose/compose.postgres.yaml")
			require.NoError(t, err)
			require.Equal(t, "64d212275ab3f00b2bdae23fc9261896a94dc880b0e25fbb2f650a406c62b486", fmt.Sprintf("%x", sha256.Sum256(currentCompose)))
			native["compose.postgres.yaml"] = currentCompose
			switch scenario {
			case "native index from legacy":
				after = native
			case "native index from roles":
				before, after = after, native
			case "native index unchanged":
				before, after = native, native
			case "native index reverse":
				before = native
			case "native index legacy reverse":
				before, after = native, before
			case "native index hybrid":
				native["postgres/bundled-init.sh"] = before["postgres/bundled-init.sh"]
				after = native
			case "unreviewed image":
				native["compose.postgres.yaml"] = []byte(strings.Replace(string(currentCompose), "77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873", strings.Repeat("a", 64), 1))
				after = native
			case "unchanged tuple":
				before = after
			case "reverse":
				before, after = after, before
			case "old compose hybrid":
				after["compose.postgres.yaml"] = before["compose.postgres.yaml"]
			case "old init hybrid":
				after["postgres/bundled-init.sh"] = before["postgres/bundled-init.sh"]
			case "capability":
				after["compose.postgres.yaml"] = []byte(strings.Replace(string(after["compose.postgres.yaml"]), "KILL,", "SYS_ADMIN, KILL,", 1))
			case "volume":
				after["compose.postgres.yaml"] = []byte(strings.Replace(string(after["compose.postgres.yaml"]), "leapview-postgres-data:/var/lib/postgresql", "other-data:/var/lib/postgresql", 1))
			case "SQL":
				after["postgres/bundled-init.sh"] = append(after["postgres/bundled-init.sh"], []byte("\npsql -c 'ALTER ROLE leapview_control_readonly SUPERUSER'\n")...)
			case "entrypoint":
				after["postgres/bundled-entrypoint.sh"] = append(after["postgres/bundled-entrypoint.sh"], []byte("\nexit 0\n")...)
			}
			effects := nativeEffectsFixture(t)
			effects.log = os.Stderr
			for name, data := range before {
				path := filepath.Join(effects.root, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, data, 0o600))
			}
			if scenario == "installed drift" || scenario == "base drift" {
				name := "compose.postgres.yaml"
				if scenario == "base drift" {
					name = "compose.yaml"
				}
				require.NoError(t, os.WriteFile(filepath.Join(effects.root, name), []byte("operator drift"), 0o600))
			}
			original, err := os.ReadFile(filepath.Join(effects.root, "leapview.env"))
			require.NoError(t, err)
			writeFixture := func(payload map[string][]byte) string {
				dir := t.TempDir()
				writeTestPayload(t, dir)
				for name, data := range payload {
					path := filepath.Join(dir, name)
					require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
					require.NoError(t, os.WriteFile(path, data, 0o600))
				}
				return dir
			}
			t.Setenv("TEMPLATE_PREDECESSOR", writeFixture(before))
			t.Setenv("TEMPLATE_CANDIDATE", writeFixture(after))
			t.Setenv("TEMPLATE_PREDECESSOR_IMAGE", effects.id.Predecessor)
			t.Setenv("TEMPLATE_CANDIDATE_IMAGE", effects.id.Candidate)
			trace := filepath.Join(t.TempDir(), "trace")
			t.Setenv("TEMPLATE_TRACE", trace)
			bin := t.TempDir()
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(`#!/bin/sh
case "$1" in
 pull) exit 0 ;;
 create)
  if [ "$2" = "$TEMPLATE_PREDECESSOR_IMAGE" ]; then printf 'predecessor\n'
  elif [ "$2" = "$TEMPLATE_CANDIDATE_IMAGE" ]; then printf 'candidate\n'
  else exit 1; fi ;;
 cp)
  case "$2" in
   predecessor:*) cp -R "$TEMPLATE_PREDECESSOR/." "$3"; printf 'predecessor\n' >> "$TEMPLATE_TRACE" ;;
   candidate:*) cp -R "$TEMPLATE_CANDIDATE/." "$3"; printf 'candidate\n' >> "$TEMPLATE_TRACE" ;;
   *) exit 1 ;;
  esac ;;
 rm) exit 0 ;;
 *) exit 1 ;;
esac
`), 0o700))
			err = effects.stage(t.Context())
			extracted, readErr := os.ReadFile(trace)
			require.NoError(t, readErr)
			require.Equal(t, "candidate\npredecessor\n", string(extracted))
			reference, parseErr := ociref.ParseImmutable(effects.id.Candidate)
			require.NoError(t, parseErr)
			generation := filepath.Join(effects.root, "releases", reference.Generation)
			if scenario == "reviewed tuple" || scenario == "unchanged tuple" || scenario == "native index from legacy" || scenario == "native index from roles" || scenario == "native index unchanged" {
				require.NoError(t, err)
				for name, data := range after {
					staged, readErr := os.ReadFile(filepath.Join(generation, name))
					require.NoError(t, readErr)
					require.Equal(t, data, staged, "candidate staged file: %s", name)
					installed, readErr := os.ReadFile(filepath.Join(effects.root, name))
					require.NoError(t, readErr)
					require.Equal(t, before[name], installed, "stage must not cut over: %s", name)
				}
			} else {
				require.Error(t, err)
				if strings.Contains(scenario, "drift") {
					require.ErrorContains(t, err, "installed deployment differs from predecessor payload")
				} else {
					require.ErrorContains(t, err, "deployment topology changed")
				}
				_, statErr := os.Stat(generation)
				require.True(t, os.IsNotExist(statErr), "rejected payload must not be staged")
			}
			unchanged, readErr := os.ReadFile(filepath.Join(effects.root, "leapview.env"))
			require.NoError(t, readErr)
			require.Equal(t, original, unchanged)
		})
	}
}
