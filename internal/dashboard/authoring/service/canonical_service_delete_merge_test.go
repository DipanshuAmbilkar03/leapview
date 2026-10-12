package service_test

import (
	"errors"
	"testing"

	"context"
	"github.com/flidai/leapview/internal/access"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	"github.com/flidai/leapview/internal/dashboard/authoring/accessadapter"
	"github.com/flidai/leapview/internal/dashboard/authoring/service"
	"github.com/flidai/leapview/internal/project/graph"
)

func TestCanonicalServiceDeleteOnlyRemovesPrivateDraft(t *testing.T) {
	repository, authorizer, compiler := newCanonicalRepository(), &canonicalAuthorizer{}, &canonicalCompiler{}
	svc := newCanonicalService(t, repository, authorizer, compiler, "dashboard-delete", "draft-delete", "revision-delete")
	created, err := svc.Create(t.Context(), service.CreateRequest{ProjectID: "project:test", ActorID: "actor", OwnerPrincipalID: "actor", Title: "Orders", Slug: "orders", SemanticModel: "model:test", Visibility: authoring.VisibilityPrivate, Origin: authoring.OriginUI, IdempotencyKey: "create-delete"})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := svc.Execute(t.Context(), "project:test", authoring.Command{ID: "delete-1", DashboardID: created.Lifecycle.ID, Provenance: authoring.Provenance{Origin: authoring.OriginUI, ActorID: "actor"}, Delete: &authoring.DeletePayload{}})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Revision != created.Revision || repository.lifecycle.ID != "" {
		t.Fatalf("deleted result = %#v, repository lifecycle = %#v", deleted, repository.lifecycle)
	}

	repository, authorizer, compiler = newCanonicalRepository(), &canonicalAuthorizer{}, &canonicalCompiler{}
	svc = newCanonicalService(t, repository, authorizer, compiler, "dashboard-restricted", "draft-restricted", "revision-restricted")
	created, err = svc.Create(t.Context(), service.CreateRequest{ProjectID: "project:test", ActorID: "actor", OwnerPrincipalID: "actor", Title: "Restricted", Slug: "restricted", SemanticModel: "model:test", Visibility: authoring.VisibilityRestricted, Origin: authoring.OriginUI, IdempotencyKey: "create-restricted"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Execute(t.Context(), "project:test", authoring.Command{ID: "delete-2", DashboardID: created.Lifecycle.ID, Provenance: authoring.Provenance{Origin: authoring.OriginUI, ActorID: "actor"}, Delete: &authoring.DeletePayload{}}); !errors.Is(err, authoring.ErrConflict) {
		t.Fatalf("restricted delete error = %v, want conflict", err)
	}
	if repository.lifecycle.ID == "" {
		t.Fatal("restricted dashboard was deleted")
	}
}

func TestCanonicalServiceDeleteReplayRequiresCurrentAuthorization(t *testing.T) {
	repository, authorizer, compiler := newCanonicalRepository(), &canonicalAuthorizer{}, &canonicalCompiler{}
	svc := newCanonicalService(t, repository, authorizer, compiler, "dashboard-delete-replay", "draft-delete-replay", "revision-delete-replay")
	created, err := svc.Create(t.Context(), service.CreateRequest{
		ProjectID: "project:test", ActorID: "actor", OwnerPrincipalID: "owner", Title: "Orders", Slug: "orders",
		SemanticModel: "model:test", Visibility: authoring.VisibilityPrivate, Origin: authoring.OriginUI, IdempotencyKey: "create-delete-replay",
	})
	if err != nil {
		t.Fatal(err)
	}
	command := authoring.Command{
		ID: "delete-replay", DashboardID: created.Lifecycle.ID,
		Provenance: authoring.Provenance{Origin: authoring.OriginUI, ActorID: "actor"}, Delete: &authoring.DeletePayload{},
	}
	if _, err := svc.Execute(t.Context(), "project:test", command); err != nil {
		t.Fatal(err)
	}

	// The lifecycle has been removed, so this authorization must use the owner
	// retained in the delete fence. Simulate the actor losing access after the
	// first request and ensure a replay cannot disclose or return the result.
	authorizer.denied = true
	if _, err := svc.Execute(t.Context(), "project:test", command); err == nil || err.Error() != "denied" {
		t.Fatalf("revoked delete replay error = %v", err)
	}
	if len(authorizer.calls) == 0 || authorizer.calls[len(authorizer.calls)-1].OwnerPrincipalID != "owner" || authorizer.calls[len(authorizer.calls)-1].Action != authoring.AuthorizationActionDelete {
		t.Fatalf("replay authorization = %#v", authorizer.calls)
	}
}

func TestPrivateDraftOwnerDeleteRechecksEditAuthorityOnReplay(t *testing.T) {
	repo, bootstrapAuth, compiler := newCanonicalRepository(), &canonicalAuthorizer{}, &canonicalCompiler{}
	bootstrap := newCanonicalService(t, repo, bootstrapAuth, compiler, "owned-dashboard", "owned-draft", "owned-revision")
	created, err := bootstrap.Create(t.Context(), service.CreateRequest{ProjectID: "project:test", ActorID: "owner", OwnerPrincipalID: "owner", Title: "Orders", Slug: "orders", SemanticModel: "model:test", Visibility: authoring.VisibilityPrivate, Origin: authoring.OriginUI, IdempotencyKey: "create-owned"})
	if err != nil {
		t.Fatal(err)
	}
	canEdit := true
	adapter, err := accessadapter.New(accessadapter.Options{
		AuthorizeTypedResource: func(context.Context, string, graph.ResourceID, access.ResourceRef, access.Action) (bool, bool, error) {
			return true, false, nil
		},
		AuthorizeTypedProject: func(context.Context, string, graph.ResourceID, access.Action) (bool, bool, error) {
			return true, false, nil
		},
		AuthorizeOwnedDraftDelete: func(context.Context, string, graph.ResourceID, access.ResourceRef) (bool, bool, error) {
			return true, canEdit, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newCanonicalService(t, repo, adapter, compiler)
	command := authoring.Command{ID: "discard-owned", DashboardID: created.Lifecycle.ID, Provenance: authoring.Provenance{Origin: authoring.OriginUI, ActorID: "owner"}, Delete: &authoring.DeletePayload{}}
	repo.lifecycle.ProjectID = "another-project"
	if _, err := svc.Execute(t.Context(), "project:test", command); !errors.Is(err, authoring.ErrInvalidAuthoring) {
		t.Fatalf("mismatched lifecycle deletion = %v", err)
	}
	repo.lifecycle.ProjectID = "project:test"
	// Durable lifecycle evidence blocks accidental deletion of published content.
	repo.lifecycle.Published = &authoring.Published{Revision: created.Revision}
	if _, err := svc.Execute(t.Context(), "project:test", command); !errors.Is(err, authoring.ErrConflict) {
		t.Fatalf("published draft deletion = %v", err)
	}
	repo.lifecycle.Published = nil
	other := command
	other.Provenance.ActorID = "another-editor"
	if _, err := svc.Execute(t.Context(), "project:test", other); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("other owner's deletion = %v", err)
	}
	result, err := svc.Execute(t.Context(), "project:test", command)
	if err != nil || repo.lifecycle.ID != "" {
		t.Fatalf("owner deletion = %#v, %v", result, err)
	}
	if _, err := svc.Execute(t.Context(), "project:test", command); err != nil {
		t.Fatalf("authorized replay = %v", err)
	}
	canEdit = false
	if _, err := svc.Execute(t.Context(), "project:test", command); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked replay = %v", err)
	}
}
