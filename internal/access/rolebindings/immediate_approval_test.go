package rolebindings

import (
	"context"
	"testing"

	"github.com/flidai/leapview/internal/access"
	projectgraph "github.com/flidai/leapview/internal/project/graph"
)

func TestAdminApprovedRoleRequiresCurrentManagementAndCredentialAuthority(t *testing.T) {
	for _, scenario := range []string{"admin session", "delegator session", "admin token", "attenuated admin token", "forged approval"} {
		t.Run(scenario, func(t *testing.T) {
			mutation, authority, repo := roleBindingFixture(t)
			mutation.GrantAdminEnvelopeID = repo.envelope.ID
			manage, err := access.NewProjectPermissionPair(access.ActionProjectAccessManage, projectgraph.ResourceID(mutation.Scope.ProjectID))
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "delegator session" && scenario != "forged approval" {
				authority.Permissions = append(authority.Permissions, manage)
			}
			if scenario == "forged approval" {
				mutation.Binding.AdminApproved = true
			}
			if scenario == "admin token" || scenario == "attenuated admin token" {
				authority.Credential = access.RoleBindingAdministrationCredential{Class: access.GrantCredentialClassAPIToken, ID: "token", Fingerprint: "fp", PrincipalID: authority.ActorID, AuthenticatedPrincipalID: authority.ActorID, PermissionProfile: access.PermissionCatalogProfile, TokenPermissions: access.ClonePermissionPairs(authority.Permissions)}
				repo.envelope.Issuer.Credential = access.GrantCredentialEvidence{Class: access.GrantCredentialClassAPIToken, ID: "token", Fingerprint: "fp"}
				if scenario == "attenuated admin token" {
					authority.Credential.TokenPermissions = authority.Credential.TokenPermissions[:len(authority.Credential.TokenPermissions)-1]
				}
			}
			_, err = Apply(t.Context(), repo, mutation, access.RoleBindingAdministrationPorts{ResolveCurrentAuthority: func(context.Context, string) (access.RoleBindingAdministrationAuthority, error) {
				return authority, nil
			}}, func(apply func(access.Repository) (access.AuditEventInput, error)) error {
				_, err := apply(repo)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			want := scenario == "admin session" || scenario == "admin token"
			if repo.upsertInput.Binding.AdminApproved != want {
				t.Fatalf("approval=%t, want %t", repo.upsertInput.Binding.AdminApproved, want)
			}
		})
	}
}
