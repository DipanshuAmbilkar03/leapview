package local

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Retain the exact registry index bytes so this offline check proves that the
// shipped pin selects native images for both supported Linux architectures.
// A platform-specific manifest digest must not satisfy the package contract.
func TestShippedPostgresImageSupportsBothNativeArchitectures(t *testing.T) {
	encoded, err := os.ReadFile("testdata/postgres-image-index.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
	if !strings.HasSuffix(postgresImage, "@"+digest) {
		t.Fatalf("shipped PostgreSQL pin %s does not match verified multi-platform index %s", postgresImage, digest)
	}
	var index struct {
		MediaType string `json:"mediaType"`
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(encoded, &index); err != nil {
		t.Fatal(err)
	}
	if index.MediaType != "application/vnd.oci.image.index.v1+json" {
		t.Fatalf("PostgreSQL pin is not a multi-platform index: %s", index.MediaType)
	}
	for _, architecture := range []string{"amd64", "arm64"} {
		found := 0
		for _, manifest := range index.Manifests {
			if manifest.Platform.OS == "linux" && manifest.Platform.Architecture == architecture && strings.HasPrefix(manifest.Digest, "sha256:") {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("PostgreSQL index has %d native linux/%s images, want one", found, architecture)
		}
	}
	// The conventional bundled server has the same native-platform promise.
	compose, err := os.ReadFile("../compose/compose.postgres.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "image: "+postgresImage+"\n") {
		t.Fatal("conventional Compose PostgreSQL pin differs from the verified index")
	}
}
