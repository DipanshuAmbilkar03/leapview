package architecture

import (
	"os/exec"
	"strings"
	"testing"
)

func TestOfflineControllerExcludesBrowserAuthAndDeprecatedOpenPGP(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/leapviewctl")
	command.Dir = repoRoot(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("list controller dependencies: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if dependency == modulePath+"/internal/access/module" || dependency == "github.com/gorilla/csrf" ||
			dependency == "golang.org/x/crypto/openpgp" || strings.HasPrefix(dependency, "golang.org/x/crypto/openpgp/") {
			t.Errorf("offline controller imports unrelated browser auth or deprecated cryptography: %s", dependency)
		}
	}
}
