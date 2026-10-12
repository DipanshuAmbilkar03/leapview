package module

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
)

func TestEverySelectableInstancePermissionRequiresAdminAndExactCredential(t *testing.T) {
	for _, definition := range access.PermissionCatalog() {
		if definition.Scope != access.PermissionScopeInstance || !definition.UISelectable {
			continue
		}
		t.Run(string(definition.Action), func(t *testing.T) {
			for _, scenario := range []string{"admin session", "user session", "admin token", "wrong instance", "empty token"} {
				t.Run(scenario, func(t *testing.T) {
					admin := scenario != "user session"
					module := browserGuardModule(browserGuardRepository{admin: admin}, Principal{ID: "principal"}, true)
					contract := APIGenOperationContract{OperationID: "matrix", Method: http.MethodPost, Path: "/matrix", Protected: true, AuthzMode: "privilege", Action: string(definition.Action), Resolver: string(access.TypedOperationResolverInstance), Extensions: map[string]any{apiGenObjectScopeExtension: "instance"}}
					authorizer, err := module.APIGenAuthorizer(nil, map[string]APIGenOperationContract{"matrix": contract}, APIGenResourceResolvers{Instance: func(*http.Request) string { return "instance_matrix" }})
					if err != nil {
						t.Fatal(err)
					}
					guarded, ok := authorizer.Protect("matrix", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
					if !ok {
						t.Fatal("permission gate is absent")
					}
					request := httptest.NewRequest(http.MethodPost, "/matrix", nil)
					if scenario != "admin session" && scenario != "user session" {
						instance := "instance_matrix"
						if scenario == "wrong instance" {
							instance = "instance_other"
						}
						pair, err := access.NewInstancePermissionPair(definition.Action, instance)
						if err != nil {
							t.Fatal(err)
						}
						permissions := []access.PermissionPair{pair}
						if scenario == "empty token" {
							permissions = []access.PermissionPair{}
						}
						request = request.WithContext(WithAPICredential(request.Context(), access.APICredential{Principal: access.Principal{ID: "principal"}, Token: access.APIToken{ID: "token", PrincipalID: "principal", PermissionProfile: access.PermissionCatalogProfile, Permissions: permissions}}))
					}
					recorder := httptest.NewRecorder()
					guarded.ServeHTTP(recorder, request)
					want := http.StatusForbidden
					if scenario == "admin session" || scenario == "admin token" {
						want = http.StatusNoContent
					}
					if recorder.Code != want {
						t.Fatalf("status=%d want=%d", recorder.Code, want)
					}
				})
			}
		})
	}
}
