package accessadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/flidai/leapview/internal/access"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	"github.com/flidai/leapview/internal/dashboard/authoring/service"
	"github.com/flidai/leapview/internal/project/graph"
)

func TestOwnedPrivateDraftDeletionKeepsGeneralDeleteSeparate(t *testing.T) {
	for _, tc := range []struct {
		name, actor, owner                 string
		privateDraft, edit, explicitDelete bool
		action                             authoring.AuthorizationAction
		want                               bool
	}{
		{name: "owner can discard editable draft", actor: "owner", owner: "owner", privateDraft: true, edit: true, want: true},
		{name: "ownership alone is insufficient", actor: "owner", owner: "owner", privateDraft: true},
		{name: "another owner's draft", actor: "editor", owner: "owner", privateDraft: true, edit: true},
		{name: "unproven lifecycle", actor: "owner", owner: "owner", edit: true},
		{name: "archive still requires delete", actor: "owner", owner: "owner", privateDraft: true, edit: true, action: authoring.AuthorizationActionArchive},
		{name: "explicit delete remains valid", actor: "admin", owner: "owner", explicitDelete: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			adapter, err := New(Options{
				AuthorizeTypedResource: func(context.Context, string, graph.ResourceID, access.ResourceRef, access.Action) (bool, bool, error) {
					return true, tc.explicitDelete, nil
				},
				AuthorizeTypedProject: func(context.Context, string, graph.ResourceID, access.Action) (bool, bool, error) {
					return true, false, nil
				},
				AuthorizeOwnedDraftDelete: func(context.Context, string, graph.ResourceID, access.ResourceRef) (bool, bool, error) {
					calls++
					return true, tc.edit, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			action := tc.action
			if action == "" {
				action = authoring.AuthorizationActionDelete
			}
			err = adapter.Authorize(t.Context(), service.AuthorizationRequest{ActorID: tc.actor, OwnerPrincipalID: tc.owner, ProjectID: "project", DashboardID: "dashboard", Target: service.AuthorizationTargetAuthoredDashboard, Visibility: authoring.VisibilityPrivate, PrivateDraft: tc.privateDraft, Action: action})
			if (err == nil) != tc.want || (err != nil && !errors.Is(err, access.ErrForbidden)) {
				t.Fatalf("authorization = %v, want allowed %t", err, tc.want)
			}
			eligible := tc.privateDraft && tc.actor == tc.owner && action == authoring.AuthorizationActionDelete && !tc.explicitDelete
			if (calls > 0) != eligible {
				t.Fatalf("owner-policy calls = %d, eligible %t", calls, eligible)
			}
		})
	}
}

func TestOwnedPrivateDraftDeletionFailsClosedOnUnavailablePolicy(t *testing.T) {
	for _, untyped := range []bool{false, true} {
		backendErr := errors.New("authorization unavailable")
		adapter, _ := New(Options{
			AuthorizeTypedResource: func(context.Context, string, graph.ResourceID, access.ResourceRef, access.Action) (bool, bool, error) {
				return true, false, nil
			},
			AuthorizeTypedProject: func(context.Context, string, graph.ResourceID, access.Action) (bool, bool, error) {
				return true, false, nil
			},
			AuthorizeOwnedDraftDelete: func(context.Context, string, graph.ResourceID, access.ResourceRef) (bool, bool, error) {
				if untyped {
					return false, true, nil
				}
				return true, false, backendErr
			},
		})
		err := adapter.Authorize(t.Context(), service.AuthorizationRequest{ActorID: "owner", OwnerPrincipalID: "owner", ProjectID: "project", DashboardID: "dashboard", Target: service.AuthorizationTargetAuthoredDashboard, Visibility: authoring.VisibilityPrivate, PrivateDraft: true, Action: authoring.AuthorizationActionDelete})
		if untyped && !errors.Is(err, access.ErrForbidden) || !untyped && !errors.Is(err, backendErr) {
			t.Fatalf("authorization = %v", err)
		}
	}
}
