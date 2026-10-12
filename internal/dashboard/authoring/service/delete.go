package service

import (
	"context"
	"fmt"

	"github.com/flidai/leapview/internal/dashboard/authoring"
	"github.com/flidai/leapview/internal/project/graph"
)

func (s *Service) delete(ctx context.Context, projectID graph.ResourceID, command authoring.Command) (Result, error) {
	deleter, ok := s.repository.(authoring.DeleteRepository)
	if !ok {
		return Result{}, fmt.Errorf("dashboard authoring delete operation is unavailable")
	}
	fingerprint, err := command.Fingerprint()
	if err != nil {
		return Result{}, err
	}
	evidenceAt, err := s.utcNow()
	if err != nil {
		return Result{}, err
	}
	evidence := authoring.CommandEvidence{ID: command.ID, Fingerprint: fingerprint, Action: authoring.AuthorizationActionDelete, Provenance: command.Provenance, OccurredAt: evidenceAt}
	if err := evidence.Validate(); err != nil {
		return Result{}, err
	}
	if replay, found, err := deleter.LookupDeleteCommand(ctx, projectID, command.DashboardID, evidence); err != nil {
		return Result{}, err
	} else if found {
		// The lifecycle is gone by the time a delete command is replayed, so
		// authorize from the owner retained in the delete fence before
		// disclosing the prior result. This preserves the private draft policy and
		// ensures a revoked actor cannot replay a previously authorized delete.
		if err := s.authorizer.Authorize(ctx, AuthorizationRequest{
			ActorID: command.Provenance.ActorID, ProjectID: projectID, DashboardID: command.DashboardID,
			OwnerPrincipalID: replay.OwnerPrincipalID, Target: AuthorizationTargetAuthoredDashboard,
			Visibility: authoring.VisibilityPrivate, PrivateDraft: true, Action: authoring.AuthorizationActionDelete,
		}); err != nil {
			return Result{}, err
		}
		return Result{Revision: replay.Revision, Lifecycle: authoring.DashboardLifecycle{}}, nil
	}
	lifecycle, err := s.repository.Get(ctx, projectID, command.DashboardID)
	if err != nil {
		return Result{}, err
	}
	if lifecycle.ProjectID != projectID || lifecycle.ID != command.DashboardID {
		return Result{}, fmt.Errorf("%w: dashboard lifecycle identity does not match delete request", authoring.ErrInvalidAuthoring)
	}
	if !lifecycle.IsPrivateDraft() {
		return Result{}, fmt.Errorf("%w: only private draft dashboards can be deleted", authoring.ErrConflict)
	}
	if command.DraftID != "" && (lifecycle.Draft == nil || command.DraftID != lifecycle.Draft.ID) {
		return Result{}, fmt.Errorf("%w: command belongs to a different draft", authoring.ErrInvalidAuthoring)
	}
	if err := s.authorizer.Authorize(ctx, AuthorizationRequest{ActorID: command.Provenance.ActorID, ProjectID: projectID, DashboardID: lifecycle.ID, OwnerPrincipalID: lifecycle.OwnerPrincipalID, PrivateDraft: lifecycle.IsPrivateDraft(), SemanticModel: lifecycle.SemanticModel, Target: AuthorizationTargetAuthoredDashboard, Visibility: lifecycle.Visibility, Action: authoring.AuthorizationActionDelete}); err != nil {
		return Result{}, err
	}
	token := currentToken(lifecycle)
	if token.IsZero() {
		return Result{}, fmt.Errorf("%w: expected revision does not match current lifecycle", authoring.ErrStaleRevision)
	}
	if command.ExpectedRevision.IsZero() {
		command.ExpectedRevision = token
	} else if !sameToken(token, command.ExpectedRevision) {
		return Result{}, fmt.Errorf("%w: expected revision does not match current lifecycle", authoring.ErrStaleRevision)
	}
	deleted, err := deleter.Delete(ctx, authoring.DeleteInput{ProjectID: projectID, DashboardID: command.DashboardID, ExpectedCurrentRevision: command.ExpectedRevision, Evidence: evidence})
	if err != nil {
		return Result{}, err
	}
	if deleted.Revision.IsZero() {
		deleted.Revision = token
	}
	return Result{Revision: deleted.Revision, Lifecycle: lifecycle}, nil
}
