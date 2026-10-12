package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
	accessmodule "github.com/flidai/leapview/internal/access/module"
	accesssnapshot "github.com/flidai/leapview/internal/access/snapshot"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

func TestPipelineBrowserAuthorizerSeparatesReadAndRun(t *testing.T) {
	identity := projectgraph.ServingIdentity{ProjectID: "project_demo", Environment: "production", GenerationID: "generation_1"}
	graph, err := projectgraph.NewProjectGraph([]projectgraph.Resource{{ID: "pipeline_a", Kind: projectgraph.KindPipeline, Name: "a"}, {ID: "pipeline_b", Kind: projectgraph.KindPipeline, Name: "b"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	subject := access.SubjectRef{Kind: access.SubjectKindPrincipal, ID: "reader"}
	resource, err := access.NewResourceRef("pipeline_a", projectgraph.KindPipeline)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []access.Action{access.ActionPipelineRead, access.ActionPipelineRun} {
		t.Run(string(action), func(t *testing.T) {
			pair, err := access.NewExactPermissionPair(action, identity.ProjectID, resource)
			if err != nil {
				t.Fatal(err)
			}
			grant, err := accesssnapshot.NewTypedGrant("grant", "grant", subject, []access.PermissionPair{pair})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := accesssnapshot.NewAuthorizationSnapshot(identity, graph, []accesssnapshot.Grant{grant}, nil)
			if err != nil {
				t.Fatal(err)
			}
			authorizer := pipelineBrowserAuthorizer(tusAccess{principal: accessmodule.Principal{ID: subject.ID}, ok: true, subjects: []access.SubjectRef{subject}}, tusRuntime{project: identity.ProjectID, lease: tusLease{identity: identity, snapshot: snapshot}}, func(context.Context) (projectgraph.ResourceID, error) { return identity.ProjectID, nil })
			for _, capability := range []access.Capability{access.CapabilityResourceRead, access.CapabilityResourceUse} {
				allowed, err := authorizer(httptest.NewRequest(http.MethodGet, "/pipelines/pipeline_a/runs/run_a", nil), "pipeline_a", capability)
				want := action == access.ActionPipelineRead && capability == access.CapabilityResourceRead || action == access.ActionPipelineRun && capability == access.CapabilityResourceUse
				if err != nil || allowed != want {
					t.Errorf("%s: allowed=%t error=%v; want %t", capability, allowed, err, want)
				}
				allowed, err = authorizer(httptest.NewRequest(http.MethodGet, "/pipelines/pipeline_b", nil), "pipeline_b", capability)
				if err != nil || allowed {
					t.Errorf("permission crossed pipeline: allowed=%t error=%v", allowed, err)
				}
			}
			if allowed, err := authorizer(httptest.NewRequest(http.MethodGet, "/pipelines/pipeline_a", nil), "pipeline_a", access.CapabilityResourceEdit); err == nil || allowed {
				t.Fatalf("unsupported capability: allowed=%t error=%v", allowed, err)
			}
		})
	}
}
