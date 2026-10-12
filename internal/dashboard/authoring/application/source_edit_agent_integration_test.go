package application_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/access"
	agenttools "github.com/flidai/leapview/internal/agent/tools"
	semanticmodel "github.com/flidai/leapview/internal/analytics/model"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	"github.com/flidai/leapview/internal/dashboard/authoring/application"
	authoringservice "github.com/flidai/leapview/internal/dashboard/authoring/service"
	"github.com/flidai/leapview/internal/dashboard/compiler"
	"github.com/flidai/leapview/internal/dashboard/document"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
	projectruntime "github.com/flidai/leapview/internal/project/runtime"
	configschema "github.com/flidai/leapview/internal/project/schema"
	agentcore "github.com/flidai/leapview/pkg/agent"
	toon "github.com/toon-format/toon-go"
)

// Exercise the production provider and default model serialization together.
// Creation uses the production authoring service and an empty in-memory
// repository. There is no query runtime, so the final assertion compiles the
// persisted document rather than faking a successful query preview.
func TestAgentSourceEditRecoversFromRejectedCandidateUsingCanonicalToolResults(t *testing.T) {
	_, _, blueprint, _ := retainedSourceEditFixture(t)
	repo := &sourceEditAgentRepository{applicationRepository: &applicationRepository{requireAudit: true}}
	sequence := 0
	service, err := authoringservice.NewService(authoringservice.Options{
		Repository: repo, Authorizer: &applicationAuthorizer{}, Compiler: applicationCompiler{}, Now: func() time.Time { return time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC) },
		NewDashboardID: func() (authoring.DashboardID, error) { return "created-dashboard", nil },
		NewDraftID:     func() (authoring.DraftID, error) { return "created-draft", nil },
		NewRevisionID: func() (authoring.RevisionID, error) {
			sequence++
			return authoring.RevisionID(fmt.Sprintf("revision-%d", sequence)), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(application.Options{Authoring: service, Repository: repo, Authorizer: &applicationAuthorizer{}, AcquireRuntime: func(context.Context) (projectruntime.Lease, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	tools := (agenttools.DashboardAuthoringProvider{Application: app, ProjectID: "project", Resolve: func(_ context.Context, _ agenttools.Scope, id projectgraph.ResourceID, kind projectgraph.Kind, capability access.Capability) (projectgraph.ResourceID, error) {
		if id != "sales_model" || kind != projectgraph.KindSemanticModel || capability != access.CapabilityResourceUse {
			return "", access.ErrForbidden
		}
		return id, nil
	}}).Definitions(agenttools.Scope{PrincipalID: "agent", ConversationID: "conversation"})
	var original, latest application.SourceRead
	var retained authoring.Revision
	var rejected, complete string
	var wanted document.DashboardDocument
	var receipt authoring.ResourceCreateReceipt
	step := 0
	model := agentcore.ModelFunc(func(_ context.Context, request agentcore.ModelRequest, _ agentcore.ModelStream) (agentcore.ModelResponse, error) {
		call := func(id, name string, input any) (agentcore.ModelResponse, error) {
			arguments, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			return agentcore.ModelResponse{FinishReason: agentcore.FinishReasonToolCalls, ToolCalls: []agentcore.ToolCall{{ID: id, Name: name, Arguments: arguments}}}, nil
		}
		edit := func(id string, source application.SourceRead, oldText, newText string) (agentcore.ModelResponse, error) {
			return call(id, agenttools.EditDashboardSourceToolName, map[string]any{
				"dashboardId": source.DashboardID, "draftId": source.DraftID, "expectedRevision": source.Revision,
				"edits": []application.SourceEdit{{OldText: oldText, NewText: newText}},
			})
		}
		lastTool := func(name string, isError bool, destination any) {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != agentcore.RoleTool || last.ToolName != name || last.IsError != isError || last.DisplayContent != nil {
				t.Fatalf("model received unexpected tool message: %#v", last)
			}
			decoded, err := toon.Decode([]byte(last.Content))
			if err != nil {
				t.Fatalf("decode default model tool result: %v", err)
			}
			encoded, err := json.Marshal(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, destination); err != nil {
				t.Fatalf("decode typed model tool result: %v", err)
			}
		}
		recovery := func(message string) application.SourceRead {
			var result struct {
				Error struct {
					Code        string                    `json:"code"`
					Message     string                    `json:"message"`
					Diagnostics []configschema.Diagnostic `json:"diagnostics"`
				} `json:"error"`
				Recovery struct {
					Instructions  string                  `json:"instructions"`
					CurrentSource *application.SourceRead `json:"currentSource"`
				} `json:"recovery"`
			}
			lastTool(agenttools.EditDashboardSourceToolName, true, &result)
			if result.Error.Code != "invalid_arguments" || !strings.Contains(result.Error.Message, message) ||
				result.Recovery.Instructions == "" || result.Recovery.CurrentSource == nil {
				t.Fatalf("failed edit lacks actionable canonical recovery: %#v", result)
			}
			if *result.Recovery.CurrentSource != original || repo.appendCalls != 0 || len(repo.commands) != 0 || len(repo.revisions) != 1 {
				t.Fatal("failed edit changed persisted source or returned rejected candidate as current source")
			}
			if message == "decode edited dashboard source" && (len(result.Error.Diagnostics) == 0 ||
				result.Error.Diagnostics[0].FieldPath != "apiVersion" || result.Error.Diagnostics[0].Line <= 0 || result.Error.Diagnostics[0].Column <= 0) {
				t.Fatalf("schema rejection lost its structured source diagnostic: %#v", result.Error.Diagnostics)
			}
			return *result.Recovery.CurrentSource
		}
		current := step
		step++
		switch current {
		case 0:
			return call("create-dashboard", agenttools.CreateDashboardDraftToolName, map[string]any{"title": "Executive Sales", "semanticModelId": "sales_model"})
		case 1:
			lastTool(agenttools.CreateDashboardDraftToolName, false, &receipt)
			if receipt.ID == "" || receipt.Status != "draft" || repo.createCalls != 1 {
				t.Fatalf("production creation failed: %#v", receipt)
			}
			return call("read-original", agenttools.ReadDashboardSourceToolName, map[string]any{"dashboardId": receipt.ID})
		case 2:
			lastTool(agenttools.ReadDashboardSourceToolName, false, &original)
			retained = repo.revisions[original.Revision.RevisionID]
			if original.DashboardID.String() != receipt.ID || original.Revision.Number != 1 || len(retained.Document.Spec.Visuals) != 0 {
				t.Fatalf("source tool returned incomplete canonical draft: %#v", original)
			}
			wanted, err = blueprint.Document.Clone()
			if err != nil {
				t.Fatal(err)
			}
			wanted.Metadata = retained.Document.Metadata
			smooth := true
			wanted.Spec.Visuals["trend"].Presentation.Value.(*document.CartesianDashboardPresentation).Smooth = &smooth
			encoded, err := document.EncodeYAML(wanted)
			if err != nil {
				t.Fatal(err)
			}
			complete = string(encoded)
			rejected = strings.Replace(complete, "leapview.dev/v1", "leapview.dev/v2", 1)
			return edit("schema-rejected", original, original.YAML, rejected)
		case 3:
			source := recovery("decode edited dashboard source")
			// Reproduce the mistaken retry against text from the rejected edit.
			return edit("candidate-missing", source, rejected, complete)
		case 4:
			source := recovery("oldText was not found")
			return edit("canonical-correction", source, source.YAML, complete)
		case 5:
			var result application.SourceEditResult
			lastTool(agenttools.EditDashboardSourceToolName, false, &result)
			if result.Revision.Number != retained.Number+1 || !strings.Contains(result.YAML, "smooth: true") || repo.appendCalls != 1 {
				t.Fatalf("canonical correction did not commit: %#v", result)
			}
			latest = application.SourceRead{DashboardID: original.DashboardID, DraftID: original.DraftID, Revision: result.Revision, YAML: result.YAML}
			return call("read-completed", agenttools.ReadDashboardSourceToolName, map[string]any{"dashboardId": latest.DashboardID})
		case 6:
			var result application.SourceRead
			lastTool(agenttools.ReadDashboardSourceToolName, false, &result)
			if result != latest {
				t.Fatal("final source read differs from committed correction")
			}
			return agentcore.ModelResponse{Content: "Dashboard draft created.", FinishReason: agentcore.FinishReasonStop}, nil
		default:
			t.Fatalf("unexpected model turn %d", current)
			return agentcore.ModelResponse{}, nil
		}
	})
	harness, err := agentcore.New(agentcore.Definition{Name: "dashboard-authoring-recovery", SystemPrompt: "Use canonical tool results for dashboard authoring.", Model: model, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	result, err := harness.Prompt(t.Context(), agentcore.PromptRequest{Input: "Create a complete executive sales dashboard."})
	if err != nil || result.StopReason != agentcore.StopReasonCompleted || result.ToolCalls != 6 {
		t.Fatalf("agent authoring result = %#v, err=%v", result, err)
	}
	if repo.createCalls != 1 || repo.appendCalls != 1 || len(repo.revisions) != 2 || len(repo.commands) != 1 || repo.lifecycle.ID != retained.DashboardID || repo.lifecycle.Visibility != authoring.VisibilityPrivate {
		t.Fatal("retry loop persisted a failed write or changed dashboard identity/visibility")
	}
	stored := repo.revisions[latest.Revision.RevisionID]
	if !reflect.DeepEqual(stored.Document, wanted) || repo.revisions[retained.ID].ContentHash != retained.ContentHash {
		t.Fatal("successful correction changed bound queries/layout or an earlier immutable revision")
	}
	compiled, err := compiler.CompileDocument(stored.Document, map[string]*semanticmodel.Model{"sales_model": sourceEditAgentSemanticModel()})
	if err != nil {
		t.Fatalf("compile corrected complete dashboard: %v", err)
	}
	trend := compiled.Definition.Visualizations["trend"].Query.Aggregate
	if len(compiled.Definition.Visualizations) != 2 || len(compiled.Definition.Pages) != 2 ||
		trend == nil || trend.TableID != "orders" || len(trend.Metrics) != 1 || trend.Metrics[0].FieldID != "order_count" ||
		len(trend.Dimensions) != 1 || trend.Dimensions[0].FieldID != "purchaseDate" || trend.Dimensions[0].Grain != "month" ||
		trend.Series == nil || trend.Series.FieldID != "status" || len(compiled.Definition.Pages[0].Visuals) != 2 {
		t.Fatalf("completed dashboard lost governed bindings or page placements: %#v", compiled.Definition)
	}
}

type sourceEditAgentRepository struct {
	*applicationRepository
	createCalls int
	operation   authoring.CreateOperation
}

func (r *sourceEditAgentRepository) Create(ctx context.Context, input authoring.CreateInput) (authoring.DashboardLifecycle, error) {
	if r.lifecycle.ID != "" {
		return authoring.DashboardLifecycle{}, authoring.ErrConflict
	}
	intent, ok := authoring.AuditIntentFromContext(ctx)
	if !ok || intent.EventID != input.Operation.IdempotencyKey {
		return authoring.DashboardLifecycle{}, fmt.Errorf("create requires matching command audit intent")
	}
	if err := input.Lifecycle.Validate(); err != nil {
		return authoring.DashboardLifecycle{}, err
	}
	if err := input.Revision.Validate(); err != nil {
		return authoring.DashboardLifecycle{}, err
	}
	if err := input.Operation.Validate(); err != nil {
		return authoring.DashboardLifecycle{}, err
	}
	r.createCalls++
	r.lifecycle = input.Lifecycle
	r.revisions = map[authoring.RevisionID]authoring.Revision{input.Revision.ID: input.Revision}
	r.operation = input.Operation
	return r.lifecycle, nil
}

func (r *sourceEditAgentRepository) LookupCreateOperation(_ context.Context, operation authoring.CreateOperation) (authoring.CreateOperationResult, bool, error) {
	if r.createCalls == 0 || operation.IdempotencyKey != r.operation.IdempotencyKey {
		return authoring.CreateOperationResult{}, false, nil
	}
	return authoring.CreateOperationResult{DashboardID: r.lifecycle.ID, Revision: r.revisions["revision-1"].Token(), Fingerprint: r.operation.Fingerprint}, true, nil
}

func sourceEditAgentSemanticModel() *semanticmodel.Model {
	return &semanticmodel.Model{
		Name:     "sales",
		Datasets: map[string]semanticmodel.SemanticDatasetSpec{"orders": {Model: "orders"}},
		Tables: map[string]semanticmodel.Table{"orders": {
			ModelName: "orders", GrainEntity: "status",
			Entities: map[string]semanticmodel.EntityDefinition{"status": {Type: "primary", Fields: []string{"status"}}},
			Dimensions: map[string]semanticmodel.MetricDimension{
				"status":     {Field: "orders.status", Type: "string", Datatype: semanticmodel.DataTypeString},
				"ordered_at": {Type: "timestamp", Datatype: semanticmodel.DataTypeDateTimeTZ},
			},
		}},
		Dimensions: map[string]semanticmodel.SemanticDimension{
			"status": {Type: "string", Datatype: semanticmodel.DataTypeString, Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.status"}}},
			"purchaseDate": {Type: "timestamp", Datatype: semanticmodel.DataTypeDateTimeTZ, NativeGrain: "day", Grains: []string{"day", "month"},
				Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.ordered_at"}}},
		},
		Metrics: map[string]semanticmodel.Metric{"order_count": {Type: "aggregate", Dataset: "orders", Aggregation: "count", Input: &semanticmodel.MetricInput{Field: "orders.status"}, Empty: "zero"}},
	}
}
