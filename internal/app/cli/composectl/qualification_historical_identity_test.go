package composectl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func validateHistoricalTransitionCandidate(ctx context.Context, options qualificationHistoricalTransitionOptions, repoRoot string) error {
	revisionCommand := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	revisionCommand.Dir = repoRoot
	validatorRevisionBytes, err := revisionCommand.Output()
	if err != nil || strings.TrimSpace(string(validatorRevisionBytes)) != options.CandidateRevision {
		return errors.New("historical transition validator checkout must match the exact candidate source revision")
	}
	statusCommand := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "--untracked-files=all")
	statusCommand.Dir = repoRoot
	status, err := statusCommand.Output()
	if err != nil || len(status) != 0 {
		return errors.New("historical transition receipt requires a clean candidate validator checkout")
	}
	if options.FinalArtifact {
		if err := validateHistoricalTransitionAdmission(options.AdmissionPath, options); err != nil {
			return err
		}
	}
	return validateHistoricalTransitionImageIdentity(ctx, options)
}

func validateHistoricalTransitionImageIdentity(ctx context.Context, options qualificationHistoricalTransitionOptions) error {
	if (!qualificationHistoricalLocalImagePattern.MatchString(options.CandidateImage) &&
		!qualificationHistoricalOCIImagePattern.MatchString(options.CandidateImage)) ||
		!qualificationHistoricalRevisionPattern.MatchString(options.CandidateRevision) {
		return errors.New("historical transition image validation requires an immutable image and full source revision")
	}
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", options.CandidateImage, "--format", "{{.Id}}")
	imageID, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect the supplied historical-transition candidate image: %w (%s)", err, strings.TrimSpace(string(imageID)))
	}
	if qualificationHistoricalLocalImagePattern.MatchString(options.CandidateImage) && strings.TrimSpace(string(imageID)) != options.CandidateImage {
		return errors.New("local historical transition candidate image ID does not resolve to the supplied immutable image")
	}
	version := exec.CommandContext(ctx, "docker", "run", "--rm", options.CandidateImage, "version", "--format", "json")
	output, err := version.CombinedOutput()
	if err != nil {
		return fmt.Errorf("read candidate image build identity: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	var identity struct {
		Revision string `json:"revision"`
		Dirty    bool   `json:"dirty"`
	}
	if err := json.Unmarshal(output, &identity); err != nil || identity.Revision != options.CandidateRevision || identity.Dirty {
		return errors.New("historical transition candidate image does not contain the exact clean supplied source revision")
	}
	return nil
}

func validateHistoricalTransitionAdmission(path string, options qualificationHistoricalTransitionOptions) error {
	producer, err := qualificationHistoricalProducerWorkflow(options.ProducerWorkflow)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > 1<<20 {
		return errors.New("final historical transition requires a bounded regular OCI admission artifact")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read exact OCI admission evidence: %w", err)
	}
	var admission struct {
		SchemaVersion  int    `json:"schemaVersion"`
		Image          string `json:"image"`
		Digest         string `json:"digest"`
		RegistryDigest string `json:"registryDigest"`
		Attestation    struct {
			Verified       bool   `json:"verified"`
			Repository     string `json:"repository"`
			Workflow       string `json:"workflow"`
			SourceRevision string `json:"sourceRevision"`
		} `json:"attestation"`
		SBOM struct {
			Discoverable  bool   `json:"discoverable"`
			PredicateType string `json:"predicateType"`
		} `json:"sbom"`
		Vulnerability struct {
			Passed   bool   `json:"passed"`
			SHA256   string `json:"sha256"`
			Scanner  string `json:"scanner"`
			Platform string `json:"platform"`
		} `json:"vulnerabilityPolicy"`
	}
	if err := json.Unmarshal(encoded, &admission); err != nil {
		return errors.New("decode exact OCI admission evidence")
	}
	digest := strings.TrimPrefix(options.CandidateImage, "ghcr.io/flidai/leapview@")
	if admission.SchemaVersion != 1 || admission.Image != options.CandidateImage || admission.Digest != digest || admission.RegistryDigest != digest ||
		!admission.Attestation.Verified || admission.Attestation.Repository != "flidai/leapview" ||
		admission.Attestation.Workflow != producer || admission.Attestation.SourceRevision != options.AdmissionSourceRevision ||
		!admission.SBOM.Discoverable || admission.SBOM.PredicateType != "https://spdx.dev/Document/v2.3" ||
		!admission.Vulnerability.Passed || admission.Vulnerability.Scanner != "trivy" ||
		(admission.Vulnerability.Platform != "" && admission.Vulnerability.Platform != "linux/amd64") || !qualificationHistoricalSHA256Pattern.MatchString(admission.Vulnerability.SHA256) {
		return errors.New("OCI admission evidence does not bind the exact final candidate image and revision")
	}
	return nil
}
