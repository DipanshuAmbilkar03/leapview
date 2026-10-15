package composectl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apigenclient "github.com/Yacobolo/toolbelt/apigen/runtime/client"
	deploymentgen "github.com/flidai/leapview/internal/deployment/api/gen"
)

// Publication commitment precedes in-process runtime cutover. Generation reads
// can therefore return a transient 503 after the durable publication commits.
// Retry only reads, revalidating the complete evidence tuple on every attempt.
func waitQualificationCanonicalPublicationEvidence(
	ctx context.Context,
	httpClient *http.Client,
	options qualificationAuthoringOptions,
	token string,
	candidate QualificationCandidate,
	pending QualificationPublication,
	committed QualificationPublication,
	interval time.Duration,
) (QualificationPublication, QualificationDeployment, error) {
	waitCtx, cancel := qualificationContext(ctx, 5*time.Minute)
	defer cancel()
	var publication QualificationPublication
	var deployment QualificationDeployment
	err := qualificationWait(waitCtx, interval, func(pollCtx context.Context) (bool, error) {
		var err error
		publication, deployment, err = qualificationCanonicalPublicationEvidence(
			pollCtx, httpClient, options, token, candidate, pending, committed,
		)
		if qualificationTransientDeploymentError(err) {
			return false, nil
		}
		return err == nil, err
	})
	if err != nil {
		return QualificationPublication{}, QualificationDeployment{}, err
	}
	return publication, deployment, nil
}

func qualificationCanonicalPublicationEvidence(
	ctx context.Context,
	httpClient *http.Client,
	options qualificationAuthoringOptions,
	token string,
	candidate QualificationCandidate,
	pending QualificationPublication,
	committed QualificationPublication,
) (QualificationPublication, QualificationDeployment, error) {
	client := deploymentgen.NewGenClient(qualificationGeneratedTransport(
		options.Target,
		token,
		httpClient,
	))
	publication, err := client.GetDeliveryPublicationEvidence(
		ctx,
		deploymentgen.GenGetDeliveryPublicationEvidenceClientRequest{
			Project: options.ProjectID, Publication: pending.DeploymentID,
		},
	)
	if err != nil {
		return QualificationPublication{}, QualificationDeployment{}, err
	}
	serverPublication := publication.Body
	if serverPublication.Status != "committed" ||
		serverPublication.Id != pending.DeploymentID ||
		serverPublication.Id != committed.DeploymentID ||
		serverPublication.CandidateId != candidate.ID ||
		serverPublication.GenerationId != committed.GenerationID ||
		serverPublication.PlanId != candidate.PlanID ||
		serverPublication.PlanDigest != candidate.PlanDigest ||
		serverPublication.TargetId != candidate.TargetID {
		return QualificationPublication{}, QualificationDeployment{}, fmt.Errorf("canonical publication evidence does not match the previewed candidate")
	}
	plan, err := client.GetDeliveryPlanPreview(
		ctx,
		deploymentgen.GenGetDeliveryPlanPreviewClientRequest{
			Project: options.ProjectID, Plan: serverPublication.PlanId,
		},
	)
	if err != nil {
		return QualificationPublication{}, QualificationDeployment{}, err
	}
	planMatches := qualificationPlanMatchesCandidate(plan.Body, candidate)
	provenanceMatches := plan.Body.ProvenanceDigest == candidate.ProvenanceDigest
	if !planMatches || !provenanceMatches {
		return QualificationPublication{}, QualificationDeployment{}, fmt.Errorf(
			"canonical plan evidence does not match the previewed candidate (plan digest=%t, source digest=%t, provenance=%t, target=%t)",
			plan.Body.PlanDigest == candidate.PlanDigest,
			plan.Body.SourceDigest == candidate.ArtifactDigest,
			plan.Body.ProvenanceDigest == candidate.ProvenanceDigest,
			plan.Body.TargetId == candidate.TargetID,
		)
	}
	serverCandidate, err := client.GetDeliveryCandidateStatus(
		ctx,
		deploymentgen.GenGetDeliveryCandidateStatusClientRequest{
			Project: options.ProjectID, Candidate: candidate.ID,
		},
	)
	if err != nil {
		return QualificationPublication{}, QualificationDeployment{}, err
	}
	snapshotSealID := ""
	if serverCandidate.Body.SnapshotSealId != nil {
		snapshotSealID = strings.TrimSpace(*serverCandidate.Body.SnapshotSealId)
	}
	if serverCandidate.Body.Id != candidate.ID ||
		serverCandidate.Body.Status != deploymentgen.DeliveryCandidateStatusReady ||
		serverCandidate.Body.PlanId != candidate.PlanID ||
		serverCandidate.Body.PlanDigest != candidate.PlanDigest ||
		serverCandidate.Body.SourceDigest != candidate.ArtifactDigest ||
		serverCandidate.Body.TargetId != candidate.TargetID || snapshotSealID == "" {
		return QualificationPublication{}, QualificationDeployment{}, fmt.Errorf("canonical candidate evidence does not match the previewed candidate")
	}
	generation, err := client.GetDeliveryGenerationStatus(
		ctx,
		deploymentgen.GenGetDeliveryGenerationStatusClientRequest{
			Project: options.ProjectID, Generation: serverPublication.GenerationId,
		},
	)
	if err != nil {
		return QualificationPublication{}, QualificationDeployment{}, err
	}
	if generation.Body.Status != "active" ||
		generation.Body.CandidateId != candidate.ID ||
		generation.Body.PlanId != candidate.PlanID ||
		generation.Body.PlanDigest != candidate.PlanDigest ||
		generation.Body.TargetId != candidate.TargetID ||
		generation.Body.ServingArtifactDigest != serverCandidate.Body.ServingArtifactDigest {
		return QualificationPublication{}, QualificationDeployment{}, fmt.Errorf("canonical generation evidence does not match the previewed candidate")
	}
	pending.ArtifactDigest = serverCandidate.Body.ServingArtifactDigest
	pending.CandidateRevision = candidate.Revision
	pending.PrincipalID = candidate.PrincipalID
	pending.ReleaseDigest = candidate.ProvenanceDigest
	pending.Status = string(serverPublication.Status)
	pending.GenerationID = serverPublication.GenerationId
	return pending, QualificationDeployment{
		CandidateID: candidate.ID, CandidateRevision: candidate.Revision,
		TargetID: candidate.TargetID, PrincipalID: candidate.PrincipalID,
		ArtifactDigest: generation.Body.ServingArtifactDigest,
		ReleaseDigest:  candidate.ProvenanceDigest,
		GenerationID:   generation.Body.Id, SnapshotSealID: snapshotSealID,
		PlanID:     generation.Body.PlanId,
		PlanDigest: generation.Body.PlanDigest, Status: string(generation.Body.Status),
	}, nil
}

func qualificationPlanMatchesCandidate(plan deploymentgen.DeliveryPlanPreviewResponse, candidate QualificationCandidate) bool {
	// Candidate provenance identifies the synchronized project artifact, while
	// plan provenance identifies the canonical delivery inputs (including the
	// retained source attestation). They are independently bound by the plan
	// digest and must not be compared as if they were the same hash domain.
	return plan.PlanDigest == candidate.PlanDigest &&
		plan.SourceDigest == candidate.ArtifactDigest &&
		plan.TargetId == candidate.TargetID
}

// qualificationTransientDeploymentError treats generated problem responses and
// plain-text 503/429s from authorization, readiness, or rate-limit middleware
// as transient while a native publication is being activated. These responses
// may bypass APIGen's problem+json envelope, so checking only ProblemError
// would abort observation before the async worker finishes.
func qualificationTransientDeploymentError(err error) bool {
	if err == nil {
		return false
	}
	var problem *apigenclient.ProblemError
	if errors.As(err, &problem) {
		return problem.Problem.Status == http.StatusServiceUnavailable || problem.Problem.Status == http.StatusTooManyRequests || problem.Response.StatusCode == http.StatusServiceUnavailable || problem.Response.StatusCode == http.StatusTooManyRequests
	}
	message := strings.TrimSpace(err.Error())
	return strings.HasSuffix(message, ": "+http.StatusText(http.StatusServiceUnavailable)) || strings.HasSuffix(message, ": "+http.StatusText(http.StatusTooManyRequests))
}
