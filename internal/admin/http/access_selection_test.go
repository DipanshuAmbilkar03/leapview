package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
	"github.com/stretchr/testify/require"
)

type accessSelectionRepository struct{ access.Repository }

func (accessSelectionRepository) ListPrincipals(context.Context, access.PrincipalFilter) ([]access.Principal, error) {
	return []access.Principal{{ID: "user", Kind: access.PrincipalKindUser}, {ID: "explicit", Kind: access.PrincipalKindUser}}, nil
}
func (accessSelectionRepository) ListAllGroups(context.Context) ([]access.Group, error) {
	return []access.Group{{ID: "group", Name: "Group"}}, nil
}
func (accessSelectionRepository) ListGroupMembersByGroup(context.Context, string) ([]access.GroupMember, error) {
	return nil, nil
}
func (accessSelectionRepository) ListSessions(context.Context, string) ([]access.Session, error) {
	return nil, nil
}
func (accessSelectionRepository) ListAuditEvents(context.Context, access.AuditEventFilter) ([]access.AuditEvent, error) {
	return nil, nil
}

func (accessSelectionRepository) ListServicePrincipalSecrets(context.Context, string) ([]access.ServicePrincipalSecret, error) {
	return nil, nil
}

func TestAccessMutationReloadRetainsSelectedDetailFromCommandRoute(t *testing.T) {
	handler := Handler{SettingsRepository: accessSelectionRepository{}}
	state, err := handler.loadAccessAdministration(httptest.NewRequest(http.MethodPost, "/admin/access/command?section=principal-detail&principal=user", nil), "admin", "", "")
	require.NoError(t, err)
	require.Equal(t, "user", state.SelectedPrincipalID)
	state, err = handler.loadAccessAdministration(httptest.NewRequest(http.MethodPost, "/admin/access/command?section=principal-detail&principal=user", nil), "admin", "explicit", "")
	require.NoError(t, err)
	require.Equal(t, "explicit", state.SelectedPrincipalID)
	state, err = handler.loadAccessAdministration(httptest.NewRequest(http.MethodPost, "/admin/access/command?section=group-detail&group=group", nil), "admin", "", "")
	require.NoError(t, err)
	require.Equal(t, "group", state.SelectedGroupID)
}
