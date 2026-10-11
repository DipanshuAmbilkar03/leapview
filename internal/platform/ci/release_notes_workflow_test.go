package ci

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleasePublicationUsesVersionedNotes(t *testing.T) {
	workflow := readQualificationWorkflow(t, "../../../.github/workflows/release.yml")
	var script string
	for _, step := range workflow.Jobs["publish"].Steps {
		if step.Name == "Publish GitHub release" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("publication step missing")
	}
	for _, present := range []bool{true, false} {
		t.Run(map[bool]string{true: "reviewed notes", false: "missing notes"}[present], func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "release-notes"), 0o700); err != nil {
				t.Fatal(err)
			}
			const notes = "# Reviewed alpha\n\nLegacy tokens must be replaced.\n"
			if present {
				if err := os.WriteFile(filepath.Join(root, "release-notes/v1.2.3-alpha.1.md"), []byte(notes), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// Execute the actual workflow shell; only replace the network client.
			stub := `gh() {
  if [ "$1 $2" = 'release view' ]; then return 1; fi
  printf '%s\n' "$@" > arguments
  while [ "$#" -gt 0 ]; do
    if [ "$1" = --notes-file ]; then cat "$2" > published-notes; return; fi
    shift
  done
};
`
			cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", stub+script)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "RELEASE_TAG=v1.2.3-alpha.1", "RELEASE_VERSION=1.2.3-alpha.1", "GITHUB_REPOSITORY=flidai/leapview")
			output, err := cmd.CombinedOutput()
			if !present {
				if err == nil {
					t.Fatal("publication accepted missing reviewed notes")
				}
				if _, err := os.Stat(filepath.Join(root, "arguments")); !os.IsNotExist(err) {
					t.Fatal("publication attempted before rejecting missing notes")
				}
				return
			}
			if err != nil {
				t.Fatalf("publication: %v\n%s", err, output)
			}
			got, err := os.ReadFile(filepath.Join(root, "published-notes"))
			if err != nil || string(got) != notes {
				t.Fatalf("reviewed notes were not published: %q, %v", got, err)
			}
			args, err := os.ReadFile(filepath.Join(root, "arguments"))
			if err != nil || !strings.Contains(string(args), "--prerelease\n") || !strings.Contains(string(args), "--verify-tag\n") {
				t.Fatalf("publication lost alpha/tag guards: %s, %v", args, err)
			}
		})
	}
}
