package cli

import (
	"strings"
	"testing"

	"github.com/flidai/leapview/internal/access"
)

func validAccessTransitionRequest() StageAccessTransitionRequest {
	return StageAccessTransitionRequest{
		Intent: AccessTransitionIntent{
			TargetID: "target_demo", Environment: "production",
			ProjectID:              "project_demo",
			ExpectedPolicyRevision: 32, ExpectedPolicyDigest: "sha256:" + strings.Repeat("b", 64),
			ExpectedServingGeneration: "generation_legacy", ExpectedServingPolicyDigest: "sha256:" + strings.Repeat("c", 64),
			PublisherPrincipalID: "principal_release", ReviewerPrincipalID: "principal_reviewer",
			RoleBindings: []AccessTransitionRoleIntent{{BindingID: "release-operator", Name: "release operator", Principal: "principal_release", Role: string(access.PermissionRoleReleaseOperator)}},
			Grants:       []AccessTransitionGrantIntent{{GrantID: "dashboard-viewer", Principal: "principal_cfo", ResourceID: "dashboard_revenue", ResourceKind: "dashboard", Actions: []string{string(access.ActionDashboardRead)}}},
		},
		MaintenanceOperationID:     "upgrade-42",
		MaintenanceOperationDigest: "sha256:" + strings.Repeat("a", 64),
		OperationID:                "access-transition-42",
	}
}

func TestAccessTransitionPlanIsStableAndFenced(t *testing.T) {
	request := validAccessTransitionRequest()
	first, err := request.Plan()
	if err != nil {
		t.Fatal(err)
	}
	request.Intent.Grants[0].Actions = []string{string(access.ActionDashboardRead)}
	second, err := request.Intent.Plan()
	if err != nil || second.IntentDigest != first.IntentDigest {
		t.Fatalf("same intent changed digest: %q %q err=%v", first.IntentDigest, second.IntentDigest, err)
	}
	request.MaintenanceOperationID = "different-outer-operation"
	request.MaintenanceOperationDigest = "sha256:" + strings.Repeat("d", 64)
	request.OperationID = "different-command"
	outerIndependent, err := request.Intent.Plan()
	if err != nil || outerIndependent.IntentDigest != first.IntentDigest {
		t.Fatalf("maintenance envelope changed semantic digest: %q %q err=%v", first.IntentDigest, outerIndependent.IntentDigest, err)
	}
	request.Intent.ExpectedPolicyRevision++
	third, err := request.Intent.Plan()
	if err != nil || third.IntentDigest == first.IntentDigest {
		t.Fatalf("expected revision was not bound into intent: %q %q err=%v", first.IntentDigest, third.IntentDigest, err)
	}
}

func TestStageAccessTransitionRejectsInvalidMaintenanceEnvelope(t *testing.T) {
	tests := map[string]func(*StageAccessTransitionRequest){
		"missing maintenance digest":    func(r *StageAccessTransitionRequest) { r.MaintenanceOperationDigest = "" },
		"missing maintenance operation": func(r *StageAccessTransitionRequest) { r.MaintenanceOperationID = "" },
		"missing command operation":     func(r *StageAccessTransitionRequest) { r.OperationID = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := validAccessTransitionRequest()
			mutate(&request)
			if _, err := request.Plan(); err == nil {
				t.Fatal("accepted an invalid maintenance envelope")
			}
		})
	}
}
