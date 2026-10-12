package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flidai/leapview/internal/dashboard/authoring"
	"github.com/flidai/leapview/internal/dashboard/authoring/application"
	"github.com/flidai/leapview/internal/dashboard/document"
	projectruntime "github.com/flidai/leapview/internal/project/runtime"
	configschema "github.com/flidai/leapview/internal/project/schema"
)

func TestSourceEditCommitsCanonicalYAMLAtomicallyAndReplays(t *testing.T) {
	provenance := authoring.Provenance{Origin: authoring.OriginAgent, ActorID: "agent", ConversationID: "conversation", ToolCallID: "edit-source"}
	doc := document.DashboardDocument{
		APIVersion: document.DashboardApiVersionLeapviewDevV1,
		Kind:       document.DashboardResourceKindDashboard,
		Metadata:   document.DashboardMetadata{ID: "dashboard", Name: "dashboard"},
		Spec: document.DashboardSpec{
			SemanticModel: "sales", Filters: []document.DashboardFilter{}, Visuals: map[string]document.DashboardVisual{},
			Pages: []document.DashboardPage{{ID: "overview", Title: "Overview", Components: []document.DashboardPageComponent{}}},
		},
	}
	revision, err := authoring.NewRevision("revision-1", "dashboard", 1, time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), doc, provenance)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := authoring.NewDashboardLifecycle(authoring.NewDashboardLifecycleInput{
		ProjectID: "project", ID: "dashboard", OwnerPrincipalID: "agent", Slug: "dashboard", Title: "Dashboard",
		SemanticModel: "sales", Visibility: authoring.VisibilityPrivate,
		Draft: &authoring.Draft{ID: "draft", DashboardID: "dashboard", Revision: revision.Token(), Provenance: provenance},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &applicationRepository{
		lifecycle: lifecycle, revisions: map[authoring.RevisionID]authoring.Revision{revision.ID: revision},
		commands: map[authoring.CommandID]authoring.CommandResult{}, fingerprints: map[authoring.CommandID]string{},
	}
	app, err := application.New(application.Options{
		Authoring: newApplicationService(t, repo, &applicationAuthorizer{}), Repository: repo, Authorizer: &applicationAuthorizer{},
		AcquireRuntime: func(context.Context) (projectruntime.Lease, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	read, err := app.ReadSource(t.Context(), application.DraftRequest{ProjectID: "project", ActorID: "agent", DashboardID: "dashboard"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.YAML, "title: Overview") || read.Revision != revision.Token() {
		t.Fatalf("source read = %#v", read)
	}
	request := application.SourceEditRequest{
		ProjectID: "project", ActorID: "agent", DashboardID: "dashboard", DraftID: "draft", ExpectedRevision: revision.Token(),
		Edits:     []application.SourceEdit{{OldText: "title: Overview", NewText: "title: Executive overview"}},
		CommandID: "edit-source", Provenance: provenance,
	}
	result, err := app.EditSource(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision.Number != 2 || repo.appendCalls != 1 || !strings.Contains(result.YAML, "title: Executive overview") || !strings.Contains(result.Diff, "+          title: Executive overview") {
		t.Fatalf("source edit = %#v, appends=%d", result, repo.appendCalls)
	}
	replay, err := app.EditSource(t.Context(), request)
	if err != nil || replay.Revision != result.Revision || repo.appendCalls != 1 {
		t.Fatalf("source edit replay = %#v, err=%v, appends=%d", replay, err, repo.appendCalls)
	}
	stale := request
	stale.CommandID = "different-command"
	stale.Provenance.ToolCallID = "different-command"
	if _, err := app.EditSource(t.Context(), stale); !errors.Is(err, authoring.ErrStaleRevision) {
		t.Fatalf("stale source edit error = %v", err)
	}
	if repo.appendCalls != 1 {
		t.Fatalf("stale edit appended %d revisions", repo.appendCalls)
	}
}

// Use the frozen nonempty v1 revision shared with compiler qualification. Source
// edits must reject unsupported syntax before the authoring transaction appends
// anything, even when an existing visual mark could suggest a fallback query.
func TestSourceEditRejectsFutureContractWithoutChangingRetainedDraft(t *testing.T) {
	for _, kind := range []string{"version", "query", "presentation"} {
		t.Run(kind, func(t *testing.T) {
			app, repo, retained, provenance := retainedSourceEditFixture(t)
			read, err := app.ReadSource(t.Context(), application.DraftRequest{ProjectID: "project", ActorID: "agent", DashboardID: "sales"})
			if err != nil {
				t.Fatal(err)
			}
			edited := read.YAML
			diagnostic := ""
			switch kind {
			case "version":
				edited = strings.Replace(edited, "leapview.dev/v1", "leapview.dev/v2", 1)
				diagnostic = "apiVersion"
			case "query":
				edited = strings.Replace(edited, "type: aggregate", "type: futureQuery", 1)
				diagnostic = "query"
			case "presentation":
				for _, line := range strings.Split(edited, "\n") {
					if strings.TrimSpace(line) == "smooth: false" {
						indentation := strings.TrimSuffix(line, "smooth: false")
						edited = strings.Replace(edited, line, line+"\n"+indentation+"futureCapability: true", 1)
						break
					}
				}
				diagnostic = "presentation"
			}
			if edited == read.YAML {
				t.Fatal("test mutation did not change the source")
			}
			beforeLifecycle, err := json.Marshal(repo.lifecycle)
			if err != nil {
				t.Fatal(err)
			}
			beforeRevision, err := json.Marshal(repo.revisions[retained.ID])
			if err != nil {
				t.Fatal(err)
			}
			_, err = app.EditSource(t.Context(), application.SourceEditRequest{
				ProjectID: "project", ActorID: "agent", DashboardID: "sales", DraftID: "draft", ExpectedRevision: retained.Token(),
				CommandID: "edit-source", Provenance: provenance,
				Edits: []application.SourceEdit{{OldText: read.YAML, NewText: edited}},
			})
			if !errors.Is(err, authoring.ErrInvalidPayload) || !strings.Contains(err.Error(), diagnostic) {
				t.Fatalf("unsupported %s source error = %v", kind, err)
			}
			afterLifecycle, err := json.Marshal(repo.lifecycle)
			if err != nil {
				t.Fatal(err)
			}
			afterRevision, err := json.Marshal(repo.revisions[retained.ID])
			if err != nil {
				t.Fatal(err)
			}
			if repo.appendCalls != 0 || len(repo.revisions) != 1 || len(repo.commands) != 0 ||
				string(afterLifecycle) != string(beforeLifecycle) || string(afterRevision) != string(beforeRevision) ||
				repo.lifecycle.Draft.Revision != retained.Token() {
				t.Fatal("rejected source changed retained revision, hash, draft token, or command history")
			}
			reread, err := app.ReadSource(t.Context(), application.DraftRequest{ProjectID: "project", ActorID: "agent", DashboardID: "sales"})
			if err != nil {
				t.Fatal(err)
			}
			if reread != read {
				t.Fatal("rejected source changed the canonical source read")
			}
		})
	}
}

func TestSourceEditPreservesSchemaDiagnosticsAndAllowsCanonicalCorrection(t *testing.T) {
	app, repo, retained, provenance := retainedSourceEditFixture(t)
	read, err := app.ReadSource(t.Context(), application.DraftRequest{ProjectID: "project", ActorID: "agent", DashboardID: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	rejected := strings.Replace(read.YAML, "leapview.dev/v1", "leapview.dev/v2", 1)
	var candidate document.DashboardDocument
	decodeErr := configschema.DecodeResource(configschema.KindDashboard, "dashboard.yaml", []byte(rejected), &candidate)
	if decodeErr == nil {
		t.Fatal("unsupported dashboard version passed schema validation")
	}
	wantDiagnostics := configschema.Diagnostics(decodeErr)
	if len(wantDiagnostics) == 0 || wantDiagnostics[0].FieldPath != "apiVersion" ||
		wantDiagnostics[0].File != "dashboard.yaml" || wantDiagnostics[0].Line <= 0 || wantDiagnostics[0].Column <= 0 {
		t.Fatalf("canonical schema diagnostics lack source coordinates: %#v", wantDiagnostics)
	}
	request := application.SourceEditRequest{
		ProjectID: "project", ActorID: "agent", DashboardID: "sales", DraftID: "draft", ExpectedRevision: read.Revision,
		CommandID: "edit-source", Provenance: provenance,
		Edits: []application.SourceEdit{{OldText: read.YAML, NewText: rejected}},
	}
	_, err = app.EditSource(t.Context(), request)
	if !errors.Is(err, authoring.ErrInvalidPayload) {
		t.Fatalf("rejected source error = %v, want invalid payload", err)
	}
	if got := configschema.Diagnostics(err); !reflect.DeepEqual(got, wantDiagnostics) {
		t.Fatalf("source edit diagnostics = %#v, want canonical schema diagnostics %#v", got, wantDiagnostics)
	}
	reread, err := app.ReadSource(t.Context(), application.DraftRequest{ProjectID: "project", ActorID: "agent", DashboardID: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	if reread != read || repo.appendCalls != 0 || len(repo.revisions) != 1 || len(repo.commands) != 0 {
		t.Fatal("rejected edit changed the canonical source, revision, or command history")
	}
	corrected, err := retained.Document.Clone()
	if err != nil {
		t.Fatal(err)
	}
	smooth := true
	corrected.Spec.Visuals["trend"].Presentation.Value.(*document.CartesianDashboardPresentation).Smooth = &smooth
	request.Edits = []application.SourceEdit{{OldText: reread.YAML, NewText: strings.Replace(reread.YAML, "smooth: false", "smooth: true", 1)}}
	result, err := app.EditSource(t.Context(), request)
	if err != nil {
		t.Fatalf("valid correction against unchanged canonical source: %v", err)
	}
	stored := repo.revisions[result.Revision.RevisionID]
	if repo.appendCalls != 1 || result.Revision.Number != retained.Number+1 ||
		!reflect.DeepEqual(stored.Document, corrected) || repo.lifecycle.Draft.Revision != result.Revision {
		t.Fatal("corrected complete dashboard did not preserve its bound queries and placements in one new revision")
	}
}

func TestSourceEditPresentationChangePreservesRetainedQueryAndPlacements(t *testing.T) {
	app, repo, retained, provenance := retainedSourceEditFixture(t)
	read, err := app.ReadSource(t.Context(), application.DraftRequest{ProjectID: "project", ActorID: "agent", DashboardID: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(retained)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := retained.Document.Clone()
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.EditSource(t.Context(), application.SourceEditRequest{
		ProjectID: "project", ActorID: "agent", DashboardID: "sales", DraftID: "draft", ExpectedRevision: retained.Token(),
		CommandID: "edit-source", Provenance: provenance,
		Edits: []application.SourceEdit{{OldText: "smooth: false", NewText: "smooth: true"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	smooth := true
	expected.Spec.Visuals["trend"].Presentation.Value.(*document.CartesianDashboardPresentation).Smooth = &smooth
	stored := repo.revisions[result.Revision.RevisionID]
	if repo.appendCalls != 1 || result.Revision.Number != retained.Number+1 || result.Revision.ContentHash == retained.ContentHash ||
		!reflect.DeepEqual(stored.Document, expected) || repo.lifecycle.Draft.Revision != stored.Token() {
		t.Fatal("presentation source edit changed neighboring authored meaning or failed to append exactly once")
	}
	if err := stored.Validate(); err != nil {
		t.Fatal(err)
	}
	// The canonical source read must expose the committed edit while the
	// original retained revision remains unchanged.
	reread, err := app.ReadSource(t.Context(), application.DraftRequest{ProjectID: "project", ActorID: "agent", DashboardID: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	if reread.Revision != result.Revision || reread.YAML != result.YAML || !strings.Contains(reread.YAML, "smooth: true") || !strings.Contains(read.YAML, "smooth: false") {
		t.Fatal("canonical source read did not preserve the presentation edit")
	}
	stillRetained, err := json.Marshal(repo.revisions[retained.ID])
	if err != nil {
		t.Fatal(err)
	}
	if string(stillRetained) != string(original) {
		t.Fatal("source edit mutated the earlier retained revision")
	}
}

func retainedSourceEditFixture(t *testing.T) (*application.Application, *applicationRepository, authoring.Revision, authoring.Provenance) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "compileradapter", "testdata", "retained-v1-revision.json"))
	if err != nil {
		t.Fatal(err)
	}
	var retained authoring.Revision
	if err := json.Unmarshal(content, &retained); err != nil {
		t.Fatal(err)
	}
	if err := retained.Validate(); err != nil {
		t.Fatal(err)
	}
	provenance := authoring.Provenance{Origin: authoring.OriginAgent, ActorID: "agent", ConversationID: "conversation", ToolCallID: "edit-source"}
	lifecycle, err := authoring.NewDashboardLifecycle(authoring.NewDashboardLifecycleInput{
		ProjectID: "project", ID: retained.DashboardID, OwnerPrincipalID: "agent", Slug: "sales", Title: "Sales",
		SemanticModel: "sales_model", Visibility: authoring.VisibilityPrivate,
		Draft: &authoring.Draft{ID: "draft", DashboardID: retained.DashboardID, Revision: retained.Token(), Provenance: provenance},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &applicationRepository{lifecycle: lifecycle, revisions: map[authoring.RevisionID]authoring.Revision{retained.ID: retained},
		commands: map[authoring.CommandID]authoring.CommandResult{}, fingerprints: map[authoring.CommandID]string{}}
	app, err := application.New(application.Options{Authoring: newApplicationService(t, repo, &applicationAuthorizer{}), Repository: repo, Authorizer: &applicationAuthorizer{},
		AcquireRuntime: func(context.Context) (projectruntime.Lease, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	return app, repo, retained, provenance
}
