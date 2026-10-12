package http

import (
	"context"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access"
	"github.com/flidai/leapview/internal/dashboard/authoring"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

func TestBuilderDeleteUsesCanonicalDomainPolicyAndPreservesDenial(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprintf("denied_%t", denied), func(t *testing.T) {
			fake := &builderAuthoringFake{}
			if denied {
				fake.err = access.ErrForbidden
			}
			prechecks := 0
			handler := Handler{Authoring: fake, CurrentPrincipalID: func(*nethttp.Request) string { return "owner" }, AuthorizeTypedDashboardAction: func(context.Context, projectgraph.ResourceID, projectgraph.ResourceID, access.Action) (bool, bool, error) {
				prechecks++
				return true, false, nil
			}}
			req := builderRequest(nethttp.MethodPost, "/dashboards/revenue/draft/command", map[string]any{"builderCommand": map[string]any{"dashboardId": "revenue", "draftId": "draft-1", "revisionId": "revision-1", "revisionNumber": "7", "revisionContentHash": "sha256:" + strings.Repeat("a", 64), "action": "delete"}})
			req.Header.Set("X-LeapView-Operation-ID", dashboardBuilderOperationID)
			recorder := httptest.NewRecorder()
			handler.DashboardBuilderCommand(recorder, withBuilderURLParams(req, "sales", "revenue"))
			wantStatus := nethttp.StatusOK
			if denied {
				wantStatus = nethttp.StatusForbidden
			}
			if recorder.Code != wantStatus || fake.executeCalls != 1 || prechecks != 0 {
				t.Fatalf("status=%d domain calls=%d exact prechecks=%d body=%s", recorder.Code, fake.executeCalls, prechecks, recorder.Body.String())
			}
			if denied && strings.Contains(recorder.Body.String(), "redirectTo") {
				t.Fatal("denied deletion redirected as successful")
			}
		})
	}
}

func TestDeleteCommandHasExactTypedActionForAPI(t *testing.T) {
	action, ok := authoringCommandTypedAction(authoring.Command{Delete: &authoring.DeletePayload{}})
	if !ok || action != access.ActionDashboardDelete {
		t.Fatalf("delete action = %q, %t", action, ok)
	}
}
