package application

import (
	"context"

	"github.com/flidai/leapview/internal/analytics/exploration/saved"
)

var _ saved.MutationCapabilityReader = (*Service)(nil)

// MutationCapabilities reads lifecycle metadata and evaluates the same
// authorizer as UpdateVersion and Archive under one validated serving lease.
func (s *Service) MutationCapabilities(ctx context.Context, request saved.ReadRequest) (saved.MutationCapabilities, error) {
	if err := request.Validate(); err != nil {
		return saved.MutationCapabilities{}, err
	}
	lease, err := s.acquire(ctx, request.ProjectID)
	if err != nil {
		return saved.MutationCapabilities{}, err
	}
	defer lease.Release()
	lifecycle, err := s.repository.GetLifecycle(ctx, saved.ReadInput{ProjectID: request.ProjectID, ID: request.ID})
	if err != nil {
		return saved.MutationCapabilities{}, err
	}
	if err := ensureUsableLifecycle(lifecycle, request.ProjectID, request.ID); err != nil {
		return saved.MutationCapabilities{}, err
	}
	permission := AuthorizationRequest{
		ActorID: request.ActorID, ProjectID: request.ProjectID, ExplorationID: request.ID,
		OwnerPrincipalID: lifecycle.OwnerPrincipalID, Title: lifecycle.Title,
		Visibility: lifecycle.Visibility, Status: lifecycle.Status,
		SemanticModelID: lifecycle.SemanticModelID, Lifecycle: lifecycle,
		Action: AuthorizationActionView,
	}
	if err := s.authorize(ctx, lease, permission); err != nil {
		return saved.MutationCapabilities{}, err
	}
	if lifecycle.Status != saved.StatusActive {
		return saved.MutationCapabilities{}, nil
	}
	permission.Action = AuthorizationActionEdit
	canEdit := s.authorize(ctx, lease, permission) == nil
	permission.Action = AuthorizationActionArchive
	canArchive := s.authorize(ctx, lease, permission) == nil
	return saved.MutationCapabilities{CanEdit: canEdit, CanArchive: canArchive}, nil
}
