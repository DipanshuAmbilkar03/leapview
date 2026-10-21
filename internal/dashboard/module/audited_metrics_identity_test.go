package module

import (
	"context"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/analytics/dataquery"
	"github.com/flidai/leapview/internal/analytics/queryaudit"
	"github.com/google/uuid"
)

func TestQueryEventsHaveIndependentIdentityAndPreserveResultMetadata(t *testing.T) {
	request := dataquery.Query{ProjectID: "sales", PrincipalID: "actor", Surface: "explorer", RequestID: "same-stream", Operation: "analyze"}
	result := dataquery.Result{SQL: "select 1", RowsReturned: 1, DurationMS: 12, PlanningMS: 2, DatabaseMS: 3, CacheOutcome: dataquery.CacheHit}
	first, second := queryEventInput(request, result), queryEventInput(request, result)
	if !strings.Contains(first.QueryJSON, `"cacheOutcome":"hit"`) {
		t.Fatalf("missing cache outcome: %s", first.QueryJSON)
	}
	if _, err := uuid.Parse(first.EventID); err != nil {
		t.Fatalf("missing event identity: %v", err)
	}
	if first.EventID == second.EventID {
		t.Fatal("queries on one stream must have independent identities")
	}
	if first.Surface != "explorer" || first.SQL != "select 1" || first.RowsReturned != 1 || first.DurationMS != 12 || first.PlanningMS != 2 || first.DatabaseMS != 3 {
		t.Fatalf("lost query metadata: %#v", first)
	}
}

type identityAuditCapture struct {
	events     []queryaudit.EventInput
	contextErr error
}

func (r *identityAuditCapture) RecordQueryEvent(ctx context.Context, event queryaudit.EventInput) error {
	r.contextErr = ctx.Err()
	r.events = append(r.events, event)
	return nil
}
func TestInteractiveAuditWithoutHTTPRequestRecordsCachedExplorerAndDashboardQueries(t *testing.T) {
	recorder := &identityAuditCapture{}
	metrics := auditedMetrics{recorder: recorder, principalID: func(context.Context) (string, bool) { return "actor", true }}
	for _, surface := range []string{"explorer", "dashboard"} {
		ctx := metrics.auditContext(context.Background())
		query := dataquery.Query{ProjectID: "sales", Surface: surface, Operation: "interactive", ModelID: "sales", Kind: dataquery.KindModelRows, Target: "orders"}
		_, err := dataquery.ExecuteAudited(ctx, query, func(context.Context, dataquery.Query) (dataquery.Result, error) {
			return dataquery.Result{SQL: "select 1", Rows: []dataquery.Row{{"value": 1}}, CacheOutcome: dataquery.CacheHit}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.events) != 2 {
		t.Fatalf("events = %#v", recorder.events)
	}
	for _, event := range recorder.events {
		if _, err := uuid.Parse(event.EventID); err != nil {
			t.Fatal(err)
		}
		if event.RequestID != "" || event.PrincipalID != "actor" || event.RowsReturned != 1 || event.Status != "success" {
			t.Fatalf("lost interactive metadata: %#v", event)
		}
		if !strings.Contains(event.QueryJSON, `"cacheOutcome":"hit"`) {
			t.Fatalf("cache outcome missing: %s", event.QueryJSON)
		}
	}
}

func TestCanceledInteractiveRequestStillRecordsQueryHistory(t *testing.T) {
	capture := &identityAuditCapture{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (queryEventRecorder{repo: capture}).RecordDataQuery(ctx, dataquery.Query{}, dataquery.Result{Status: dataquery.StatusCanceled}); err != nil {
		t.Fatal(err)
	}
	if capture.contextErr != nil || len(capture.events) != 1 {
		t.Fatalf("canceled audit: %v, %d events", capture.contextErr, len(capture.events))
	}
}
