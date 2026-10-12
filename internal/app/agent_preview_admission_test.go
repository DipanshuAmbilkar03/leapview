package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	accessmodule "github.com/flidai/leapview/internal/access/module"
	agentmodule "github.com/flidai/leapview/internal/agent/module"
	agenttools "github.com/flidai/leapview/internal/agent/tools"
	previewservice "github.com/flidai/leapview/internal/dashboard/authoring/preview"
	"github.com/flidai/leapview/internal/workload"
	agentcore "github.com/flidai/leapview/pkg/agent"
)

// A raw preview runtime cannot rely on the admitted metrics decorator. Exercise
// the real tool adapter from an unadmitted drawer context, including rejection.
type admissionPreviewApplication struct {
	agenttools.DashboardAuthoring
	queried bool
}

func (a *admissionPreviewApplication) Preview(ctx context.Context, _ previewservice.PreviewRequest) (previewservice.Preview, error) {
	admitter, ok := workload.FromContext(ctx)
	if !ok {
		return previewservice.Preview{}, fmt.Errorf("preview has no workload admitter")
	}
	if _, _, active := workload.Current(ctx); active {
		return previewservice.Preview{}, fmt.Errorf("tool context acquired a permit before querying")
	}
	lease, err := admitter.Acquire(ctx, workload.Request{Class: workload.Interactive, PrincipalID: "viewer", Operation: "dashboard.draft.preview", EstimatedMemoryBytes: 64 << 20})
	if err != nil {
		return previewservice.Preview{}, err
	}
	defer lease.Release()
	if _, _, admitted := workload.Current(lease.Context()); !admitted {
		return previewservice.Preview{}, fmt.Errorf("physical query is unadmitted")
	}
	a.queried = true
	return previewservice.Preview{}, nil
}
func TestEmbeddedAgentPreviewCarriesAdmissionAndHonorsRejection(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprintf("reject=%t", reject), func(t *testing.T) {
			controller, err := workload.New(workload.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			if reject {
				controller.Close()
			} else {
				t.Cleanup(controller.Close)
			}
			application := &admissionPreviewApplication{}
			scope := agentmodule.Scope{ProjectID: "project:analytics", PrincipalID: "viewer"}
			ctx := withAgentToolContext(t.Context(), accessmodule.Principal{ID: "viewer"}, scope, controller)
			tools := (agenttools.DashboardAuthoringProvider{Application: application, ProjectID: "project:analytics"}).Definitions(agenttools.Scope{PrincipalID: "viewer"})
			for _, tool := range tools {
				if tool.Name != agenttools.PreviewDashboardDraftToolName {
					continue
				}
				result, err := tool.Handler.Run(ctx, agentcore.ToolCall{ID: "preview", Name: tool.Name, Arguments: json.RawMessage(`{"dashboardId":"dashboard_sales","draftId":"0198f2c0-7c7a-7f00-8a11-000000000001","expectedRevision":{"revisionId":"revision_1","number":1,"contentHash":"hash"},"page":"overview"}`)})
				if err != nil {
					t.Fatal(err)
				}
				if result.IsError != reject || application.queried == reject {
					t.Fatalf("preview result=%#v queried=%t reject=%t", result, application.queried, reject)
				}
				return
			}
			t.Fatal("preview tool not found")
		})
	}
}

func TestAgentToolContextPreservesDurableWorkerAdmission(t *testing.T) {
	controller, err := workload.New(workload.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	outer, err := controller.Acquire(t.Context(), workload.Request{Class: workload.Background, PrincipalID: "viewer", Operation: "agent.run", EstimatedMemoryBytes: 64 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Release()
	scope := agentmodule.Scope{ProjectID: "project:analytics", PrincipalID: "viewer"}
	ctx := withAgentToolContext(outer.Context(), accessmodule.Principal{ID: "viewer"}, scope, controller)
	class, principal, admitted := workload.Current(ctx)
	if !admitted || class != workload.Background || principal != "viewer" {
		t.Fatalf("worker admission lost: %s %s %t", class, principal, admitted)
	}
	if _, attached := workload.FromContext(ctx); attached {
		t.Fatal("already admitted worker acquired an extra query admitter")
	}
}
