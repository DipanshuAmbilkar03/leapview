package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	dashboardauthoring "github.com/flidai/leapview/internal/dashboard/authoring"
	authoringapplication "github.com/flidai/leapview/internal/dashboard/authoring/application"
	agentcore "github.com/flidai/leapview/pkg/agent"
)

type sourceRecoveryAuthoring struct {
	projectAuthoringFake
	failure     error
	readFailure error
	reads       []authoringapplication.DraftRequest
}

func (f *sourceRecoveryAuthoring) EditSource(context.Context, authoringapplication.SourceEditRequest) (authoringapplication.SourceEditResult, error) {
	return authoringapplication.SourceEditResult{}, f.failure
}
func (f *sourceRecoveryAuthoring) ReadSource(_ context.Context, request authoringapplication.DraftRequest) (authoringapplication.SourceRead, error) {
	f.reads = append(f.reads, request)
	return f.source, f.readFailure
}

func TestSourceEditFailureReturnsAuthorizedCanonicalRecoveryToModel(t *testing.T) {
	for _, failure := range []error{dashboardauthoring.ErrInvalidPayload, dashboardauthoring.ErrInvalidAuthoring, dashboardauthoring.ErrStaleRevision} {
		t.Run(failure.Error(), func(t *testing.T) {
			app := &sourceRecoveryAuthoring{failure: failure, projectAuthoringFake: projectAuthoringFake{source: authoringapplication.SourceRead{DashboardID: "dashboard_sales", DraftID: "current_draft", Revision: dashboardauthoring.RevisionToken{RevisionID: "current_revision", Number: 3, ContentHash: "current_hash"}, YAML: "    title: Saved title\n"}}}
			result := runSourceRecoveryTool(t, app)
			data, err := json.Marshal(result.ModelContent)
			if err != nil {
				t.Fatal(err)
			}
			var recovery struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
				Recovery struct {
					CurrentSource authoringapplication.SourceRead `json:"currentSource"`
				} `json:"recovery"`
			}
			if err := json.Unmarshal(data, &recovery); err != nil {
				t.Fatal(err)
			}
			if !result.IsError || recovery.Error.Code == "" || recovery.Recovery.CurrentSource != app.source {
				t.Fatalf("missing model recovery: %s", data)
			}
			if len(app.reads) != 1 || app.reads[0].ActorID != "principal" || app.reads[0].DashboardID != "dashboard_sales" || app.reads[0].ProjectID != projectIDForTest() {
				t.Fatalf("recovery read scope=%#v", app.reads)
			}
		})
	}
}

func TestSourceEditRecoveryDoesNotReadAfterUnrecoverableFailure(t *testing.T) {
	app := &sourceRecoveryAuthoring{failure: errors.New("permission denied")}
	result := runSourceRecoveryTool(t, app)
	if !result.IsError || result.ModelContent != nil || len(app.reads) != 0 {
		t.Fatalf("unrecoverable result=%#v reads=%#v", result, app.reads)
	}
}

func TestSourceEditRecoveryOmitsSourceWhenReadIsDenied(t *testing.T) {
	app := &sourceRecoveryAuthoring{failure: dashboardauthoring.ErrInvalidPayload, readFailure: errors.New("permission revoked"), projectAuthoringFake: projectAuthoringFake{source: authoringapplication.SourceRead{YAML: "private source"}}}
	result := runSourceRecoveryTool(t, app)
	data, _ := json.Marshal(result.ModelContent)
	var recovery struct {
		Recovery struct {
			CurrentSource *authoringapplication.SourceRead `json:"currentSource"`
		} `json:"recovery"`
	}
	if err := json.Unmarshal(data, &recovery); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || recovery.Recovery.CurrentSource != nil {
		t.Fatalf("unauthorized source exposed: %s", data)
	}
}

func runSourceRecoveryTool(t *testing.T, app DashboardAuthoring) agentcore.ToolResult {
	t.Helper()
	tool := definitionByName((DashboardAuthoringProvider{Application: app, ProjectID: projectIDForTest()}).Definitions(Scope{PrincipalID: "principal", ConversationID: "conversation"}), EditDashboardSourceToolName)
	result, err := tool.Handler.Run(t.Context(), agentcore.ToolCall{ID: "source-edit-recovery", Name: EditDashboardSourceToolName, Arguments: json.RawMessage(`{"dashboardId":"dashboard_sales","draftId":"draft_1","expectedRevision":{"revisionId":"revision_1","number":1,"contentHash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"edits":[{"oldText":"title: Old","newText":"title: New"}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSourceEditRecoveryBoundsCanonicalSource(t *testing.T) {
	app := &sourceRecoveryAuthoring{failure: dashboardauthoring.ErrInvalidPayload, projectAuthoringFake: projectAuthoringFake{source: authoringapplication.SourceRead{YAML: strings.Repeat("x", (32<<10)+1)}}}
	result := runSourceRecoveryTool(t, app)
	data, _ := json.Marshal(result.ModelContent)
	var recovery struct {
		Recovery struct {
			Instructions  string                           `json:"instructions"`
			CurrentSource *authoringapplication.SourceRead `json:"currentSource"`
		} `json:"recovery"`
	}
	if err := json.Unmarshal(data, &recovery); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || recovery.Recovery.CurrentSource != nil || !strings.Contains(recovery.Recovery.Instructions, "read_dashboard_source") {
		t.Fatal("oversized source has no bounded recovery path")
	}
}
