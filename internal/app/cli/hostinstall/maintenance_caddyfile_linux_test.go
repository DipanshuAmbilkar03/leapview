//go:build linux

package hostinstall

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flidai/leapview/internal/platform/ociref"
	"github.com/stretchr/testify/require"
)

// The reviewed proxy transition must reach the host: staging writes the
// candidate Caddyfile into its own generation, and cutover makes the installed
// link resolve to it so the next upgrade sees installed == predecessor.
func TestNativeUpgradeStagesAndActivatesReviewedCaddyfile(t *testing.T) {
	for _, scenario := range []string{"reviewed", "reverse", "unreviewed candidate"} {
		t.Run(scenario, func(t *testing.T) {
			predecessorCaddy, candidateCaddy := reviewedCaddyfileFixtures(t)
			before, after := map[string][]byte{}, map[string][]byte{}
			for _, name := range []string{"compose.yaml", "compose.https.yaml", "deployment.env.example", "compose.postgres.yaml", "postgres/bundled-entrypoint.sh", "postgres/bundled-init.sh"} {
				data, err := os.ReadFile(filepath.Join("../../../../deploy/compose", name))
				require.NoError(t, err)
				before[name], after[name] = data, data
			}
			before["Caddyfile"], after["Caddyfile"] = predecessorCaddy, candidateCaddy
			switch scenario {
			case "reverse":
				before["Caddyfile"], after["Caddyfile"] = candidateCaddy, predecessorCaddy
			case "unreviewed candidate":
				after["Caddyfile"] = append(append([]byte{}, candidateCaddy...), "\n"...)
			}
			effects := nativeEffectsFixture(t)
			effects.log = os.Stderr
			writeFixture := func(payload map[string][]byte) string {
				dir := t.TempDir()
				writeTestPayload(t, dir)
				for name, data := range payload {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
				}
				return dir
			}
			predecessorPayload, candidatePayload := writeFixture(before), writeFixture(after)
			// Install the predecessor the way hosts do: root files link into the
			// active generation.
			paths := InstalledPaths(effects.root)
			predecessorRef, err := ociref.ParseImmutable(effects.id.Predecessor)
			require.NoError(t, err)
			predecessorFiles, err := readPayload(predecessorPayload)
			require.NoError(t, err)
			_, err = stageGeneration(paths, effects.id.Predecessor, predecessorFiles)
			require.NoError(t, err)
			require.NoError(t, activateGeneration(paths, predecessorRef.Generation))
			for name := range before {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(effects.root, name)), 0o755))
				target, err := filepath.Rel(filepath.Dir(filepath.Join(effects.root, name)), filepath.Join(effects.root, "current", name))
				require.NoError(t, err)
				require.NoError(t, os.Symlink(target, filepath.Join(effects.root, name)))
			}
			t.Setenv("TEMPLATE_PREDECESSOR", predecessorPayload)
			t.Setenv("TEMPLATE_CANDIDATE", candidatePayload)
			t.Setenv("TEMPLATE_PREDECESSOR_IMAGE", effects.id.Predecessor)
			t.Setenv("TEMPLATE_CANDIDATE_IMAGE", effects.id.Candidate)
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
   predecessor:*) cp -R "$TEMPLATE_PREDECESSOR/." "$3" ;;
   candidate:*) cp -R "$TEMPLATE_CANDIDATE/." "$3" ;;
   *) exit 1 ;;
  esac ;;
 rm) exit 0 ;;
 *) exit 1 ;;
esac
`), 0o700))
			candidateRef, err := ociref.ParseImmutable(effects.id.Candidate)
			require.NoError(t, err)
			err = effects.stage(t.Context())
			if scenario != "reviewed" {
				require.ErrorContains(t, err, "deployment topology changed: Caddyfile")
				_, statErr := os.Stat(filepath.Join(effects.root, "releases", candidateRef.Generation))
				require.True(t, os.IsNotExist(statErr), "rejected payload must not be staged")
				return
			}
			require.NoError(t, err)
			installed, err := os.ReadFile(filepath.Join(effects.root, "Caddyfile"))
			require.NoError(t, err)
			require.Equal(t, predecessorCaddy, installed, "stage must not cut over")
			require.NoError(t, effects.link("releases/"+candidateRef.Generation))
			installed, err = os.ReadFile(filepath.Join(effects.root, "Caddyfile"))
			require.NoError(t, err)
			require.Equal(t, candidateCaddy, installed, "cutover must install the reviewed Caddyfile")
			// Recovery reactivates the predecessor generation and its proxy file.
			require.NoError(t, effects.link("releases/"+predecessorRef.Generation))
			installed, err = os.ReadFile(filepath.Join(effects.root, "Caddyfile"))
			require.NoError(t, err)
			require.Equal(t, predecessorCaddy, installed)
		})
	}
}
