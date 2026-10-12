package access

import (
	"fmt"
	"strings"
	"testing"
)

func validAccessTransitionIntent() AccessTransitionIntent {
	return AccessTransitionIntent{
		TargetID: "target_demo", Environment: "production",
		ProjectID:              "project_demo",
		ExpectedPolicyRevision: 32, ExpectedPolicyDigest: "sha256:" + strings.Repeat("b", 64),
		ExpectedServingGeneration: "generation_legacy", ExpectedServingPolicyDigest: "sha256:" + strings.Repeat("c", 64),
		PublisherPrincipalID: "principal_release", ReviewerPrincipalID: "principal_reviewer",
		RoleBindings: []AccessTransitionRoleIntent{{BindingID: "release-operator", Name: "release operator", Principal: "principal_release", Role: string(PermissionRoleReleaseOperator)}},
		Grants:       []AccessTransitionGrantIntent{{GrantID: "dashboard-viewer", Principal: "principal_cfo", ResourceID: "dashboard_revenue", ResourceKind: "dashboard", Actions: []string{string(ActionDashboardRead)}}},
	}
}

func TestAccessTransitionPlanBuildsOnlyTypedRoleAndExactGrants(t *testing.T) {
	request := validAccessTransitionIntent()
	plan, err := request.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RoleBindings) != 1 || !plan.RoleBindings[0].TypedRoleBinding() || plan.RoleBindings[0].PermissionRole != PermissionRoleReleaseOperator || len(plan.RoleBindings[0].Capabilities) != 0 {
		t.Fatalf("role plan is not typed catalog authority: %+v", plan.RoleBindings)
	}
	if len(plan.Grants) != 1 || plan.Grants[0].Capability != "" || plan.Grants[0].PermissionProfile != PermissionCatalogProfile || len(plan.Grants[0].Permissions) != 1 {
		t.Fatalf("grant plan is not typed authority: %+v", plan.Grants)
	}
	if plan.Grants[0].ID != "dashboard-viewer" {
		t.Fatalf("single-action grant ID changed: %q", plan.Grants[0].ID)
	}
	pair := plan.Grants[0].Permissions[0]
	if pair.Action != ActionDashboardRead || pair.Target.IncludeFuture || pair.Target.ResourceID != "dashboard_revenue" || pair.Target.ProjectID != "project_demo" {
		t.Fatalf("grant is not exact dashboard.read: %+v", pair)
	}
	if !canonicalTransitionDigest(plan.IntentDigest) {
		t.Fatalf("intent digest is not canonical: %q", plan.IntentDigest)
	}
	if plan.PublisherPrincipalID != "principal_release" || plan.ReviewerPrincipalID != "principal_reviewer" {
		t.Fatalf("publication principals are not bound: %+v", plan)
	}
}

func TestAccessTransitionPlanSplitsMultiActionGrantsDeterministically(t *testing.T) {
	request := validAccessTransitionIntent()
	request.Grants = []AccessTransitionGrantIntent{{
		GrantID: "publisher-connection", Name: "connection access",
		Principal: request.PublisherPrincipalID, ResourceID: "connection_finance", ResourceKind: "connection",
		Actions: []string{string(ActionConnectionUse), string(ActionConnectionManage)},
	}}
	first, err := request.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Grants) != 2 {
		t.Fatalf("multi-action input produced %d persisted grants, want 2", len(first.Grants))
	}
	idsByAction := make(map[Action]string, len(first.Grants))
	for _, grant := range first.Grants {
		if grant.ID == "publisher-connection" || len(grant.ID) == 0 || len(grant.ID) > 255 {
			t.Errorf("multi-action grant ID is not a bounded derived ID: %q", grant.ID)
		}
		if grant.PermissionProfile != PermissionCatalogProfile || len(grant.Permissions) != 1 {
			t.Fatalf("expanded grant must have one typed permission: %+v", grant)
		}
		pair := grant.Permissions[0]
		if pair.Target.ProjectID != "project_demo" || pair.Target.ResourceID != "connection_finance" || pair.Target.ResourceKind != "connection" || pair.Target.IncludeFuture {
			t.Errorf("expanded pair is not exact: %+v", pair)
		}
		if _, duplicate := idsByAction[pair.Action]; duplicate {
			t.Fatalf("action %q was emitted more than once", pair.Action)
		}
		idsByAction[pair.Action] = grant.ID
	}
	if len(idsByAction) != 2 || idsByAction[ActionConnectionUse] == "" || idsByAction[ActionConnectionManage] == "" {
		t.Fatalf("expanded actions = %v, want exact use and manage", idsByAction)
	}

	request.Grants[0].Actions = []string{string(ActionConnectionManage), string(ActionConnectionUse)}
	reordered, err := request.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if reordered.IntentDigest != first.IntentDigest {
		t.Fatalf("action order changed intent digest: %q != %q", reordered.IntentDigest, first.IntentDigest)
	}
	for _, grant := range reordered.Grants {
		if len(grant.Permissions) != 1 || idsByAction[grant.Permissions[0].Action] != grant.ID {
			t.Fatalf("action reordering changed its emitted grant ID: %+v", grant)
		}
	}
}

func TestAccessTransitionPlanDoesNotReorderCallerIntent(t *testing.T) {
	request := validAccessTransitionIntent()
	request.Grants[0].Actions = []string{string(ActionDashboardUpdate), string(ActionDashboardRead)}
	before := append([]string(nil), request.Grants[0].Actions...)
	if _, err := request.Plan(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(request.Grants[0].Actions, ",") != strings.Join(before, ",") {
		t.Fatalf("planning mutated caller action order: got %v, want %v", request.Grants[0].Actions, before)
	}
}

func TestAccessTransitionPlanAllowsOnlyNarrowExactConnectionManageException(t *testing.T) {
	request := validAccessTransitionIntent()
	request.Grants = []AccessTransitionGrantIntent{{
		GrantID: "publisher-connection", Principal: request.PublisherPrincipalID,
		ResourceID: "connection_finance", ResourceKind: "connection",
		Actions: []string{string(ActionConnectionManage)},
	}}
	plan, err := request.Plan()
	if err != nil {
		t.Fatalf("explicit exact connection.manage intent rejected: %v", err)
	}
	if len(plan.Grants) != 1 || len(plan.Grants[0].Permissions) != 1 || plan.Grants[0].Permissions[0].Action != ActionConnectionManage || plan.Grants[0].Permissions[0].Target.IncludeFuture {
		t.Fatalf("connection grant is not one exact typed pair: %+v", plan.Grants)
	}
	request.Grants[0].ResourceKind = "dashboard"
	if _, err := request.Plan(); err == nil {
		t.Fatal("connection.manage accepted for a non-connection resource")
	}
	request.Grants[0].ResourceKind = "connection"
	request.Grants[0].Principal = "principal_cfo"
	if _, err := request.Plan(); err == nil {
		t.Fatal("connection.manage accepted for a non-publisher principal")
	}
}

func TestAccessTransitionPlanRejectsAmbiguousOrBroadIntent(t *testing.T) {
	tests := map[string]func(*AccessTransitionIntent){
		"missing existing policy revision": func(r *AccessTransitionIntent) { r.ExpectedPolicyRevision = 0 },
		"missing expected policy digest":   func(r *AccessTransitionIntent) { r.ExpectedPolicyDigest = "" },
		"missing serving identity":         func(r *AccessTransitionIntent) { r.ExpectedServingGeneration = "" },
		"same publisher and reviewer":      func(r *AccessTransitionIntent) { r.ReviewerPrincipalID = r.PublisherPrincipalID },
		"legacy role name":                 func(r *AccessTransitionIntent) { r.RoleBindings[0].Role = "legacy_admin" },
		"project-wide grant action": func(r *AccessTransitionIntent) {
			r.Grants[0].Actions = []string{string(ActionProjectAccessManage)}
		},
		"nondelegable action": func(r *AccessTransitionIntent) {
			r.Grants[0].Actions = []string{string(ActionWorkloadDelegate)}
		},
		"wildcard resource": func(r *AccessTransitionIntent) { r.Grants[0].ResourceID = "*" },
		"duplicate grant action": func(r *AccessTransitionIntent) {
			r.Grants[0].Actions = []string{string(ActionDashboardRead), string(ActionDashboardRead)}
		},
		"multi-action grant with empty ID": func(r *AccessTransitionIntent) {
			r.Grants[0].GrantID = ""
			r.Grants[0].Actions = []string{string(ActionDashboardRead), string(ActionDashboardUpdate)}
		},
		"multi-action grant with oversized ID": func(r *AccessTransitionIntent) {
			r.Grants[0].GrantID = strings.Repeat("g", 256)
			r.Grants[0].Actions = []string{string(ActionDashboardRead), string(ActionDashboardUpdate)}
		},
		"duplicate binding identity": func(r *AccessTransitionIntent) {
			r.RoleBindings = append(r.RoleBindings, r.RoleBindings[0])
		},
		"no assignments": func(r *AccessTransitionIntent) { r.RoleBindings, r.Grants = nil, nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := validAccessTransitionIntent()
			mutate(&request)
			if _, err := request.Plan(); err == nil {
				t.Fatal("accepted invalid or ambiguous transition intent")
			}
		})
	}
}

func TestAccessTransitionPlanRejectsEmittedGrantIDCollisions(t *testing.T) {
	request := validAccessTransitionIntent()
	request.Grants = []AccessTransitionGrantIntent{{
		GrantID: "multi-action", Principal: request.PublisherPrincipalID,
		ResourceID: "connection_finance", ResourceKind: "connection",
		Actions: []string{string(ActionConnectionUse), string(ActionConnectionManage)},
	}}
	first, err := request.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Grants) != 2 {
		t.Fatalf("multi-action input produced %d grants, want 2", len(first.Grants))
	}
	generatedID := first.Grants[0].ID
	request.Grants = append(request.Grants, AccessTransitionGrantIntent{
		GrantID: generatedID, Principal: "principal_other", ResourceID: "dashboard_revenue", ResourceKind: "dashboard",
		Actions: []string{string(ActionDashboardRead)},
	})
	if _, err := request.Plan(); err == nil {
		t.Fatal("accepted collision between a derived multi-action ID and an explicit grant ID")
	}
}

func TestAccessTransitionPlanRejectsDuplicateAuthorityAndAssignmentOverflow(t *testing.T) {
	t.Run("duplicate role authority", func(t *testing.T) {
		request := validAccessTransitionIntent()
		duplicate := request.RoleBindings[0]
		duplicate.BindingID = "second-viewer-binding"
		request.RoleBindings = append(request.RoleBindings, duplicate)
		if _, err := request.Plan(); err == nil {
			t.Fatal("accepted the same principal and role under two binding IDs")
		}
	})

	t.Run("duplicate typed pair authority", func(t *testing.T) {
		request := validAccessTransitionIntent()
		duplicate := request.Grants[0]
		duplicate.GrantID = "second-dashboard-grant"
		request.Grants = append(request.Grants, duplicate)
		if _, err := request.Plan(); err == nil {
			t.Fatal("accepted the same principal and exact action/resource under two grant IDs")
		}
	})

	t.Run("same pair for different principals", func(t *testing.T) {
		request := validAccessTransitionIntent()
		separate := request.Grants[0]
		separate.GrantID = "other-viewer"
		separate.Principal = "principal_other"
		request.Grants = append(request.Grants, separate)
		plan, err := request.Plan()
		if err != nil {
			t.Fatalf("different principals should retain separate authority: %v", err)
		}
		if len(plan.Grants) != 2 {
			t.Fatalf("same pair for separate principals produced %d grants, want 2", len(plan.Grants))
		}
	})

	t.Run("expanded action count is bounded", func(t *testing.T) {
		request := validAccessTransitionIntent()
		request.RoleBindings = nil
		request.Grants = make([]AccessTransitionGrantIntent, 43)
		for i := range request.Grants {
			request.Grants[i] = AccessTransitionGrantIntent{
				GrantID:      fmt.Sprintf("connection-%02d", i),
				Principal:    request.PublisherPrincipalID,
				ResourceID:   fmt.Sprintf("connection_resource_%02d", i),
				ResourceKind: "connection",
				Actions:      []string{string(ActionConnectionRead), string(ActionConnectionUse), string(ActionConnectionManage)},
			}
		}
		if _, err := request.Plan(); err == nil {
			t.Fatal("accepted 129 expanded assignments above the transition bound")
		}
	})
}
