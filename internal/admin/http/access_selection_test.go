package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flidai/leapview/internal/access"
	"github.com/go-chi/chi/v5"
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
	state, err := handler.loadRoleAccessAdministration(httptest.NewRequest(http.MethodPost, "/admin/access/command?section=principal-detail&principal=user", nil), "admin", "", "")
	require.NoError(t, err)
	require.Equal(t, "user", state.SelectedPrincipalID)
	state, err = handler.loadRoleAccessAdministration(httptest.NewRequest(http.MethodPost, "/admin/access/command?section=principal-detail&principal=user", nil), "admin", "explicit", "")
	require.NoError(t, err)
	require.Equal(t, "explicit", state.SelectedPrincipalID)
	state, err = handler.loadRoleAccessAdministration(httptest.NewRequest(http.MethodPost, "/admin/access/command?section=group-detail&group=group", nil), "admin", "", "")
	require.NoError(t, err)
	require.Equal(t, "group", state.SelectedGroupID)
	state, err = handler.loadAccessAdministration(httptest.NewRequest(http.MethodPost, "/admin/access/command?section=principal-detail&principal=deleted", nil), "admin", "", "")
	require.NoError(t, err)
	require.Empty(t, state.SelectedPrincipalID, "clearing a deleted selection must not restore it from the URL")
}

type failingServiceAccountRepository struct{ accessSelectionRepository }

func (failingServiceAccountRepository) ListServicePrincipals(context.Context) ([]access.Principal, error) {
	return nil, errors.New("query: relation \"service_principals\" does not exist (SQLSTATE 42P01)")
}

func TestAdminServerErrorsDoNotLeakCauseToClients(t *testing.T) {
	handler := Handler{SettingsRepository: failingServiceAccountRepository{}}
	request := httptest.NewRequest(http.MethodGet, "/admin/service-accounts/sa-1", nil)
	request.Header.Set("X-Request-ID", "req-admin-1")
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	recorder := httptest.NewRecorder()
	handler.ServiceAccountDetail(recorder, request)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "SQLSTATE")
	require.NotContains(t, recorder.Body.String(), "service_principals")
	require.Contains(t, recorder.Body.String(), "req-admin-1")
}
